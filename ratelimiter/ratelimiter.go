package ratelimiter

import (
	"errors"
	"time"

	"failsafe"
	"failsafe/internal/util"
)

var ErrRateLimitExceeded = errors.New("rate limit exceeded")

type RateLimiter[R any] interface {
	failsafe.Policy[R]
	AcquirePermit()
	AcquirePermits(permits int)
	AcquirePermitWithMaxWait(maxWaitTime time.Duration) error
	AcquirePermitsWithMaxWait(requestedPermits int, maxWaitTime time.Duration) error
	ReservePermit() time.Duration
	ReservePermits(permits int) time.Duration
	TryAcquirePermit() bool
	TryAcquirePermits(permits int) bool
	TryAcquirePermitWithMaxWait(maxWaitTime time.Duration) bool
	TryAcquirePermitsWithMaxWait(requestedPermits int, maxWaitTime time.Duration) bool
	TryReservePermit(maxWaitTime time.Duration) time.Duration
	TryReservePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration
}

/*
RateLimiterBuilder builds RateLimiter instances.

This type is not threadsafe.
*/
type RateLimiterBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[RateLimiterBuilder[R], R]
	WithMaxWaitTime(maxWaitTime time.Duration) RateLimiterBuilder[R]
	Build() RateLimiter[R]
}

type rateLimiterConfig[R any] struct {
	*failsafe.BaseListenablePolicy[R]

	// Smooth
	interval time.Duration

	// Bursty
	periodPermits int
	period        time.Duration

	// Common
	maxWaitTime time.Duration
}

var _ RateLimiterBuilder[any] = &rateLimiterConfig[any]{}

func SmoothBuilder[R any](maxExecutions int64, period time.Duration) RateLimiterBuilder[R] {
	return &rateLimiterConfig[R]{
		BaseListenablePolicy: &failsafe.BaseListenablePolicy[R]{},
		interval:             period / time.Duration(maxExecutions),
	}
}

func SmoothBuilderForMaxRate[R any](maxRate time.Duration) RateLimiterBuilder[R] {
	return &rateLimiterConfig[R]{
		BaseListenablePolicy: &failsafe.BaseListenablePolicy[R]{},
		interval:             maxRate,
	}
}

func BurstyBuilder[R any](maxExecutions int, period time.Duration) RateLimiterBuilder[R] {
	return &rateLimiterConfig[R]{
		BaseListenablePolicy: &failsafe.BaseListenablePolicy[R]{},
		periodPermits:        maxExecutions,
		period:               period,
	}
}

func (c *rateLimiterConfig[R]) WithMaxWaitTime(maxWaitTime time.Duration) RateLimiterBuilder[R] {
	c.maxWaitTime = maxWaitTime
	return c
}

func (c *rateLimiterConfig[R]) OnSuccess(listener func(event failsafe.ExecutionCompletedEvent[R])) RateLimiterBuilder[R] {
	c.BaseListenablePolicy.OnSuccess(listener)
	return c
}

func (c *rateLimiterConfig[R]) OnFailure(listener func(event failsafe.ExecutionCompletedEvent[R])) RateLimiterBuilder[R] {
	c.BaseListenablePolicy.OnFailure(listener)
	return c
}

func (c *rateLimiterConfig[R]) Build() RateLimiter[R] {
	if c.interval != 0 {
		return &rateLimiter[R]{
			config: c,
			stats: &smoothRateLimiterStats[R]{
				config:    c,
				stopwatch: util.NewStopwatch(),
			},
		}
	}
	return &rateLimiter[R]{
		config: c,
		stats: &burstyRateLimiterStats[R]{
			config:           c,
			Stopwatch:        util.NewStopwatch(),
			availablePermits: c.periodPermits,
		},
	}
}

type rateLimiter[R any] struct {
	config *rateLimiterConfig[R]
	stats  rateLimiterStats
}

var _ RateLimiter[any] = &rateLimiter[any]{}

func (r *rateLimiter[R]) AcquirePermit() {
	r.AcquirePermits(1)
}

func (r *rateLimiter[R]) AcquirePermits(permits int) {
	time.Sleep(r.ReservePermits(permits))
}

func (r *rateLimiter[R]) AcquirePermitWithMaxWait(maxWaitTime time.Duration) error {
	return r.AcquirePermitsWithMaxWait(1, maxWaitTime)
}

func (r *rateLimiter[R]) AcquirePermitsWithMaxWait(requestedPermits int, maxWaitTime time.Duration) error {
	if !r.TryAcquirePermitsWithMaxWait(requestedPermits, maxWaitTime) {
		return ErrRateLimitExceeded
	}
	return nil
}

func (r *rateLimiter[R]) ReservePermit() time.Duration {
	return r.ReservePermits(1)
}

func (r *rateLimiter[R]) ReservePermits(permits int) time.Duration {
	return r.stats.acquirePermits(permits, -1)
}

func (r *rateLimiter[R]) TryAcquirePermit() bool {
	return r.TryAcquirePermits(1)
}

func (r *rateLimiter[R]) TryAcquirePermits(permits int) bool {
	return r.TryReservePermits(permits, 0) == 0
}

func (r *rateLimiter[R]) TryAcquirePermitWithMaxWait(maxWaitTime time.Duration) bool {
	return r.TryAcquirePermitsWithMaxWait(1, maxWaitTime)
}

func (r *rateLimiter[R]) TryAcquirePermitsWithMaxWait(requestedPermits int, maxWaitTime time.Duration) bool {
	waitTime := r.stats.acquirePermits(requestedPermits, maxWaitTime)
	if waitTime == -1 {
		return false
	}
	time.Sleep(waitTime)
	return true
}

func (r *rateLimiter[R]) TryReservePermit(maxWaitTime time.Duration) time.Duration {
	return r.TryReservePermits(1, maxWaitTime)
}

func (r *rateLimiter[R]) TryReservePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
	return r.stats.acquirePermits(requestedPermits, maxWaitTime)
}

func (r *rateLimiter[R]) ToExecutor() failsafe.PolicyExecutor[R] {
	rle := rateLimiterExecutor[R]{
		BasePolicyExecutor: &failsafe.BasePolicyExecutor[R]{
			BaseListenablePolicy: r.config.BaseListenablePolicy,
		},
		rateLimiter: r,
	}
	rle.PolicyExecutor = &rle
	return &rle
}
