package adaptivelimiter

import (
	"context"
	"math/rand"
	"sync/atomic"
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

	// // Calibrate calibrates the RejectionRate based on recent execution times from registered limiters.
	// Calibrate()
	//
	// // ScheduleCalibrations runs calibration on the interval until the ctx is done or the returned CancelFunc is called.
	// ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc
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
		// calibrations: &pidCalibrationWindow{
		// 	window: make([]pidCalibrationPeriod, 30),
		// 	// integralEWMA: util.NewEWMA(30, 5), // 30 sample window, 5 warmup samples
		// },
		// Using a small value (.1) results in a gradual response to spikes
		// If P(t)=0.5 (50% overload), this kp value adds 0.05 to the rejection rate
		kp: .1,

		// Using a large value (1.4) results in aggressive response to sustained load
		// If sum(P)=1.0, this ki value adds 1.4 to the rejection rate
		ki: 1.4,
	}
}

// latencyLimiter wraps an adaptiveLimiter and blocks some portion of requests when the adaptiveLimiter is at its
// limit.
type latencyLimiter[R any] struct {
	*adaptiveLimiter[R]
	*latencyLimiterConfig[R]

	// PI parameters
	kp float64 // Proportional gain: responds to recent load
	ki float64 // Integral gain: responds to longer term load

	// Mutable state
	inCount  atomic.Uint32 // Requests received in current calibration period
	outCount atomic.Uint32 // Requests permitted in current calibration period
	// calibrations  *pidCalibrationWindow
	rejectionRate float64 // Guarded by mu
}

func (l *latencyLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	// Try to get a permit without waiting
	if permit, ok := l.adaptiveLimiter.TryAcquirePermit(); ok {
		l.inCount.Add(1)
		l.outCount.Add(1)
		return permit, nil
	}

	if l.isQueueFull() {
		return nil, ErrExceeded
	}

	// Acquire a permit, blocking if needed
	l.inCount.Add(1)
	permit, err := l.adaptiveLimiter.AcquirePermit(ctx)
	l.outCount.Add(1)
	if err != nil {
		return nil, err
	}

	return permit, nil
}

func (l *latencyLimiter[R]) CanAcquirePermit() bool {
	return !l.semaphore.IsFull() || !l.isQueueFull()
}

// Returns whether the "queue" that forms when the limiter is full is also considered full, based on the current
// rejection rate for a new execution.
func (l *latencyLimiter[R]) isQueueFull() bool {
	l.mu.Lock()
	rejectionRate := l.rejectionRate
	l.mu.Unlock()

	// Reject requests based on the rejection rate
	if rejectionRate >= 1 || rejectionRate >= rand.Float64() {
		return true
	}

	return false
}

func (l *latencyLimiter[R]) RejectionRate() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rejectionRate
}

func (l *latencyLimiter[R]) ToExecutor(_ R) any {
	e := &latencyLimiterExecutor[R]{
		BaseExecutor:   &policy.BaseExecutor[R]{},
		latencyLimiter: l,
	}
	e.Executor = e
	return e
}
