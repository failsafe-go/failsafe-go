package ratelimiter

import (
	"context"
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/spi"
)

// ErrRateLimitExceeded is returned when an execution exceeds a configured rate limit.
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

/*
RateLimiter is a Policy that can control the rate of executions as a way of preventing system overload.

There are two types of rate limiting: smooth and bursty. Smooth rate limiting will evenly spread out execution requests
over-time, effectively smoothing out uneven execution request rates. Bursty rate limiting allows potential bursts of
executions to occur, up to a configured max per time period.

Rate limiting is based on permits, which can be requested in order to perform rate limited execution. Permits are
automatically refreshed over time based on the rate limiter's configuration.

This type provides methods that block while waiting for permits to become available, and also methods that return
immediately. The blocking methods include:

  - AcquirePermit
  - AcquirePermits
  - AcquirePermitWithMaxWait
  - AcquirePermitsWithMaxWait

The methods that return immediately include:

  - TryAcquirePermit
  - TryAcquirePermits
  - ReservePermit
  - ReservePermits
  - TryReservePermit
  - TryReservePermits

This type provides methods that return ErrRateLimitExceeded when permits cannot be acquired, and also methods that
return a bool. The Acquire methods all return ErrRateLimitExceeded when permits cannot be acquired, and the TryAcquire
methods return a boolean.

The ReservePermit methods attempt to reserve permits and return an expected wait time before the permit can be used.
This helps integrate with scenarios where you need to wait externally.

This type is concurrency safe.
*/
type RateLimiter[R any] interface {
	failsafe.Policy[R]

	// AcquirePermit attempts to acquire a permit to perform an execution against the rate limiter, waiting until one is
	// available or the ctx is canceled. Returns an error if the ctx is canceled.
	//
	// ctx may be nil.
	AcquirePermit(ctx context.Context) error

	// AcquirePermits attempts to acquire the requested permits to perform executions against the rate limiter, waiting until
	// they are available or the ctx is canceled. Returns an error if the ctx is canceled.
	//
	// ctx may be nil.
	AcquirePermits(ctx context.Context, permits int) error

	// AcquirePermitWithMaxWait attempts to acquire a permit to perform an execution against the rate limiter, waiting up to
	// the maxWaitTime until one is available or the ctx is canceled. Returns ErrRateLimitExceeded if a permit would not be
	// available in time. Returns an error if the context is canceled.
	//
	// ctx may be nil.
	AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) error

	// AcquirePermitsWithMaxWait attempts to acquire the requested permits to perform executions against the rate limiter,
	// waiting up to the maxWaitTime until they are available or the ctx is canceled. Returns ErrRateLimitExceeded if the
	// permits would not be available in time. Returns an error if the context is canceled.
	//
	// ctx may be nil.
	AcquirePermitsWithMaxWait(ctx context.Context, requestedPermits int, maxWaitTime time.Duration) error

	// ReservePermit reserves a permit to perform an execution against the rate limiter, and returns the time that the caller
	// is expected to wait before acting on the permit. Returns 0 if the permit is immediately available and no waiting is
	// needed.
	ReservePermit() time.Duration

	// ReservePermits reserves the permits to perform executions against the rate limiter, and returns the time that the
	// caller is expected to wait before acting on the permits. Returns 0 if the permits are immediately available and no
	// waiting is needed.
	ReservePermits(permits int) time.Duration

	// TryAcquirePermit tries to acquire a permit to perform an execution against the rate limiter, returning immediately
	// without waiting.
	TryAcquirePermit() bool

	// TryAcquirePermits tries to acquire the requested permits to perform executions against the rate limiter, returning
	// immediately without waiting. Returns true if the permit was successfull acquired, else false.
	TryAcquirePermits(permits int) bool

	// TryReservePermit tries to reserve a permit to perform an execution against the rate limiter, and returns the time that
	// the caller is expected to wait before acting on the permit, as long as it's less than the maxWaitTime.
	//
	//  - Returns the expected wait time for the permit if it was successfully reserved.
	//  - Returns 0 if the permit was successfully reserved and no waiting is needed.
	//  - Returns -1 if the permit was not reserved because the wait time would be greater than the maxWaitTime.
	TryReservePermit(maxWaitTime time.Duration) time.Duration

	// TryReservePermits tries to reserve the permits to perform executions against the rate limiter, and returns the time
	// that the caller is expected to wait before acting on the permits, as long as it's less than the maxWaitTime.
	//
	//  - Returns the expected wait time for the permit if it was successfully reserved.
	//  - Returns 0 if the permit was successfully reserved and no waiting is needed.
	//  - Returns -1 if the permit was not reserved because the wait time would be greater than the maxWaitTime.
	TryReservePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration
}

/*
RateLimiterBuilder builds RateLimiter instances.

This type is not concurrency safe.
*/
type RateLimiterBuilder[R any] interface {
	// WithMaxWaitTime configures the maxWaitTime to wait for permits to be available. If permits cannot be acquired before
	// the maxWaitTime is exceeded, then the rate limiter will return ErrRateLimitExceeded.
	//
	// This setting only applies when the resulting RateLimiter is used with the failsafe.Run or related APIs. It does not
	// apply when the RateLimiter is used in a standalone way.
	WithMaxWaitTime(maxWaitTime time.Duration) RateLimiterBuilder[R]

	// OnRateLimitExceeded registers the listener to be called when the rate limit is exceeded.
	OnRateLimitExceeded(listener func(event failsafe.ExecutionCompletedEvent[R])) RateLimiterBuilder[R]

	// Build returns a new RateLimiter using the builder's configuration.
	Build() RateLimiter[R]
}

