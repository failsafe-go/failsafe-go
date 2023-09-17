package ratelimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// rateLimiterExecutor is a failsafe.Executor that handles failures according to a RateLimiter.
type rateLimiterExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*rateLimiter[R]
}

var _ policy.Executor[any] = &rateLimiterExecutor[any]{}

func (rle *rateLimiterExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		if err := rle.rateLimiter.acquirePermitsWithMaxWait(execInternal.Context(), exec.Canceled(), 1, rle.config.maxWaitTime); err != nil {
			if rle.config.onRateLimitExceeded != nil {
				rle.config.onRateLimitExceeded(failsafe.ExecutionEvent[R]{
					ExecutionAttempt: execInternal.Copy(),
				})
			}
			return internal.FailureResult[R](err)
		}
		return innerFn(exec)
	}
}
