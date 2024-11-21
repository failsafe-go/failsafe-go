package adaptivelimiter

import (
	"context"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go/policy"
)

const warmupSamples = 10

// BlockingLimiter wraps an AdaptiveLimiter and blocks some portion of requests when the AdaptiveLimiter is at its
// limit. Requests block up to some max latency based on an estimated latency for incoming requests.
//
// Estimated latency considers the current number of blocked requests, the underlying AdaptiveLimiter's current limit,
// and the average request processing time.
type BlockingLimiter[R any] interface {
	AdaptiveLimiter[R]

	// BlockedExecutions returns the number of currently blocked executions.
	BlockedExecutions() int
}

/*
BlockingLimiterBuilder builds BlockingLimiter instances.

This type is not concurrency safe.
*/
type BlockingLimiterBuilder[R any] interface {
	Build() BlockingLimiter[R]
}

type blockingConfig[R any] struct {
	delegate   AdaptiveLimiter[R]
	maxLatency time.Duration
}

var _ BlockingLimiterBuilder[any] = &blockingConfig[any]{}

func NewBlockingLimiter[R any](delegate AdaptiveLimiter[R]) BlockingLimiter[R] {
	return NewBlockingLimiterBuilder(delegate).Build()
}

func NewBlockingLimiterBuilder[R any](delegate AdaptiveLimiter[R]) BlockingLimiterBuilder[R] {
	return &blockingConfig[R]{
		delegate:   delegate,
		maxLatency: 10 * time.Second,
	}
}

func (c *blockingConfig[R]) Build() BlockingLimiter[R] {
	return &blockingLimiter[R]{
		blockingConfig: c,
	}
}

type blockingLimiter[R any] struct {
	*blockingConfig[R]
	mu sync.Mutex

	// Guarded by mu
	blockedCount int
}

func (l *blockingLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	// Try to get a permit without waiting
	if permit, ok := l.delegate.TryAcquirePermit(); ok {
		return permit, nil
	}

	// Estimate if the queue is full
	l.mu.Lock()
	estimatedLatency := l.estimateLatency()
	if estimatedLatency > l.maxLatency {
		l.mu.Unlock()
		return nil, ErrExceeded
	}

	// Acquire a permit, blocking if needed
	l.blockedCount++
	l.mu.Unlock()
	permit, err := l.delegate.AcquirePermit(ctx)
	l.mu.Lock()
	l.blockedCount--
	l.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return permit, nil
}

// estimateLatency estimates the latency for a new request by considering the delegate's limit, how many batches of
// blocked requests it would take before a new request is serviced, and the average processing time per request.
func (l *blockingLimiter[R]) estimateLatency() time.Duration {
	delegateLimiter := l.delegate.(*adaptiveLimiter[R])
	avgProcessing := time.Duration(delegateLimiter.longRTT.Value())
	// avgProcessing := time.Duration(l.processingTime.Value())
	if avgProcessing == 0 {
		avgProcessing = l.maxLatency / warmupSamples
	}

	// Include current request in the latency estimate
	totalRequests := l.blockedCount + 1

	// Calculate complete batches needed
	concurrency := l.delegate.Limit()
	fullBatches := totalRequests / concurrency

	// If we have any remaining requests, count it as a full batch
	if totalRequests%concurrency > 0 {
		fullBatches++
	}

	return time.Duration(float64(fullBatches) * float64(avgProcessing))
}

func (l *blockingLimiter[R]) TryAcquirePermit() (Permit, bool) {
	return l.delegate.TryAcquirePermit()
}

func (l *blockingLimiter[R]) Limit() int {
	return l.delegate.Limit()
}

func (l *blockingLimiter[R]) Inflight() int {
	return l.delegate.Inflight()
}

func (l *blockingLimiter[R]) BlockedExecutions() int {
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
