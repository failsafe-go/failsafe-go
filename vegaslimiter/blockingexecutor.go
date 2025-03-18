package vegaslimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// blockingLimiterExecutor is a policy.Executor that handles failures according to a blockingLimiter.
type blockingLimiterExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*blockingLimiter[R]
}

var _ policy.Executor[any] = &blockingLimiterExecutor[any]{}

func (e *blockingLimiterExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		if permit, err := e.AcquirePermit(exec.Context()); err != nil {
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
