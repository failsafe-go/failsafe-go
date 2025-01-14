package adaptivelimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// latencyLimiterExecutor is a policy.Executor that handles failures according to a latencyLimiter.
type latencyLimiterExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*latencyLimiter[R]
}

var _ policy.Executor[any] = &latencyLimiterExecutor[any]{}

func (e *latencyLimiterExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
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
