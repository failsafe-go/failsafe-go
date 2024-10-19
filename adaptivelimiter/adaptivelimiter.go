package adaptivelimiter

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds the current limit.
var ErrExceeded = errors.New("limit exceeded")

type Permit interface {
	RecordSuccess()

	RecordFailure()

	// Release releases an execution permit back to the AdaptiveLimiter.
	Release()
}

type AdaptiveLimiter[R any] interface {
	failsafe.Policy[R]

	// AcquirePermit attempts to acquire a permit to perform an execution against within the AdaptiveLimiter and returns
	// ErrExceeded if no permits are available.
	// Callers should call RecordSuccess, RecordFailure, or Release to release a successfully acquired permit back to the AdaptiveLimiter.
	AcquirePermit() (Permit, error)

	// TryAcquirePermit attempts to acquire a permit to perform an execution against within the AdaptiveLimiter,
	// returning whether the permit was acquired or not.
	// Callers should call RecordSuccess, RecordFailure, or Release to release a successfully acquired permit back to the AdaptiveLimiter.
	TryAcquirePermit() (Permit, bool)
}

// LimitChangedEvent indicates a AdaptiveLimiter's limit has changed.
type LimitChangedEvent struct {
}

/*
Builder builds AdaptiveLimiter instances.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	WithWindow(minDuration time.Duration, maxDuration time.Duration, windowSize uint) Builder[R]

	WithInitialLimit(initialLimit uint) Builder[R]

	WithMinLimit(minLimit uint) Builder[R]

	WithMaxLimit(maxLimit uint) Builder[R]

	WithQueueSizeFunc(fn func(limit uint) (queueSize uint)) Builder[R]

	// TODO call this RTT increase factor threshold?
	WithRTTolerance(rttTolerance float64) Builder[R]

	WithSmoothing(smoothing float64) Builder[R]

	WithLongWindow(longWindow uint) Builder[R]

	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]

	// Build returns a new AdaptiveLimiter using the builder's configuration.
	Build() AdaptiveLimiter[R]
}

type config[R any] struct {
	minWindow  time.Duration
	maxWindow  time.Duration
	windowSize uint

	initialLimit uint
	minLimit     uint
	maxLimit     uint
	smoothing    float64
	queueSizeFn  func(uint) uint
	longWindow   uint
	rttTolerance float64

	limitChangedListener func(LimitChangedEvent)
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		minWindow:    time.Second,
		maxWindow:    time.Second,
		windowSize:   10,
		initialLimit: 20,
		minLimit:     1,
		maxLimit:     200,
		smoothing:    0.2,
		queueSizeFn:  func(uint) uint { return 4 },
		longWindow:   600,
		rttTolerance: 1.5,
	}
}

func (c *config[R]) WithWindow(minDuration time.Duration, maxDuration time.Duration, windowSize uint) Builder[R] {
	c.minWindow = minDuration
	c.maxWindow = maxDuration
	c.windowSize = windowSize
	return c
}

func (c *config[R]) WithInitialLimit(initialLimit uint) Builder[R] {
	c.initialLimit = initialLimit
	return c
}

func (c *config[R]) WithMinLimit(minLimit uint) Builder[R] {
	c.minLimit = minLimit
	return c
}

func (c *config[R]) WithMaxLimit(maxLimit uint) Builder[R] {
	c.maxLimit = maxLimit
	return c
}

func (c *config[R]) WithQueueSizeFunc(fn func(limit uint) (queueSize uint)) Builder[R] {
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

func (c *config[R]) WithLongWindow(longWindow uint) Builder[R] {
	c.longWindow = longWindow
	return c
}

func (c *config[R]) OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R] {
	c.limitChangedListener = listener
	return c
}

func (c *config[R]) Build() AdaptiveLimiter[R] {
	return &adaptiveLimiter[R]{
		config:  c,
		mtx:     sync.Mutex{},
		limit:   float64(c.initialLimit),
		longRtt: util.NewEWMA(c.longWindow, 10),
		window:  newAverageSampleWindow(),
	}
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	mtx            sync.Mutex
	inflight       uint
	limit          float64
	lastRTT        time.Duration
	longRtt        util.MovingAverage
	window         SampleWindow
	nextUpdateTime time.Time
}

func (l *adaptiveLimiter[R]) recordSample(startTime time.Time, inflight uint, didDrop bool) {
	endTime := time.Now()
	rtt := endTime.Sub(startTime)
	l.window = l.window.AddSample(rtt, inflight, didDrop)

	if endTime.After(l.nextUpdateTime) {
		// Only update the limit if we received the expected number of samples
		if l.window.SampleCount() > l.windowSize {
			l.updateLimit(rtt, inflight)

			// TODO this is different from how the java library works
			// that library will discard samples and reset the window if end time is hit
			l.window = newAverageSampleWindow()
			l.nextUpdateTime = endTime.Add(min(max(l.window.MinRTT()*2, l.minWindow), l.maxWindow))
		}
	}
}

func (l *adaptiveLimiter[R]) updateLimit(rtt time.Duration, inflight uint) uint {
	l.lastRTT = rtt
	shortRTT := float64(rtt)
	longRTT := l.longRtt.Add(float64(rtt))
	queueSize := float64(l.queueSizeFn(uint(l.limit)))

	// If the long RTT is substantially larger than the short RTT then reduce the long RTT measurement.
	// This can happen when latency returns to normal after a prolonged prior of excessive load. Reducing the
	// long RTT without waiting for the exponential smoothing helps bring the system back to steady state.
	if longRTT/shortRTT > 2 {
		// TODO this might not be a good idea if a single shortRTT value ends up bumping the longRTT down
		l.longRtt.Set(longRTT * 0.95)
	}

	// Don't grow the limit if we are app limited
	if float64(inflight) < l.limit/2 {
		return uint(l.limit)
	}

	// Rtt could be higher than rtt_noload because of smoothing rtt noload updates
	// so set to 1.0 to indicate no queuing. Otherwise calculate the slope and don't
	// allow it to be reduced by more than half to avoid aggressive load-shedding due to
	// outliers.
	gradient := max(0.5, min(1.0, l.rttTolerance*longRTT/shortRTT))
	newLimit := l.limit*gradient + queueSize
	newLimit = l.limit*(1-l.smoothing) + newLimit*l.smoothing
	newLimit = max(float64(l.minLimit), min(float64(l.maxLimit), newLimit))

	if newLimit != l.limit {
		fmt.Println(fmt.Sprintf("%s new limit=%0.2f, shortRTT=%0.2f ms, longRTT=%0.2f ms, queueSize=%0.0f, gradient=%0.2f", time.Now().Format("2006/01/02 15:04:05"),
			newLimit, shortRTT/1e6, longRTT/1e6, queueSize, gradient))
	}

	l.limit = newLimit
	return uint(l.limit)
}

func (l *adaptiveLimiter[R]) AcquirePermit() (Permit, error) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if l.inflight >= uint(l.limit) {
		return nil, ErrExceeded
	}
	l.inflight++

	return &permit[R]{
		limiter:         l,
		currentInflight: l.inflight,
		startTime:       time.Now(),
	}, nil
}

func (l *adaptiveLimiter[R]) TryAcquirePermit() (Permit, bool) {
	p, err := l.AcquirePermit()
	return p, err == nil
}

// func (l *adaptiveLimiter[R]) lastRTT(units time.Duration) int64 {
// 	return int64(units) * l.lastRTT / int64(time.Nanosecond)
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
		BaseExecutor:    &policy.BaseExecutor[R]{},
		adaptiveLimiter: l,
	}
	be.Executor = be
	return be
}

// func (l *adaptiveLimiter[R]) String() string {
// 	return fmt.Sprintf("AdaptiveLimiter [limit=%d]", l.limit)
// }

type permit[R any] struct {
	limiter         *adaptiveLimiter[R]
	currentInflight uint
	startTime       time.Time
}

func (p *permit[R]) RecordSuccess() {
	p.limiter.mtx.Lock()
	defer p.limiter.mtx.Unlock()
	p.limiter.inflight--
	p.limiter.recordSample(p.startTime, p.currentInflight, false)
}

func (p *permit[R]) RecordFailure() {
	p.limiter.mtx.Lock()
	defer p.limiter.mtx.Unlock()
	p.limiter.inflight--
	p.limiter.recordSample(p.startTime, p.currentInflight, true)
}

func (p *permit[R]) Release() {
	p.limiter.mtx.Lock()
	defer p.limiter.mtx.Unlock()
	p.limiter.inflight--
}
