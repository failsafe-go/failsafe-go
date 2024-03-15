package retrypolicy

import (
	"math/rand"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// retryPolicyExecutor is a policy.Executor that handles failures according to a RetryPolicy.
type retryPolicyExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*retryPolicy[R]

	// Mutable state
	failedAttempts  int
	retriesExceeded bool
	lastDelay       time.Duration // The last fixed, backoff, random, or computed delay time
}

var _ policy.Executor[any] = &retryPolicyExecutor[any]{}

func (e *retryPolicyExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])

		for {
			result := innerFn(exec)
			if canceled, cancelResult := execInternal.IsCanceledWithResult(); canceled {
				return cancelResult
			}
			if e.retriesExceeded {
				return result
			}

			result = e.PostExecute(execInternal, result)
			if result.Done {
				return result
			}

			// Record result
			if cancelResult := execInternal.RecordResult(result); cancelResult != nil {
				return cancelResult
			}

			// Delay
			delay := e.getDelay(exec)
			if e.config.onRetryScheduled != nil {
				e.config.onRetryScheduled(failsafe.ExecutionScheduledEvent[R]{
					ExecutionAttempt: execInternal.CopyWithResult(result),
					Delay:            delay,
				})
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-exec.Canceled():
				timer.Stop()
			}

			// Prepare for next iteration
			if cancelResult := execInternal.InitializeRetry(); cancelResult != nil {
				return cancelResult
			}

			// Call retry listener
			if e.config.onRetry != nil {
				e.config.onRetry(failsafe.ExecutionEvent[R]{ExecutionAttempt: execInternal.CopyWithResult(result)})
			}
		}
	}
}

// OnFailure updates failedAttempts and retriesExceeded, and calls event listeners
func (e *retryPolicyExecutor[R]) OnFailure(exec policy.ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R] {
	e.BaseExecutor.OnFailure(exec, result)

	e.failedAttempts++
	maxRetriesExceeded := e.config.maxRetries != -1 && e.failedAttempts > e.config.maxRetries
	maxDurationExceeded := e.config.maxDuration != 0 && exec.ElapsedTime() > e.config.maxDuration
	e.retriesExceeded = maxRetriesExceeded || maxDurationExceeded
	isAbortable := e.config.IsAbortable(result.Result, result.Error)
	shouldRetry := !isAbortable && !e.retriesExceeded && e.config.allowsRetries()
	done := isAbortable || !shouldRetry

	// Call listeners
	if isAbortable && e.config.onAbort != nil {
		e.config.onAbort(failsafe.ExecutionEvent[R]{ExecutionAttempt: exec.CopyWithResult(result)})
	}
	if e.retriesExceeded {
		if !isAbortable && e.config.onRetriesExceeded != nil {
			e.config.onRetriesExceeded(failsafe.ExecutionEvent[R]{ExecutionAttempt: exec.CopyWithResult(result)})
		}
		if !e.config.returnLastFailure {
			return internal.FailureResult[R](&ExceededError{
				lastResult: result.Result,
				lastError:  result.Error,
			})
		}
	}
	return result.WithDone(done, false)
}

// getDelay updates lastDelay and returns the new delay
func (e *retryPolicyExecutor[R]) getDelay(exec failsafe.ExecutionAttempt[R]) time.Duration {
	delay := e.lastDelay
	computedDelay := e.config.ComputeDelay(exec)
	if computedDelay != -1 {
		delay = computedDelay
	} else {
		delay = getFixedOrRandomDelay(e.config, delay)
		delay = adjustForBackoff(e.config, exec, delay)
		e.lastDelay = delay
	}
	if delay != 0 {
		delay = adjustForJitter(e.config, delay)
	}
	delay = adjustForMaxDuration(e.config, delay, exec.ElapsedTime())
	return delay
}

func getFixedOrRandomDelay[R any](config *retryPolicyConfig[R], delay time.Duration) time.Duration {
	if delay == 0 && config.Delay != 0 {
		return config.Delay
	}
	if config.delayMin != 0 && config.delayMax != 0 {
		return time.Duration(util.RandomDelayInRange(config.delayMin.Nanoseconds(), config.delayMax.Nanoseconds(), rand.Float64()))
	}
	return delay
}

func adjustForBackoff[R any](config *retryPolicyConfig[R], exec failsafe.ExecutionAttempt[R], delay time.Duration) time.Duration {
	if exec.Attempts() != 1 && config.maxDelay != 0 {
		backoffDelay := time.Duration(float32(delay) * config.delayFactor)
		delay = min(backoffDelay, config.maxDelay)
	}
	return delay
}

func adjustForJitter[R any](config *retryPolicyConfig[R], delay time.Duration) time.Duration {
	if config.jitter != 0 {
		delay = util.RandomDelay(delay, config.jitter, rand.Float64())
	} else if config.jitterFactor != 0 {
		delay = util.RandomDelayFactor(delay, config.jitterFactor, rand.Float32())
	}
	return delay
}

func adjustForMaxDuration[R any](config *retryPolicyConfig[R], delay time.Duration, elapsed time.Duration) time.Duration {
	if config.maxDuration != 0 {
		delay = min(delay, config.maxDuration-elapsed)
	}
	return max(0, delay)
}
