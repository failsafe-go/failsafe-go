package bulkhead

import (
	"errors"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// bulkheadExecutor is a policy.Executor that handles failures according to a Bulkhead.
type bulkheadExecutor[R any] struct {
	*policy.BaseExecutor[BulkheadBuilder[R], R]
	*bulkhead[R]
}

var _ policy.Executor[any] = &bulkheadExecutor[any]{}

func (e *bulkheadExecutor[R]) PreExecute(exec policy.ExecutionInternal[R]) *common.PolicyResult[R] {
	execInternal := exec.(policy.ExecutionInternal[R])
	if err := e.AcquirePermitWithMaxWait(execInternal.Context(), e.config.maxWaitTime); err != nil {
		if errors.Is(err, ErrFull) && e.config.onFull != nil {
			e.config.onFull(failsafe.ExecutionEvent[R]{
				ExecutionAttempt: execInternal,
			})
		}
		return internal.FailureResult[R](err)
	}
	return nil
}

func (e *bulkheadExecutor[R]) PostExecute(_ policy.ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R] {
	e.bulkhead.ReleasePermit()
	return result
}
