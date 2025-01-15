package adaptivelimiter

import (
	"context"
	"math/rand"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// LatencyLimiter is an adaptive concurrency limiter that can limit the latency of blocked requests by rejecting
// requests once blocking exceeds the configed thresholds.
type LatencyLimiter[R any] interface {
	failsafe.Policy[R]
	Metrics

	// RejectionRate returns the current rate, from 0 to 1, at which the limiter will reject requests, based on recent
	// execution times.
	RejectionRate() float64
}

/*
LatencyLimiterBuilder builds LatencyLimiter instances.

This type is not concurrency safe.
*/
type LatencyLimiterBuilder[R any] interface {
	BaseBuilder[R]

	// Build returns a new LatencyLimiter using the builder's configuration.
	Build() LatencyLimiter[R]
}

type latencyLimiterConfig[R any] struct {
	*config[R]
	rejectionThreshold time.Duration
	maxExecutionTime   time.Duration
}

func (c *latencyLimiterConfig[R]) Build() LatencyLimiter[R] {
	return &latencyLimiter[R]{
		adaptiveLimiter:      c.config.Build().(*adaptiveLimiter[R]),
		latencyLimiterConfig: c,
	}
}

// latencyLimiter wraps an adaptiveLimiter and blocks some portion of requests when the adaptiveLimiter is at its
// limit.
type latencyLimiter[R any] struct {
	*adaptiveLimiter[R]
	*latencyLimiterConfig[R]

	rejectionRate float64 // Guarded by mu
}

func (l *latencyLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	// Try to get a permit without waiting
	if permit, ok := l.TryAcquirePermit(); ok {
		return permit, nil
	}

	// Estimate if the blocking limiter is at capacity
	rttEstimate := l.estimateRTT()
	if rttEstimate > l.maxExecutionTime {
		return nil, ErrExceeded
	}

	rejectionRate := computeRejectionRate(rttEstimate, l.rejectionThreshold, l.maxExecutionTime)
	l.mu.Lock()
	l.rejectionRate = rejectionRate
	l.mu.Unlock()

	// Reject requests based on the rejection rate
	if rejectionRate >= 1 || rejectionRate >= rand.Float64() {
		return nil, ErrExceeded
	}

	// Acquire a permit, blocking if needed
	l.blockedCount.Add(1)
	permit, err := l.adaptiveLimiter.AcquirePermit(ctx)
	l.blockedCount.Add(-1)

	if err != nil {
		return nil, err
	}

	return permit, nil
}

func (l *latencyLimiter[R]) CanAcquirePermit() bool {
	if l.semaphore.IsFull() {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.rejectionRate >= 1 || l.rejectionRate >= rand.Float64()
	}
	return true
}

func (l *latencyLimiter[R]) RejectionRate() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rejectionRate
}

func computeRejectionRate(rtt, rejectionThreshold, maxExecutionTime time.Duration) float64 {
	if maxExecutionTime <= rejectionThreshold {
		return 1
	}
	return max(0, min(1, float64(rtt-rejectionThreshold)/float64(maxExecutionTime-rejectionThreshold)))
}

func (l *latencyLimiter[R]) ToExecutor(_ R) any {
	e := &latencyLimiterExecutor[R]{
		BaseExecutor:   &policy.BaseExecutor[R]{},
		latencyLimiter: l,
	}
	e.Executor = e
	return e
}
