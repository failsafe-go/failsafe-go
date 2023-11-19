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

// retryPolicyExecutor is a failsafe.Executor that handles failures according to a RetryPolicy.
type retryPolicyExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*retryPolicy[R]

	// Mutable state
	failedAttempts  int
	retriesExceeded bool
	lastDelay       time.Duration // The last fixed, backoff, random, or computed delay time
}

func (rpe *retryPolicyExecutor[R]) PreExecute(exec policy.ExecutionInternal[R]) *common.PolicyResult[R] {
	return rpe.BaseExecutor.PreExecute(exec)
}

func (rpe *retryPolicyExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		for {
			result := innerFn(exec)
			if rpe.retriesExceeded || execInternal.IsCanceledForPolicy(rpe.PolicyIndex) {
				return execInternal.Result()
			}

			result = rpe.PostExecute(execInternal, result)
			if result.Done {
				return result
			}

			// Delay
			delay := rpe.getDelay(exec)
			if rpe.config.onRetryScheduled != nil {
				rpe.config.onRetryScheduled(failsafe.ExecutionScheduledEvent[R]{
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
			if !execInternal.InitializeAttempt(rpe.PolicyIndex) {
				return execInternal.Result()
			}

			// Call retry listener
			if rpe.config.onRetry != nil {
				rpe.config.onRetry(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: execInternal.CopyWithResult(result),
				})
			}
		}
	}
}

// OnFailure updates failedAttempts and retriesExceeded, and calls event listeners
func (rpe *retryPolicyExecutor[R]) OnFailure(exec policy.ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R] {
	rpe.BaseExecutor.OnFailure(exec, result)

	rpe.failedAttempts++
	maxRetriesExceeded := rpe.config.maxRetries != -1 && rpe.failedAttempts > rpe.config.maxRetries
	maxDurationExceeded := rpe.config.maxDuration != 0 && exec.ElapsedTime() > rpe.config.maxDuration
	rpe.retriesExceeded = maxRetriesExceeded || maxDurationExceeded
	isAbortable := rpe.config.isAbortable(result.Result, result.Error)
	shouldRetry := !isAbortable && !rpe.retriesExceeded && rpe.config.allowsRetries()
	done := isAbortable || !shouldRetry

	// Call listeners
	if isAbortable && rpe.config.onAbort != nil {
		rpe.config.onAbort(failsafe.ExecutionEvent[R]{ExecutionAttempt: exec.Copy()})
	}
	if rpe.retriesExceeded {
		if !isAbortable && rpe.config.onRetriesExceeded != nil {
			rpe.config.onRetriesExceeded(failsafe.ExecutionEvent[R]{ExecutionAttempt: exec.Copy()})
		}
		if rpe.config.returnLastFailure {
			return result.WithDone(false, false)
		} else {
			return internal.FailureResult[R](&RetriesExceededError{
				lastResult: result.Result,
				lastError:  result.Error,
			})
		}
	}
	return result.WithDone(done, false)
}

// getDelay updates lastDelay and returns the new delay
func (rpe *retryPolicyExecutor[R]) getDelay(exec failsafe.ExecutionAttempt[R]) time.Duration {
	delay := rpe.lastDelay
	computedDelay := rpe.config.ComputeDelay(exec)
	if computedDelay != -1 {
		delay = computedDelay
	} else {
		delay = getFixedOrRandomDelay(rpe.config, delay)
		delay = adjustForBackoff(rpe.config, exec, delay)
		rpe.lastDelay = delay
	}
	if delay != 0 {
		delay = adjustForJitter(rpe.config, delay)
	}
	delay = adjustForMaxDuration(rpe.config, delay, exec.ElapsedTime())
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
