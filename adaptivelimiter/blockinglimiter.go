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

	rejectionRate := l.computeRejectionRate()
	if rejectionRate == 0 {
		return true
	}
	if rejectionRate >= 1 || rejectionRate >= rand.Float64() {
		return false
	}
	return true
}

func (l *blockingLimiter[R]) computeRejectionRate() float64 {
	_, blocked, rejectionThreshold, maxQueueSize := l.queueStats()
	return computeRejectionRate(blocked, rejectionThreshold, maxQueueSize)
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

func (l *blockingLimiter[R]) ToExecutor(_ R) any {
	e := &blockingLimiterExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		blockingLimiter: l,
	}
	e.Executor = e
	return e
}
