package vegaslimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

type priorityLimiterExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*priorityLimiter[R]
}

var _ policy.Executor[any] = &priorityLimiterExecutor[any]{}

func (e *priorityLimiterExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		priority := PriorityLow
		if untypedPriority := exec.Context().Value(PriorityKey); untypedPriority != nil {
			priority, _ = untypedPriority.(Priority)
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
