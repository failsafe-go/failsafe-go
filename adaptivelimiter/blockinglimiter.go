package adaptivelimiter

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

type BlockingLimiter[R any] interface {
	AdaptiveLimiter[R]

	// RejectionRate returns the rate at which the limiter will reject attempts to acquire a permit when the
	// underlying limiter is full. This rate is adjusted regularly by the calibration loop. Returns a value between 0.0 and 1.0.
	RejectionRate() float64

	// BlockedExecutions returns the number of currently blocked executions.
	BlockedExecutions() int

	// ScheduleCalibrations schedules a calibration of the limiter's rejection rate, which runs in a separate goroutine,
	// every interval until the ctx is Done or the resulting CancelFunc is called.
	ScheduleCalibrations(ctx context.Context, interval time.Duration) context.CancelFunc
}

/*
BlockingLimiterBuilder builds BlockingLimiter instances.

This type is not concurrency safe.
*/
type BlockingLimiterBuilder[R any] interface {
	WithLogger(logger *slog.Logger) BlockingLimiterBuilder[R]

	OnRejectionRateChanged(listener func(event RejectionRateChangedEvent)) BlockingLimiterBuilder[R]

	Build() BlockingLimiter[R]
}

// RejectionRateChangedEvent indicates a BlockingLimiter's limit has changed.
type RejectionRateChangedEvent struct {
	OldRate float64
	NewRate float64
}

type blockingConfig[R any] struct {
	delegate            AdaptiveLimiter[R]
	logger              *slog.Logger
	rateChangedListener func(event RejectionRateChangedEvent)
}

var _ BlockingLimiterBuilder[any] = &blockingConfig[any]{}

func NewBlockingLimiter[R any](delegate AdaptiveLimiter[R]) BlockingLimiter[R] {
	return NewBlockingLimiterBuilder(delegate).Build()
}

func NewBlockingLimiterBuilder[R any](delegate AdaptiveLimiter[R]) BlockingLimiterBuilder[R] {
	return &blockingConfig[R]{
		delegate: delegate,
	}
}

func (c *blockingConfig[R]) WithLogger(logger *slog.Logger) BlockingLimiterBuilder[R] {
	c.logger = logger
	return c
}

func (c *blockingConfig[R]) OnRejectionRateChanged(listener func(event RejectionRateChangedEvent)) BlockingLimiterBuilder[R] {
	c.rateChangedListener = listener
	return c
}

func (c *blockingConfig[R]) Build() BlockingLimiter[R] {
	return &blockingLimiter[R]{
		blockingConfig: c,
		calibrations: &calibrationWindow{
			window:       make([]calibrationPeriod, 30),
			integralEWMA: util.NewEWMA(30, 5), // 30 sample window, 5 warmup samples
		},

		// Using a small value (.1) results in a gradual response to spikes
		// If P(t)=0.5 (50% overload), this kp value adds 0.05 to the rejection rate
		kp: .1,

		// Using a large value (1.4) results in aggressive response to sustained load
		// If sum(P)=1.0, this ki value adds 1.4 to the rejection rate
		ki: 1.4,
	}
}

type blockingLimiter[R any] struct {
	*blockingConfig[R]

	// PI parameters
	kp float64 // Proportional gain: responds to immediate load
	ki float64 // Integral gain: responds to sustained load over time

	// Mutable state
	inCount  atomic.Uint32 // Requests received in current calibration period
	outCount atomic.Uint32 // Requests permitted in current calibration period
	blocked  atomic.Int32  // Requests received in current calibration period
	mu       sync.Mutex

	// Guarded by mu
	calibrations  *calibrationWindow
	rejectionRate float64
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
		l.inCount.Add(1)
		l.outCount.Add(1)
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
	l.blocked.Add(1)
	permit, err := l.delegate.AcquirePermit(ctx)
	l.outCount.Add(1)
	l.blocked.Add(-1)
	return permit, err
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

func (l *blockingLimiter[R]) RejectionRate() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rejectionRate
}

func (l *blockingLimiter[R]) BlockedExecutions() int {
	return int(l.blocked.Load())
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

// calibrate calibrates the limiter's rejection rate based on proportional (P) and integral (I) values that are tracked
// from previous executions. The P value represents instantaneous load, and the I value represents historical load.
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
	newRate := max(0, min(1, l.rejectionRate+adjustment))
	if l.rejectionRate != newRate {
		if l.rateChangedListener != nil {
			l.rateChangedListener(RejectionRateChangedEvent{
				OldRate: l.rejectionRate,
				NewRate: newRate,
			})
		}
	}
	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		l.logger.Debug(fmt.Sprintf("newRejectionRate=%0.2f, oldRejectionRate=%0.2f, in=%d, out=%d, blocked=%d, p=%0.2f, i=%0.2f, adjustment=%0.2f", l.rejectionRate, newRate, inCount, outCount, l.blocked.Load(), p, i, adjustment))
	}
	l.rejectionRate = newRate
}

func (l *blockingLimiter[R]) ToExecutor(_ R) any {
	e := &blockingExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		blockingLimiter: l,
	}
	e.Executor = e
	return e
}

type calibrationWindow struct {
	window []calibrationPeriod
	size   int
	head   int
	// integralSum float64 // Sum of P values over the window
	integralEWMA util.MovingAverage
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
		// c.integralSum -= c.window[c.head].pValue
	}

	pValue = computePValue(in, out, freeInflight, limit)
	integralSum = c.integralEWMA.Add(pValue)
	// c.integralSum += pValue
	c.window[c.head] = calibrationPeriod{
		inCount:  in,
		outCount: out,
		pValue:   pValue,
	}
	c.head = (c.head + 1) % len(c.window)
	return pValue, integralSum
}

// Computes a P value for a calibration period.
// A positive P value indicates overloaded. A negative P value indicates underloaded.
func computePValue(in, out, freeInflight int, limit int) float64 {
	if out == 0 {
		return float64(limit)
	}
	return float64(in-(out+freeInflight)) / float64(out)
}
