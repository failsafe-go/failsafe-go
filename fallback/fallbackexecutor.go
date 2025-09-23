package fallback

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/policy"
)

// executor is a policy.Executor that handles failures according to a Fallback.
type executor[R any] struct {
	policy.BaseExecutor[R]
	*fallback[R]
}

var _ policy.Executor[any] = &executor[any]{}

// Apply performs an execution by calling the innerFn, applying a fallback if it fails, and calling post-execute.
func (e *executor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		result := innerFn(exec)
		result = e.PostExecute(execInternal, result)
		if !result.Success {
			// Check for cancellation during execution
			if canceled, cancelResult := execInternal.IsCanceledWithResult(); canceled {
				return cancelResult
			}
			// Call fallback fn
			fallbackResult, fallbackError := e.fn(execInternal.CopyWithResult(result))
			// Check for cancellation during fallback
			if canceled, cancelResult := execInternal.IsCanceledWithResult(); canceled {
				return cancelResult
			}
			if e.onFallbackExecuted != nil {
				e.onFallbackExecuted(failsafe.ExecutionDoneEvent[R]{
					ExecutionInfo: execInternal,
					Result:        fallbackResult,
					Error:         fallbackError,
				})
			}

			success := !e.IsFailure(fallbackResult, fallbackError)
			result = &common.PolicyResult[R]{
				Result:     fallbackResult,
				Error:      fallbackError,
				Done:       true,
				Success:    success,
				SuccessAll: success,
			}
		}
		return result
	}
}
