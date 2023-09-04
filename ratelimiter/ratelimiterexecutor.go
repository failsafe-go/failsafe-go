package ratelimiter

import (
	"failsafe"
	"failsafe/spi"
)

// rateLimiterExecutor is a failsafe.PolicyExecutor that handles failures according to a RateLimiter.
type rateLimiterExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*rateLimiter[R]
}

var _ failsafe.PolicyExecutor[any] = &rateLimiterExecutor[any]{}

func (rle *rateLimiterExecutor[R]) PreExecute(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
	if err := rle.rateLimiter.AcquirePermitWithMaxWait(exec.Context, rle.rateLimiter.config.maxWaitTime); err != nil {
		return &failsafe.ExecutionResult[R]{
			Err:      err,
			Complete: true,
		}
	}
	return nil
}
