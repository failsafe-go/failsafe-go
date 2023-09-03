package retrypolicy

import (
	"math/rand"
	"time"

	"failsafe"
	"failsafe/internal"
	"failsafe/internal/util"
	"failsafe/spi"
)

// retryPolicyExecutor is a failsafe.PolicyExecutor that handles failures according to a RetryPolicy.
type retryPolicyExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*retryPolicy[R]

	// Mutable state
	failedAttempts  int
	retriesExceeded bool
	lastDelay       time.Duration // The last fixed, backoff, random, or computed delay time
}

func (rpe *retryPolicyExecutor[R]) PreExecute() *failsafe.ExecutionResult[R] {
	return rpe.BasePolicyExecutor.PreExecute()
}

func (rpe *retryPolicyExecutor[R]) Apply(innerFn failsafe.ExecutionHandler[R]) failsafe.ExecutionHandler[R] {
	return func(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
		for {
			result := innerFn(exec)
			if rpe.retriesExceeded {
				return result
			}

			result = rpe.PostExecute(exec, result)
			if result.Complete {
				return result
			}

			// Delay
			delay := rpe.getDelay(&exec.Execution)
			if rpe.config.retryScheduledListener != nil {
				rpe.config.retryScheduledListener(failsafe.ExecutionScheduledEvent[R]{
					Execution: internal.NewExecutionForResult(result, &exec.Execution),
					Delay:     delay,
				})
			}
			if exec.Context != nil {
				select {
				case <-time.After(delay):
				case <-exec.Context.Done():
					return result
				}
			} else {
				time.Sleep(delay)
			}

			// Call retry listener
			if rpe.config.retryListener != nil {
				rpe.config.retryListener(failsafe.ExecutionAttemptedEvent[R]{
					Execution: internal.NewExecutionForResult(result, &exec.Execution),
				})
			}

			// Prepare for next iteration
			exec.InitializeAttempt()
		}
	}
}

// OnFailure updates failedAttempts and retriesExceeded, and calls event listeners
func (rpe *retryPolicyExecutor[R]) OnFailure(exec *failsafe.Execution[R], result *failsafe.ExecutionResult[R]) *failsafe.ExecutionResult[R] {
	rpe.failedAttempts++
	maxRetriesExceeded := rpe.config.maxRetries != -1 && rpe.failedAttempts > rpe.config.maxRetries
	maxDurationExceeded := rpe.config.maxDuration != 0 && exec.GetElapsedTime() > rpe.config.maxDuration
	rpe.retriesExceeded = maxRetriesExceeded || maxDurationExceeded
	isAbortable := rpe.config.isAbortable(result.Result, result.Err)
	shouldRetry := !isAbortable && !rpe.retriesExceeded && rpe.config.allowsRetries()
	completed := isAbortable || !shouldRetry

	// Call listeners
	if rpe.config.failedAttemptListener != nil {
		rpe.config.failedAttemptListener(failsafe.ExecutionAttemptedEvent[R]{
			Execution: internal.NewExecutionForResult(result, exec),
		})
	}
	if isAbortable && rpe.config.abortListener != nil {
		rpe.config.abortListener(failsafe.ExecutionCompletedEvent[R]{
			Result:         exec.LastResult,
			Err:            exec.LastErr,
			ExecutionStats: exec.ExecutionStats,
		})
	}
	if rpe.retriesExceeded && rpe.config.retriesExceededListener != nil {
		rpe.config.retriesExceededListener(failsafe.ExecutionCompletedEvent[R]{
			Result:         exec.LastResult,
			Err:            exec.LastErr,
			ExecutionStats: exec.ExecutionStats,
		})
	}
	return result.WithComplete(completed, false)
}

// getDelay updates lastDelay and returns the new delay
func (rpe *retryPolicyExecutor[R]) getDelay(exec *failsafe.Execution[R]) time.Duration {
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
	delay = adjustForMaxDuration(rpe.config, delay, exec.GetElapsedTime())
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

func adjustForBackoff[R any](config *retryPolicyConfig[R], exec *failsafe.Execution[R], delay time.Duration) time.Duration {
	if exec.Attempts != 1 && config.maxDelay != 0 {
		backoffDelay := time.Duration(float32(delay) * config.delayFactor)
		delay = util.Min(backoffDelay, config.maxDelay)
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
		delay = util.Min(delay, config.maxDuration-elapsed)
	}
	return util.Max(0, delay)
}
