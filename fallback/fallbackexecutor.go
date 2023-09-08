package fallback

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/spi"
)

// fallbackExecutor is a failsafe.PolicyExecutor that handles failures according to a Fallback.
type fallbackExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*fallback[R]
}

var _ failsafe.PolicyExecutor[any] = &fallbackExecutor[any]{}

// Apply performs an execution by calling the innerFn, applying a fallback if it fails, and calling post-execute.
func (e *fallbackExecutor[R]) Apply(innerFn failsafe.ExecutionHandler[R]) failsafe.ExecutionHandler[R] {
	return func(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
		result := innerFn(exec)
		if exec.IsCanceled(e.PolicyIndex) {
			return result
		}

		if e.IsFailure(result) {
			// Call fallback fn
			fallbackResult, fallbackError := e.config.fn(exec.ExecutionForResult(result))
			if exec.IsCanceled(e.PolicyIndex) {
				return result
			}

			if e.config.onComplete != nil {
				e.config.onComplete(failsafe.ExecutionCompletedEvent[R]{
					ExecutionStats: exec.ExecutionStats,
					Result:         fallbackResult,
					Error:          fallbackError,
				})
			}

			result = &failsafe.ExecutionResult[R]{
				Result:     fallbackResult,
				Error:      fallbackError,
				Complete:   true,
				Success:    true,
				SuccessAll: result.SuccessAll,
			}
		}
		return e.PostExecute(exec, result)
	}
}
