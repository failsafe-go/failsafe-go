package adaptivelimiter

import (
	"errors"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// adaptiveExecutor is a policy.Executor that handles failures according to an AdaptiveLimiter.
type adaptiveExecutor[R any] struct {
	*policy.BaseExecutor[R]
	AdaptiveLimiter[R]
}

var _ policy.Executor[any] = &adaptiveExecutor[any]{}

func (e *adaptiveExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		limiter := e.AdaptiveLimiter.(*adaptiveLimiter[R])
		permit, err := e.AcquirePermitWithMaxWait(exec.Context(), limiter.maxWaitTime)

		if err != nil {
			// Check for cancellation while waiting for a permit
			if canceled, cancelResult := execInternal.IsCanceledWithResult(); canceled {
				return cancelResult
			}

			// Handle exceeded
			if limiter.onExceeded != nil && errors.Is(err, ErrExceeded) {
				limiter.onExceeded(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: exec,
				})
			}
			return internal.FailureResult[R](err)
		}

		result := innerFn(exec)
		result = e.PostExecute(execInternal, result)
		permit.Record()
		return result
	}
}
