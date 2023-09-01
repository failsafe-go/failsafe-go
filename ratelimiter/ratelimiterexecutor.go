package ratelimiter

import (
	"failsafe"
)

// rateLimiterExecutor is a failsafe.PolicyExecutor that handles failures according to a RateLimiter.
type rateLimiterExecutor[R any] struct {
	*failsafe.BasePolicyExecutor[R]
	*rateLimiter[R]
}

var _ failsafe.PolicyExecutor[any] = &rateLimiterExecutor[any]{}

func (rle *rateLimiterExecutor[R]) PreExecute() *failsafe.ExecutionResult[R] {
	if !rle.rateLimiter.TryAcquirePermitWithMaxWait(rle.rateLimiter.config.maxWaitTime) {
		return &failsafe.ExecutionResult[R]{
			Err: ErrRateLimitExceeded,
		}
	}
	return nil
}
