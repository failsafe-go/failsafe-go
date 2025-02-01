package adaptivelimiter

import (
	"context"
	"math/rand"

	"github.com/failsafe-go/failsafe-go/policy"
)

// blockingLimiter wraps an adaptiveLimiter and blocks some portion of requests when the adaptiveLimiter is at its
// limit.
type blockingLimiter[R any] struct {
	*adaptiveLimiter[R]

	// Mutable state
	rejectionRate float64 // Guarded by mu
}

func (l *blockingLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	if !l.CanAcquirePermit() {
		return nil, ErrExceeded
	}

	// Acquire a permit, blocking if needed
	return l.adaptiveLimiter.AcquirePermit(ctx)
}

func (l *blockingLimiter[R]) CanAcquirePermit() bool {
	if l.adaptiveLimiter.CanAcquirePermit() {
		return true
	}

	l.mu.Lock()
	rejectionThreshold := int(l.limit * l.initialRejectionFactor)
	maxQueueSize := int(l.limit * l.maxRejectionFactor)
	rejectionRate := computeRejectionRate(l.Blocked(), rejectionThreshold, maxQueueSize)
	l.rejectionRate = rejectionRate
	l.mu.Unlock()

	if rejectionRate == 0 {
		return true
	}
	if rejectionRate >= 1 || rejectionRate >= rand.Float64() {
		return false
	}
	return true
}

func computeRejectionRate(queueSize, rejectionThreshold, maxQueueSize int) float64 {
	if queueSize <= rejectionThreshold {
		return 0
	}
	if queueSize >= maxQueueSize {
		return 1
	}
	return float64(queueSize-rejectionThreshold) / float64(maxQueueSize-rejectionThreshold)
}

func (l *blockingLimiter[R]) RejectionRate() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rejectionRate
}

func (l *blockingLimiter[R]) ToExecutor(_ R) any {
	e := &blockingLimiterExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		blockingLimiter: l,
	}
	e.Executor = e
	return e
}
