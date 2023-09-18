package fallback

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/policy"
)

// fallbackExecutor is a failsafe.Executor that handles failures according to a Fallback.
type fallbackExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*fallback[R]
}

var _ policy.Executor[any] = &fallbackExecutor[any]{}

// Apply performs an execution by calling the innerFn, applying a fallback if it fails, and calling post-execute.
func (e *fallbackExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		result := innerFn(exec)
		if execInternal.IsCanceledForPolicy(e.PolicyIndex) {
			return result
		}

		result = e.PostExecute(execInternal, result)
		if !result.Success {
			// Call fallback fn
			fallbackResult, fallbackError := e.config.fn(execInternal.CopyWithResult(result))
			if execInternal.IsCanceledForPolicy(e.PolicyIndex) {
				return result
			}

			if e.config.onFallbackExecuted != nil {
				e.config.onFallbackExecuted(failsafe.ExecutionDoneEvent[R]{
					ExecutionStats: execInternal.Copy(),
					Result:         fallbackResult,
					Error:          fallbackError,
				})
			}

			success := e.IsFailure(fallbackResult, fallbackError)
			result = &common.PolicyResult[R]{
				Result:     fallbackResult,
				Error:      fallbackError,
				Done:       true,
				Success:    success,
				SuccessAll: success && result.SuccessAll,
			}
		}
		return result
	}
}
