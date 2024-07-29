package ratelimiter

import (
	"errors"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// rateLimiterExecutor is a policy.Executor that handles failures according to a RateLimiter.
type rateLimiterExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*rateLimiter[R]
}

var _ policy.Executor[any] = &rateLimiterExecutor[any]{}

func (e *rateLimiterExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		if err := e.acquirePermitsWithMaxWait(execInternal.Context(), exec, 1, e.config.maxWaitTime); err != nil {
			if e.config.onRateLimitExceeded != nil && errors.Is(err, ErrExceeded) {
				e.config.onRateLimitExceeded(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: execInternal,
				})
			}
			return internal.FailureResult[R](err)
		}
		return innerFn(exec)
	}
}