type rateLimiterConfig[R any] struct {
	// Common
	maxWaitTime         time.Duration
	onRateLimitExceeded func(failsafe.ExecutionCompletedEvent[R])

	// Smooth
	interval time.Duration

	// Bursty
	periodPermits int
	period        time.Duration
}

/*
SmoothBuilder returns a smooth RateLimiterBuilder for execution result type R and the maxExecutions and period, which
control how frequently an execution is permitted. The individual execution rate is computed as period / maxExecutions.
For example, with maxExecutions of 100 and a period of 1000 millis, individual executions will be permitted at a max
rate of one every 10 millis.

By default, the returned RateLimiterBuilder will have a max wait time of 0.

Executions are performed with no delay until they exceed the max rate, after which executions are either rejected or
will block and wait until the max wait time is exceeded.
*/
func SmoothBuilder[R any](maxExecutions int64, period time.Duration) RateLimiterBuilder[R] {
	return &rateLimiterConfig[R]{
		interval: period / time.Duration(maxExecutions),
	}
}

/*
SmoothBuilderWithMaxRate returns a smooth RateLimiterBuilder for execution result type R and the maxRate, which controls
how frequently an execution is permitted. For example, a maxRate of 10 millis would allow up to one execution every 10
millis.

By default, the returned RateLimiterBuilder will have a max wait time of 0.

Executions are performed with no delay until they exceed the maxRate, after which executions are either rejected or will
block and wait until the max wait time is exceeded.
*/
func SmoothBuilderWithMaxRate[R any](maxRate time.Duration) RateLimiterBuilder[R] {
	return &rateLimiterConfig[R]{
		interval: maxRate,
	}
}

/*
BurstyBuilder returns a bursty RateLimiterBuilder for execution result type R and the maxExecutions per period. For
example, a maxExecutions value of 100 with a period of 1 second would allow up to 100 executions every 1 second.

By default, the returned RateLimiterBuilder will have a max wait time of 0.

Executions are performed with no delay up until the maxExecutions are reached for the current period, after which
executions are either rejected or will block and wait until the max wait time is exceeded.
*/
func BurstyBuilder[R any](maxExecutions int, period time.Duration) RateLimiterBuilder[R] {
	return &rateLimiterConfig[R]{
		periodPermits: maxExecutions,
		period:        period,
	}
}

func (c *rateLimiterConfig[R]) WithMaxWaitTime(maxWaitTime time.Duration) RateLimiterBuilder[R] {
	c.maxWaitTime = maxWaitTime
	return c
}

func (c *rateLimiterConfig[R]) OnRateLimitExceeded(listener func(event failsafe.ExecutionCompletedEvent[R])) RateLimiterBuilder[R] {
	c.onRateLimitExceeded = listener
	return c
}

func (c *rateLimiterConfig[R]) Build() RateLimiter[R] {
	if c.interval != 0 {
		return &rateLimiter[R]{
			config: c,
			stats: &smoothRateLimiterStats[R]{
				config:    c, // TODO copy base fields
				stopwatch: util.NewStopwatch(),
			},
		}
	}
	return &rateLimiter[R]{
		config: c,
		stats: &burstyRateLimiterStats[R]{
			config:           c, // TODO copy base fields
			stopwatch:        util.NewStopwatch(),
			availablePermits: c.periodPermits,
		},
	}
}

type rateLimiter[R any] struct {
	config *rateLimiterConfig[R]
	stats  rateLimiterStats
}

func (r *rateLimiter[R]) AcquirePermit(ctx context.Context) error {
	return r.AcquirePermits(ctx, 1)
}

func (r *rateLimiter[R]) AcquirePermits(ctx context.Context, permits int) error {
	waitTime := r.ReservePermits(permits)
	if ctx != nil {
		timer := time.NewTimer(waitTime)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	} else {
		time.Sleep(waitTime)
	}
	return nil
}

func (r *rateLimiter[R]) AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) error {
	return r.acquirePermitsWithMaxWait(ctx, nil, 1, maxWaitTime)
}

func (r *rateLimiter[R]) AcquirePermitsWithMaxWait(ctx context.Context, requestedPermits int, maxWaitTime time.Duration) error {
	return r.acquirePermitsWithMaxWait(ctx, nil, requestedPermits, maxWaitTime)
}

func (r *rateLimiter[R]) acquirePermitsWithMaxWait(ctx context.Context, canceled <-chan any, requestedPermits int, maxWaitTime time.Duration) error {
	waitTime := r.stats.acquirePermits(requestedPermits, maxWaitTime)
	if waitTime == -1 {
		return ErrRateLimitExceeded
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(waitTime)
	select {
	case <-timer.C:
	case <-canceled:
		timer.Stop()
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
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

func (r *rateLimiter[R]) TryReservePermit(maxWaitTime time.Duration) time.Duration {
	return r.TryReservePermits(1, maxWaitTime)
}

func (r *rateLimiter[R]) TryReservePermits(requestedPermits int, maxWaitTime time.Duration) time.Duration {
	return r.stats.acquirePermits(requestedPermits, maxWaitTime)
}

func (r *rateLimiter[R]) ToExecutor(policyIndex int) any {
	rle := &rateLimiterExecutor[R]{
		BasePolicyExecutor: &spi.BasePolicyExecutor[R]{
			PolicyIndex: policyIndex,
		},
		rateLimiter: r,
	}
	rle.PolicyExecutor = rle
	return rle
}
