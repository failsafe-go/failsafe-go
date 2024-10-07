package adaptivelimiter

import (
	"errors"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds the current limit.
var ErrExceeded = errors.New("limit exceeded")

type Permit interface {
	RecordSuccess()

	RecordFailure()

	// ReleasePermit releases an execution permit back to the AdaptiveLimiter.
	ReleasePermit()
}

type AdaptiveLimiter[R any] interface {
	failsafe.Policy[R]

	// AcquirePermit attempts to acquire a permit to perform an execution against within the AdaptiveLimiter and returns
	// ErrExceeded if no permits are available.
	// Callers should call RecordSuccess, RecordFailure, or ReleasePermit to release a successfully acquired permit back to the AdaptiveLimiter.
	AcquirePermit() error

	// TryAcquirePermit attempts to acquire a permit to perform an execution against within the AdaptiveLimiter,
	// returning whether the permit was acquired or not.
	// Callers should call RecordSuccess, RecordFailure, or ReleasePermit to release a successfully acquired permit back to the AdaptiveLimiter.
	TryAcquirePermit() bool
}

// LimitChangedEvent indicates a AdaptiveLimiter's limit has changed.
type LimitChangedEvent struct {
}

/*
Builder builds AdaptiveLimiter instances.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	WithInitialLimit(initialLimit int) Builder[R]

	WithMinLimit(minLimit int) Builder[R]

	WithMaxConcurrency(maxLimit int) Builder[R]

	WithQueueSizeFunc(fn func(limit int) (queueSize int)) Builder[R]

	// TODO call this RTT increase factor threshold?
	WithRTTolerance(rttTolerance float64) Builder[R]

	WithSmoothing(smoothing float64) Builder[R]

	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]

	// Build returns a new AdaptiveLimiter using the builder's configuration.
	Build() AdaptiveLimiter[R]
}

type config[R any] struct {
	initialLimit int
	minLimit     int
	maxLimit     int
	smoothing    float64
	queueSizeFn  func(int) int
	longWindow   int
	rttTolerance float64

	limitChangedListener func(LimitChangedEvent)
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		initialLimit: 20,
		minLimit:     20,
		maxLimit:     200,
		smoothing:    0.2,
		queueSizeFn:  func(int) int { return 4 },
		longWindow:   600,
		rttTolerance: 1.5,
	}
}

func (c *config[R]) WithInitialLimit(initialLimit int) Builder[R] {
	c.initialLimit = initialLimit
	return c
}

func (c *config[R]) WithMinLimit(minLimit int) Builder[R] {
	c.minLimit = minLimit
	return c
}

func (c *config[R]) WithMaxConcurrency(maxLimit int) Builder[R] {
	c.maxLimit = maxLimit
	return c
}

func (c *config[R]) WithQueueSizeFunc(fn func(limit int) (queueSize int)) Builder[R] {
	c.queueSizeFn = fn
	return c
}

func (c *config[R]) WithRTTolerance(rttTolerance float64) Builder[R] {
	if rttTolerance < 1 {
		c.rttTolerance = 1
	}
	c.rttTolerance = rttTolerance
	return c
}

func (c *config[R]) WithSmoothing(smoothing float64) Builder[R] {
	c.smoothing = smoothing
	return c
}

func (c *config[R]) OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R] {
	c.limitChangedListener = listener
	return c
}

func (c *config[R]) Build() AdaptiveLimiter[R] {
	return &adaptiveLimiter[R]{
		config: c,
	}
}

// SampleListener defines a simple interface to add samples
// type SampleListener interface {
//	AddSample(float64)
// }

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	mtx      sync.Mutex
	inflight int
	limit    int
	lastRtt  int64
	longRtt  MovingAverage
	window   SampleWindow
}

func (l *adaptiveLimiter[R]) Update(startTime, rtt int64, inflight int, didDrop bool) int {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	l.lastRtt = rtt
	shortRtt := float64(rtt)
	longRtt := l.longRtt.Add(float64(rtt))
	queueSize := float64(l.queueSizeFn(l.limit))

	// If the long RTT is substantially larger than the short RTT then reduce the long RTT measurement.
	// This can happen when latency returns to normal after a prolonged prior of excessive load. Reducing the
	// long RTT without waiting for the exponential smoothing helps bring the system back to steady state.
	if longRtt/shortRtt > 2 {
		// TODO this might not be a good idea if a single shortRTT value ends up bumping the longRTT down
		l.longRtt.Set(longRtt * 0.95)
	}

	// Don't grow the limit if we are app limited
	if inflight < l.limit/2 {
		return l.limit
	}

	// Rtt could be higher than rtt_noload because of smoothing rtt noload updates
	// so set to 1.0 to indicate no queuing. Otherwise calculate the slope and don't
	// allow it to be reduced by more than half to avoid aggressive load-shedding due to
	// outliers.
	gradient := max(0.5, min(1.0, l.rttTolerance*longRtt/shortRtt))
	newLimit := float64(l.limit)*gradient + queueSize
	newLimit = float64(l.limit)*(1-l.smoothing) + newLimit*l.smoothing
	newLimit = max(float64(l.minLimit), min(float64(l.maxLimit), newLimit))

	l.limit = int(newLimit)
	return l.limit
}

func (l *adaptiveLimiter[R]) AcquirePermit() error {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if l.inflight >= l.limit {
		return ErrExceeded
	}
	l.inflight++
	return nil
}

func (l *adaptiveLimiter[R]) TryAcquirePermit() bool {
	return l.AcquirePermit() != nil
}

// func (l *adaptiveLimiter[R]) lastRTT(units time.Duration) int64 {
// 	return int64(units) * l.lastRtt / int64(time.Nanosecond)
// }
//
// func (l *adaptiveLimiter[R]) longRTT(units time.Duration) int64 {
// 	return int64(units) * int64(l.longRtt.Value()) / int64(time.Nanosecond)
// }
//
// func (l *adaptiveLimiter[R]) Limit() int {
// 	l.mtx.Lock()
// 	defer l.mtx.Unlock()
// 	return l.limit
// }

func (l *adaptiveLimiter[R]) ToExecutor(_ R) any {
	be := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{},
	}
	be.Executor = be
	return be
}

// func (l *adaptiveLimiter[R]) String() string {
// 	return fmt.Sprintf("AdaptiveLimiter [limit=%d]", l.limit)
// }

type permit[R any] struct {
	limiter   *adaptiveLimiter[R]
	startTime int64
}

func (p *permit[R]) RecordSuccess() {
	p.limiter.mtx.Lock()
	defer p.limiter.mtx.Unlock()
	p.limiter.inflight--
	endTime := time.Now().UnixNano()
	rtt := endTime - p.startTime

}

func (p *permit[R]) RecordFailure() {
	p.limiter.mtx.Lock()
	defer p.limiter.mtx.Unlock()
	p.limiter.inflight--
}

func (p *permit[R]) ReleasePermit() {
	p.limiter.mtx.Lock()
	defer p.limiter.mtx.Unlock()
	p.limiter.inflight--
}
