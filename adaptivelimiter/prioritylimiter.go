package adaptivelimiter

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

type key int

// PriorityKey is a key to use with a Context that stores the priority value.
const PriorityKey key = 0

type PriorityLimiter[R any] interface {
	failsafe.Policy[R]
	AdaptiveLimiterInfo[R]

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
	prioritizer Prioritizer[R]
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

	// Track request flow
	inCount      atomic.Uint32
	outCount     atomic.Uint32
	blockedCount atomic.Int32

	mu sync.Mutex
}

func (l *priorityBlockingLimiter[R]) AcquirePermit(ctx context.Context, priority Priority) (Permit, error) {
	// Reject if priority is higher (lower importance) than threshold
	if priority < l.prioritizer.CurrentPriority() {
		return nil, ErrExceeded
	}

	l.inCount.Add(1)

	// Try without waiting first
	if permit, ok := l.adaptiveLimiter.TryAcquirePermit(); ok {
		l.outCount.Add(1)
		return permit, nil
	}

	// Always reject if over maxExecutionTime
	estimatedLatency := l.estimateLatency()
	if estimatedLatency > l.adaptiveLimiter.maxExecutionTime {
		return nil, ErrExceeded
	}

	// Block waiting for permit
	l.blockedCount.Add(1)
	permit, err := l.adaptiveLimiter.AcquirePermit(ctx)
	if err != nil {
		l.blockedCount.Add(-1)
		return nil, err
	}

	l.outCount.Add(1)
	l.blockedCount.Add(-1)

	return permit, nil
}

func (l *priorityBlockingLimiter[R]) Blocked() int {
	return int(l.blockedCount.Load())
}

// estimateLatency estimates wait time for a new request based on current conditions
func (l *priorityBlockingLimiter[R]) estimateLatency() time.Duration {
	avgProcessing := time.Duration(l.longRTT.Value() * float64(time.Millisecond))
	if avgProcessing == 0 {
		avgProcessing = l.adaptiveLimiter.maxExecutionTime / warmupSamples
	}

	totalRequests := int(l.blockedCount.Load()) + 1
	concurrency := l.Limit()
	fullBatches := totalRequests / concurrency
	if totalRequests%concurrency > 0 {
		fullBatches++
	}

	return time.Duration(float64(fullBatches) * float64(avgProcessing))
}

func (l *priorityBlockingLimiter[R]) ToExecutor(_ R) any {
	e := &priorityExecutor[R]{
		BaseExecutor:            &policy.BaseExecutor[R]{},
		priorityBlockingLimiter: l,
	}
	e.Executor = e
	return e
}
