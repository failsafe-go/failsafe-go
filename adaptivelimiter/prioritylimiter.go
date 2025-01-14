package adaptivelimiter

import (
	"context"
	"sync"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

type key int

// PriorityKey is a key to use with a Context that stores the priority value.
const PriorityKey key = 0

type PriorityLimiter[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit, potentially blocking up to maxExecutionTime.
	// The request priority must be less than the current priority threshold for admission.
	AcquirePermit(ctx context.Context, priority Priority) (Permit, error)
}

/*
PriorityLimiterBuilder builds PriorityLimiterBuilder instances.

This type is not concurrency safe.
*/
type PriorityLimiterBuilder[R any] interface {
	BaseBuilder[R]

	// Build returns a new PriorityLimiter using the builder's configuration.
	Build() PriorityLimiter[R]
}

type priorityConfig[R any] struct {
	*config[R]
	prioritizer Prioritizer
}

func (c *priorityConfig[R]) Build() PriorityLimiter[R] {
	limiter := &priorityBlockingLimiter[R]{
		adaptiveLimiter: c.config.Build().(*adaptiveLimiter[R]),
		priorityConfig:  c,
	}
	c.prioritizer.register(limiter)
	return limiter
}

type priorityBlockingLimiter[R any] struct {
	*adaptiveLimiter[R]
	*priorityConfig[R]

	mu sync.Mutex
}

func (l *priorityBlockingLimiter[R]) AcquirePermit(ctx context.Context, priority Priority) (Permit, error) {
	// Try without waiting first
	if permit, ok := l.adaptiveLimiter.TryAcquirePermit(); ok {
		return permit, nil
	}

	// Generate a granular priority for the request and threshold it against the prioritizer threshold
	granularPriority := generateGranularPriority(priority)
	l.prioritizer.recordPriority(granularPriority)
	if granularPriority < l.prioritizer.threshold() {
		return nil, ErrExceeded
	}

	return l.adaptiveLimiter.AcquirePermit(ctx)
}

func (l *priorityBlockingLimiter[R]) ToExecutor(_ R) any {
	e := &priorityExecutor[R]{
		BaseExecutor:            &policy.BaseExecutor[R]{},
		priorityBlockingLimiter: l,
	}
	e.Executor = e
	return e
}
