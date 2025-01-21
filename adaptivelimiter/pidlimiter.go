package adaptivelimiter

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/failsafe-go/failsafe-go/policy"
)

type pidLimiter[R any] struct {
	*adaptiveLimiter[R]
	*priorityConfig[R]

	// Mutable state
	inCount  atomic.Uint32 // Requests received in current calibration period
	outCount atomic.Uint32 // Requests permitted in current calibration period
	mu       sync.Mutex
}

func (l *pidLimiter[R]) AcquirePermit(ctx context.Context, priority Priority) (Permit, error) {
	// Try without waiting first
	if permit, ok := l.adaptiveLimiter.TryAcquirePermit(); ok {
		l.inCount.Add(1)
		l.outCount.Add(1)
		return permit, nil
	}

	// Generate a granular priority for the request and compare it to the prioritizer threshold
	granularPriority := generateGranularPriority(priority)
	l.prioritizer.recordPriority(granularPriority)
	if granularPriority < l.prioritizer.threshold() {
		return nil, ErrExceeded
	}

	l.inCount.Add(1)
	defer l.outCount.Add(1)
	return l.adaptiveLimiter.AcquirePermit(ctx)
}

func (l *pidLimiter[R]) CanAcquirePermit(priority Priority) bool {
	return generateGranularPriority(priority) >= l.prioritizer.threshold()
}

func (l *pidLimiter[R]) ToExecutor(_ R) any {
	e := &pidLimiterExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
		pidLimiter:   l,
	}
	e.Executor = e
	return e
}
