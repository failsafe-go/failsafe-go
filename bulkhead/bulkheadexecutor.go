package bulkhead

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// bulkheadExecutor is a failsafe.PolicyExecutor that handles failures according to a Bulkhead.
type bulkheadExecutor[R any] struct {
	*policy.BasePolicyExecutor[R]
	*bulkhead[R]
}

var _ policy.PolicyExecutor[any] = &bulkheadExecutor[any]{}

func (be *bulkheadExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R] {
	return func(exec failsafe.Execution[R]) *common.ExecutionResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		if err := be.bulkhead.AcquirePermitWithMaxWait(execInternal.Context(), be.config.maxWaitTime); err != nil {
			if be.config.onBulkheadFull != nil {
				be.config.onBulkheadFull(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: exec,
				})
			}
			return internal.FailureResult[R](err)
		}
		return innerFn(exec)
	}
}
