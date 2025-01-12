package adaptivelimiterold

import (
	"context"
	"time"

	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/policy"
)

// blockingLimiter wraps an adaptiveLimiter and blocks some portion of requests when the adaptiveLimiter is at its
// limit.
type blockingLimiter[R any] struct {
	*adaptiveLimiter[R]

	// Guarded by mu
	blockedCount int
}

func (l *blockingLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	// Try to get a permit without waiting
	if permit, ok := l.TryAcquirePermit(); ok {
		return permit, nil
	}

	// Estimate if the blocking limiter is at capacity
	l.mu.Lock()
	if l.estimateLatency() > l.maxExecutionTime {
		l.mu.Unlock()
		return nil, adaptivelimiter.ErrExceeded
	}

	// Acquire a permit, blocking if needed
	l.blockedCount++
	l.mu.Unlock()
	permit, err := l.adaptiveLimiter.AcquirePermit(ctx)
	l.mu.Lock()
	l.blockedCount--
	l.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return permit, nil
}

func (l *blockingLimiter[R]) CanAcquirePermit() bool {
	if l.semaphore.IsFull() {
		l.mu.Lock()
		defer l.mu.Unlock()

		// Estimate if the blocking limiter is at capacity
		return l.estimateLatency() <= l.maxExecutionTime
	}
	return true
}

// estimateLatency estimates the latency for a new request by considering the delegate's limit, how many batches of
// blocked requests it would take before a new request is serviced, and the average processing time per request.
func (l *blockingLimiter[R]) estimateLatency() time.Duration {
	avgProcessing := time.Duration(l.longRTT.Value())
	if avgProcessing == 0 {
		avgProcessing = l.maxExecutionTime / warmupSamples
	}

	// Include current request in the latency estimate
	totalRequests := l.blockedCount + 1

	// Calculate complete batches needed
	concurrency := int(l.limit)
	fullBatches := totalRequests / concurrency

	// If we have any remaining requests, count it as a full batch
	if totalRequests%concurrency > 0 {
		fullBatches++
	}

	return time.Duration(float64(fullBatches) * float64(avgProcessing))
}

func (l *blockingLimiter[R]) Blocked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.blockedCount
}

func (l *blockingLimiter[R]) ToExecutor(_ R) any {
	e := &blockingExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		blockingLimiter: l,
	}
	e.Executor = e
	return e
}
