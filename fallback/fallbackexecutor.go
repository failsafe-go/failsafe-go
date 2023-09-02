package fallback

import (
	"failsafe"
	"failsafe/spi"
)

// fallbackExecutor is a failsafe.PolicyExecutor that handles failures according to a Fallback.
type fallbackExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*fallback[R]
}

var _ failsafe.PolicyExecutor[any] = &fallbackExecutor[any]{}

// Apply performs an execution by calling pre-execute else calling the supplier, applying a fallback if it fails, and
// calling post-execute.
func (e *fallbackExecutor[R]) Apply(innerFn failsafe.ExecutionHandler[R]) failsafe.ExecutionHandler[R] {
	return func(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
		result := innerFn(exec)
		if e.IsFailure(result) {
			event := failsafe.ExecutionAttemptedEvent[R]{
				Execution: exec.Execution,
			}
			if e.config.failedAttemptListener != nil {
				e.config.failedAttemptListener(event)
			}

			fallbackResult, err := e.fallback.config.fn(event)
			result = &failsafe.ExecutionResult[R]{
				Result: fallbackResult,
				Err:    err,
			}
		}
		return e.PostExecute(exec, result)
	}
}
