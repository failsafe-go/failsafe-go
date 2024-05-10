package loadlimiter

import (
	"errors"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds a configured load limit.
var ErrExceeded = errors.New("rate limit exceeded")

type LoadLimiter[R any] interface {
	failsafe.Policy[R]

	AcquirePermit() error

	TryAcquirePermit() bool
}

/*
LoadLimiterBuilder builds LoadLimiter instances.

This type is not concurrency safe.
*/
type LoadLimiterBuilder[R any] interface {
	// Build returns a new LoadLimiter using the builder's configuration.
	Build() LoadLimiter[R]
}

type loadLimiterConfig[R any] struct {
	signal   Signal
	strategy Strategy
}

func With[R any](signal Signal, strategy Strategy) LoadLimiter[R] {
	return Builder[R](signal, strategy).Build()
}

func Builder[R any](signal Signal, strategy Strategy) LoadLimiterBuilder[R] {
	return &loadLimiterConfig[R]{
		signal:   signal,
		strategy: strategy,
	}
}

func (c *loadLimiterConfig[R]) Build() LoadLimiter[R] {
	return &loadLimiter[R]{
		config: c,
	}
}

type loadLimiter[R any] struct {
	config *loadLimiterConfig[R]
}

func (r *loadLimiter[R]) AcquirePermit() error {
	return r.acquirePermit(nil)
}

func (r *loadLimiter[R]) TryAcquirePermit() bool {
	return r.acquirePermit(nil) == nil
}

func (r *loadLimiter[R]) acquirePermit(exec failsafe.Execution[R]) error {
	// waitTime := r.stats.acquirePermits(int(requestedPermits), maxWaitTime)
	// if waitTime == -1 {
	// 	return ErrExceeded
	// }
	return nil
}

func (r *loadLimiter[R]) ToExecutor(_ R) any {
	rle := &loadLimiterExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
		loadLimiter:  r,
	}
	rle.Executor = rle
	return rle
}
