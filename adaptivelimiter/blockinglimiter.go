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
	if !l.semaphore.IsFull() {
		return true
	}

	l.mu.Lock()
	maxQueueSize := int(l.limit * l.maxBlockingFactor)
	rejectionRate := computeRejectionRate(l.Blocked(), maxQueueSize)
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

// Computes a rejection rate based on the current and max queue sizes.
func computeRejectionRate(queueSize, maxQueueSize int) float64 {
	excessRequests := queueSize - maxQueueSize
	return max(0, min(1, float64(excessRequests)/float64(maxQueueSize)))
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
