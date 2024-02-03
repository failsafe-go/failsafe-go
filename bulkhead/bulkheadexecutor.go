package bulkhead

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// bulkheadExecutor is a failsafe.Executor that handles failures according to a Bulkhead.
type bulkheadExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*bulkhead[R]
}

var _ policy.Executor[any] = &bulkheadExecutor[any]{}

func (be *bulkheadExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		if err := be.bulkhead.AcquirePermitWithMaxWait(execInternal.Context(), be.config.maxWaitTime); err != nil {
			if be.config.onFull != nil {
				be.config.onFull(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: execInternal,
				})
			}
			return internal.FailureResult[R](err)
		}
		return innerFn(exec)
	}
}
