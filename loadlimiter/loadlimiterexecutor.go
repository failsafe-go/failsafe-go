package loadlimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// loadLimiterExecutor is a policy.Executor that handles failures according to a LoadLimiter.
type loadLimiterExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*loadLimiter[R]
}

var _ policy.Executor[any] = &loadLimiterExecutor[any]{}

func (e *loadLimiterExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		// execInternal := exec.(policy.ExecutionInternal[R])
		if err := e.acquirePermit(exec); err != nil {
			// if e.config.onRateLimitExceeded != nil {
			// 	e.config.onRateLimitExceeded(failsafe.ExecutionEvent[R]{
			// 		ExecutionAttempt: execInternal,
			// 	})
			// }
			return internal.FailureResult[R](err)
		}
		return innerFn(exec)
	}
}
