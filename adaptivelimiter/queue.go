package adaptivelimiter

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

type QueueingLimiter[R any] struct {
	delegate AdaptiveLimiter[R]

	// PI parameters
	kp float64 // Proportional gain: responds to immediate load
	ki float64 // Integral gain: responds to sustained load over time

	// Mutable state
	mtx            sync.Mutex         // Guarded by mtx
	calibrations   *calibrationWindow // Guarded by mtx
	rejectionRatio float64            // Guarded by mtx
	inCount        atomic.Uint32      // Requests received in current calibration period
	outCount       atomic.Uint32      // Requests permitted in current calibration period
}

func NewQueueingLimiter[R any](delegate AdaptiveLimiter[R]) *QueueingLimiter[R] {
	return &QueueingLimiter[R]{
		delegate: delegate,
		calibrations: &calibrationWindow{
			window: make([]calibrationPeriod, 30),
		},

		// Using a small value (.1) results in a gradual response to spikes
		// If P(t)=0.5 (50% overload), this kp value adds 0.05 to the rejection ratio
		kp: .1,

		// Using a large value (1.4) results in aggressive response to sustained load
		// If sum(P)=1.0, this ki value adds 1.4 to the rejection ratio
		ki: 1.4,
	}
}

func (l *QueueingLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
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

	// See if rejection ratio is exceeded
	l.mtx.Lock()
	rejectionRatio := l.rejectionRatio
	l.mtx.Unlock()
	if rejectionRatio > rand.Float64() {
		return nil, ErrExceeded
	}

	// Wait for a permit
	l.inCount.Add(1)
	permit, err := l.delegate.AcquirePermit(ctx)
	l.outCount.Add(1)
	return permit, err
}

func (l *QueueingLimiter[R]) TryAcquirePermit(ctx context.Context) (Permit, bool) {
	p, err := l.AcquirePermit(ctx)
	return p, err == nil
}

// RunCalibrationLoop runs a blocking loop that calibrates the limiter every interval until the ctx is Done.
func (l *QueueingLimiter[R]) RunCalibrationLoop(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			l.calibrate()
		}
	}
}

// calibrate calibrates the limiter's rejection ratio based on previous calibration periods.
func (l *QueueingLimiter[R]) calibrate() {
	// Get and reset queue stats
	inCount := int(l.inCount.Swap(0))
	outCount := int(l.outCount.Swap(0))

	// Update calibrations and get latest
	limit := l.delegate.Limit()
	freeInflight := limit - l.delegate.Inflight()

	l.mtx.Lock()
	defer l.mtx.Unlock()
	pValue, integralSum := l.calibrations.add(inCount, outCount, freeInflight, limit)

	// Compute PI
	p := pValue
	i := integralSum
	adjustment := l.kp*p + l.ki*i

	// Update and clamp rejection ratio
	l.rejectionRatio = max(0, min(1, l.rejectionRatio+adjustment))
}

type calibrationWindow struct {
	window      []calibrationPeriod
	size        int
	head        int
	integralSum float64 // Sum of P values over the window
}

type calibrationPeriod struct {
	inCount  int     // Items that entered the queue during the calibration period
	outCount int     // Items that exited the queue during the calibration period
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
