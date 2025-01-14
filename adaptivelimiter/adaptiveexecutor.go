package adaptivelimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// adaptiveExecutor is a policy.Executor that handles failures according to an adaptiveLimiter.
type adaptiveExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*adaptiveLimiter[R]
}

var _ policy.Executor[any] = &adaptiveExecutor[any]{}

func (e *adaptiveExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		if permit, ok := e.TryAcquirePermit(); !ok {
			return internal.FailureResult[R](ErrExceeded)
		} else {
			execInternal := exec.(policy.ExecutionInternal[R])
			result := innerFn(exec)
			result = e.PostExecute(execInternal, result)
			permit.Record()
			return result
		}
	}
}
