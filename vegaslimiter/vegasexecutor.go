package vegaslimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// vegasExecutor is a policy.Executor that handles failures according to an vegasLimiter.
type vegasExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*vegasLimiter[R]
}

var _ policy.Executor[any] = &vegasExecutor[any]{}

func (e *vegasExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		if permit, ok := e.TryAcquirePermit(); !ok {
			return internal.FailureResult[R](adaptivelimiter.ErrExceeded)
		} else {
			execInternal := exec.(policy.ExecutionInternal[R])
			result := innerFn(exec)
			result = e.PostExecute(execInternal, result)
			permit.Record()
			return result
		}
	}
}
