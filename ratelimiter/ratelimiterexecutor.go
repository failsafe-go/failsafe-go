package ratelimiter

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/spi"
)

// rateLimiterExecutor is a failsafe.PolicyExecutor that handles failures according to a RateLimiter.
type rateLimiterExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*rateLimiter[R]
}

var _ failsafe.PolicyExecutor[any] = &rateLimiterExecutor[any]{}

func (rle *rateLimiterExecutor[R]) PreExecute(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
	if err := rle.rateLimiter.acquirePermitsWithMaxWait(exec.Context, exec.Canceled(), 1, rle.config.maxWaitTime); err != nil {
		return internal.FailureResult[R](err)
	}
	return nil
}
