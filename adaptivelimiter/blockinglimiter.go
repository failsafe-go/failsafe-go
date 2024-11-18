package adaptivelimiter

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

type BlockingLimiter[R any] interface {
	Limiter

	// RejectionRate returns the rate at which the limiter will reject attempts to acquire a permit when the
	// underlying limiter is full. This rate is adjusted regularly by the calibration loop. Returns a value between 0.0 and 1.0.
	RejectionRate() float64

	// ScheduleCalibrations schedules a calibration of the limiter's rejection rate every interval until the ctx is Done or
	// the resulting CancelFunc is called.
	ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc
}

type blockingLimiter[R any] struct {
	Limiter
	delegate AdaptiveLimiter[R]

	// PI parameters
	kp float64 // Proportional gain: responds to immediate load
	ki float64 // Integral gain: responds to sustained load over time

	// Mutable state
	inCount  atomic.Uint32 // Requests received in current calibration period
	outCount atomic.Uint32 // Requests permitted in current calibration period
	mu       sync.Mutex

	// Guarded by mu
	calibrations  *calibrationWindow
	rejectionRate float64
}

func NewBlockingLimiter[R any](delegate AdaptiveLimiter[R]) BlockingLimiter[R] {
	return &blockingLimiter[R]{
		delegate: delegate,
		calibrations: &calibrationWindow{
			window: make([]calibrationPeriod, 30),
		},

		// Using a small value (.1) results in a gradual response to spikes
		// If P(t)=0.5 (50% overload), this kp value adds 0.05 to the rejection rate
		kp: .1,

		// Using a large value (1.4) results in aggressive response to sustained load
		// If sum(P)=1.0, this ki value adds 1.4 to the rejection rate
		ki: 1.4,
	}
}

func (l *blockingLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	// Ensure ctx is not Done
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// See if execution is immediately permitted
	if permit, ok := l.delegate.TryAcquirePermit(); ok {
		return permit, nil
	}

	// If no permits are available, see if rejection rate is exceeded
	l.mu.Lock()
	rejectionRate := l.rejectionRate
	l.mu.Unlock()
	if rejectionRate > rand.Float64() {
		return nil, ErrExceeded
	}

	// Wait for a permit
	l.inCount.Add(1)
	permit, err := l.delegate.AcquirePermit(ctx)
	l.outCount.Add(1)
	return permit, err
}

func (l *blockingLimiter[R]) TryAcquirePermit() (Permit, bool) {
	return l.delegate.TryAcquirePermit()
}

func (l *blockingLimiter[R]) RejectionRate() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rejectionRate
}

func (l *blockingLimiter[R]) ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc {
	child, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-child.Done():
				return
			case <-ticker.C:
				l.calibrate()
			}
		}
	}()

	return cancel
}

// calibrate calibrates the limiter's rejection rate based on the queueing characteristics of incoming and outgoing
// executions.
func (l *blockingLimiter[R]) calibrate() {
	// Get and reset stats
	inCount := int(l.inCount.Swap(0))
	outCount := int(l.outCount.Swap(0))

	// Update calibrations and get latest
	limit := l.delegate.Limit()
	freeInflight := limit - l.delegate.Inflight()

	l.mu.Lock()
	defer l.mu.Unlock()
	pValue, integralSum := l.calibrations.add(inCount, outCount, freeInflight, limit)

	// Compute PI
	p := pValue
	i := integralSum
	adjustment := l.kp*p + l.ki*i

	// Update and clamp rejection rate
	l.rejectionRate = max(0, min(1, l.rejectionRate+adjustment))
}

type calibrationWindow struct {
	window      []calibrationPeriod
	size        int
	head        int
	integralSum float64 // Sum of P values over the window
}

type calibrationPeriod struct {
	inCount  int     // Items that entered the limiter during the calibration period
	outCount int     // Items that exited the limiter during the calibration period
	pValue   float64 // The computed P value for the calibration period
}

func (c *calibrationWindow) add(in, out, freeInflight int, limit int) (pValue float64, integralSum float64) {
	if c.size < len(c.window) {
		c.size++
	} else {
		c.integralSum -= c.window[c.head].pValue
	}

	pValue = computePValue(in, out, freeInflight, limit)
	c.integralSum += pValue
	c.window[c.head] = calibrationPeriod{
		inCount:  in,
		outCount: out,
		pValue:   pValue,
	}
	c.head = (c.head + 1) % len(c.window)
	return pValue, c.integralSum
}

func computePValue(in, out, freeInflight int, limit int) float64 {
	if out == 0 {
		return float64(limit)
	}
	return float64(in-out+freeInflight) / float64(out)
}
