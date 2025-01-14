package adaptivelimiterold

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

type priorityExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*priorityBlockingLimiter[R]
}

var _ policy.Executor[any] = &blockingExecutor[any]{}

func (e *priorityExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		priority := PriorityLow
		if untypedKey := exec.Context().Value(PriorityKey); untypedKey != nil {
			priority, _ = untypedKey.(Priority)
		}

		if permit, err := e.AcquirePermit(exec.Context(), priority); err != nil {
			return internal.FailureResult[R](err)
		} else {
			execInternal := exec.(policy.ExecutionInternal[R])
			result := innerFn(exec)
			result = e.PostExecute(execInternal, result)
			permit.Record()
			return result
		}
	}
}
