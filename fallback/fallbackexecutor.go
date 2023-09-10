package fallback

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/spi"
)

// fallbackExecutor is a failsafe.PolicyExecutor that handles failures according to a Fallback.
type fallbackExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*fallback[R]
}

var _ spi.PolicyExecutor[any] = &fallbackExecutor[any]{}

// Apply performs an execution by calling the innerFn, applying a fallback if it fails, and calling post-execute.
func (e *fallbackExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R] {
	return func(exec failsafe.Execution[R]) *common.ExecutionResult[R] {
		execInternal := exec.(spi.ExecutionInternal[R])
		result := innerFn(exec)
		if execInternal.IsCanceledForPolicy(e.PolicyIndex) {
			return result
		}

		if e.IsFailure(result) {
			// Call fallback fn
			fallbackResult, fallbackError := e.config.fn(execInternal.ExecutionForResult(result))
			if execInternal.IsCanceledForPolicy(e.PolicyIndex) {
				return result
			}

			if e.config.onFallbackExecuted != nil {
				e.config.onFallbackExecuted(failsafe.ExecutionCompletedEvent[R]{
					ExecutionStats: exec,
					Result:         fallbackResult,
					Error:          fallbackError,
				})
			}

			result = &common.ExecutionResult[R]{
				Result:     fallbackResult,
				Error:      fallbackError,
				Complete:   true,
				Success:    true,
				SuccessAll: result.SuccessAll,
			}
		}
		return e.PostExecute(execInternal, result)
	}
}
