package adaptivelimiter

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

type key int

// PriorityKey is a key to use with a Context that stores the priority value.
const PriorityKey key = 0

// PriorityLimiter is an adaptive concurrency limiter that can prioritize request rejections via a Prioritizer.
type PriorityLimiter[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit, potentially blocking up to maxExecutionTime.
	// The request priority must be less than the current priority threshold for admission.
	AcquirePermit(ctx context.Context, priority Priority) (Permit, error)

	// CanAcquirePermit returns whether it's currently possible to acquire a permit for the priority.
	CanAcquirePermit(priority Priority) bool
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
	name        string
}

func (c *priorityConfig[R]) Build() PriorityLimiter[R] {
	limiter := &priorityLimiter[R]{
		adaptiveLimiter: c.config.Build().(*adaptiveLimiter[R]),
		priorityConfig:  c,
		calibrations: &pidCalibrationWindow{
			window: make([]pidCalibrationPeriod, 30),
		},
	}
	c.prioritizer.register(limiter)
	return limiter
}

type priorityLimiter[R any] struct {
	*adaptiveLimiter[R]
	*priorityConfig[R]

	inCount      atomic.Uint32 // Requests received in current calibration period
	outCount     atomic.Uint32 // Requests permitted in current calibration period
	mu           sync.Mutex
	calibrations *pidCalibrationWindow // Guaded by mu
}

func (l *priorityLimiter[R]) AcquirePermit(ctx context.Context, priority Priority) (Permit, error) {
	// Generate a granular priority for the request and compare it to the prioritizer threshold
	granularPriority := generateGranularPriority(priority)
	if granularPriority < l.prioritizer.threshold() {
		return nil, ErrExceeded
	}

	// Maintain "queue" stats
	l.inCount.Add(1)
	defer l.outCount.Add(1)

	// Try without waiting first
	if permit, ok := l.adaptiveLimiter.TryAcquirePermit(); ok {
		return permit, nil
	}

	l.prioritizer.recordPriority(granularPriority)
	return l.adaptiveLimiter.AcquirePermit(ctx)
}

func (l *priorityLimiter[R]) CanAcquirePermit(priority Priority) bool {
	return generateGranularPriority(priority) >= l.prioritizer.threshold()
}

func (l *priorityLimiter[R]) getAndResetStats() (int, int) {
	return int(l.inCount.Swap(0)), int(l.outCount.Swap(0))
}

func (l *priorityLimiter[R]) name() string {
	return l.priorityConfig.name
}

func (l *priorityLimiter[R]) ToExecutor(_ R) any {
	e := &priorityLimiterExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		priorityLimiter: l,
	}
	e.Executor = e
	return e
}
