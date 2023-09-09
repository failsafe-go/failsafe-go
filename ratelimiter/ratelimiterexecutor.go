package ratelimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/spi"
)

// rateLimiterExecutor is a failsafe.PolicyExecutor that handles failures according to a RateLimiter.
type rateLimiterExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*rateLimiter[R]
}

var _ spi.PolicyExecutor[any] = &rateLimiterExecutor[any]{}

func (rle *rateLimiterExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R] {
	return func(exec failsafe.Execution[R]) *common.ExecutionResult[R] {
		execInternal := exec.(spi.ExecutionInternal[R])
		if err := rle.rateLimiter.acquirePermitsWithMaxWait(execInternal.Context(), exec.Canceled(), 1, rle.config.maxWaitTime); err != nil {
			if rle.config.onRateLimitExceeded != nil {
				rle.config.onRateLimitExceeded(failsafe.ExecutionCompletedEvent[R]{
					ExecutionStats: exec,
					Error:          err,
				})
			}
			return internal.FailureResult[R](err)
		}
		return innerFn(exec)
	}
}
