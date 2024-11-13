package adaptivelimiter

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	OldLimit uint
	NewLimit uint
}

/*
Builder builds AdaptiveLimiter instances.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	WithShortWindow(minDuration time.Duration, maxDuration time.Duration, minWindowSize uint) Builder[R]

	WithLongWindow(longWindowSize uint) Builder[R]

	WithInitialLimit(initialLimit uint) Builder[R]

	WithMinLimit(minLimit uint) Builder[R]

	WithMaxLimit(maxLimit uint) Builder[R]

	WithSmoothing(smoothing float64) Builder[R]

	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]

	// Build returns a new AdaptiveLimiter using the builder's configuration.
	Build() AdaptiveLimiter[R]
}

type config[R any] struct {
	minWindowDuration time.Duration
	maxWindowDuration time.Duration
	minWindowSamples  uint
	longWindow        uint

	initialLimit uint
	minLimit     float64
	maxLimit     float64
	smoothing    float64

	limitChangedListener func(LimitChangedEvent)
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		minWindowDuration: time.Second,
		maxWindowDuration: time.Second,
		minWindowSamples:  10,
		longWindow:        600,

		initialLimit: 20,
		minLimit:     1,
		maxLimit:     200,
		smoothing:    0.2,
	}
}

func (c *config[R]) WithShortWindow(minDuration time.Duration, maxDuration time.Duration, minSamples uint) Builder[R] {
	c.minWindowDuration = minDuration
	c.maxWindowDuration = maxDuration
	c.minWindowSamples = minSamples
	return c
}

func (c *config[R]) WithLongWindow(longWindowSize uint) Builder[R] {
	c.longWindow = longWindowSize
	return c
}

func (c *config[R]) WithInitialLimit(initialLimit uint) Builder[R] {
	c.initialLimit = initialLimit
	return c
}

func (c *config[R]) WithMinLimit(minLimit uint) Builder[R] {
	c.minLimit = float64(minLimit)
	return c
}

func (c *config[R]) WithMaxLimit(maxLimit uint) Builder[R] {
	c.maxLimit = float64(maxLimit)
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
		config:     c,
		mtx:        sync.Mutex{},
		limit:      float64(c.initialLimit),
		window:     newAverageSampleWindow(),
		longRTT:    util.NewEWMA(c.longWindow, 10),
		covariance: newCovarianceWindow(20),
	}
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	inflight atomic.Int32

	mtx            sync.Mutex
	window         *averageSampleWindow // Guarded by mtx
	longRTT        util.MovingAverage   // Guarded by mtx
	limit          float64              // Guarded by mtx
	nextUpdateTime time.Time            // Guarded by mtx
	covariance     *covarianceWindow    // Guarded by mtx
}

func (l *adaptiveLimiter[R]) recordSample(startTime time.Time, inflight uint, didDrop bool) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	now := time.Now()
	rtt := now.Sub(startTime)
	l.window = l.window.AddSample(rtt, inflight, didDrop)

	if now.After(l.nextUpdateTime) && l.window.sampleCount >= l.minWindowSamples {
		l.updateLimit(rtt, inflight)

		l.window = newAverageSampleWindow()
		minWindowTime := max(l.window.minRTT*2, l.minWindowDuration)
		l.nextUpdateTime = now.Add(min(minWindowTime, l.maxWindowDuration))
	}
}

func (l *adaptiveLimiter[R]) updateLimit(rtt time.Duration, inflight uint) uint {
	shortRTT := float64(rtt)
	longRTT := l.longRTT.Add(float64(rtt))

	// If the long RTT is substantially larger than the short RTT then reduce the long RTT measurement.
	// This can happen when latency returns to normal after a prolonged prior of excessive load.  Reducing the
	// long RTT without waiting for the exponential smoothing helps bring the system back to steady state.
	// if (longRTT / shortRTT) > 2 {
	// 	l.longRTT.Add(longRTT * .9)
	// 	fmt.Println("lowered long RTT by .9")
	// }

	// Compute the gradient as the rate of change between the long term and short term latencies
	gradient := longRTT / shortRTT

	// Use covariance to adjust gradient
	// This is necessary to avoid situations where the gradient and limit rise indefinitely
	// Covariance describes how well increases to recent limits correlate to increases in recent latencies
	initialGradient := gradient
	covariance := l.covariance.Add(float64(inflight), shortRTT)
	if covariance > 0 {
		// Decrease gradient if concurrency correlates with higher latency
		gradient = gradient * 0.9
		covariance = 1
	} else if covariance < 0 {
		// Increase gradient if concurrency does not correlate with higher latency
		gradient = gradient * 1.1
		covariance = -1
	}

	gradient = max(0.5, min(1.5, gradient))
	newLimit := l.limit * gradient
	newLimit = util.Smooth(l.limit, newLimit, l.smoothing)
	newLimit = max(l.minLimit, min(l.maxLimit, newLimit))

	if newLimit > float64(inflight)*10 {
		fmt.Println(fmt.Sprintf("%s old limit=%0.2f, inflight=%d, shortRTT=%0.2f ms, longRTT=%0.2f ms, gradient=%0.2f",
			time.Now().Format("2006/01/02 15:04:05"), l.limit, inflight, shortRTT/1e6, longRTT/1e6, gradient))
		return uint(l.limit)
	}

	fmt.Println(fmt.Sprintf("%s new limit=%0.2f, inflight=%d, shortRTT=%0.2f ms, longRTT=%0.2f ms, covariance=%d, initialGradient=%0.2f, gradient=%0.2f", time.Now().Format("2006/01/02 15:04:05"),
		newLimit, inflight, shortRTT/1e6, longRTT/1e6, int(covariance), initialGradient, gradient))

	if uint(l.limit) != uint(newLimit) && l.limitChangedListener != nil {
		l.limitChangedListener(LimitChangedEvent{
			OldLimit: uint(l.limit),
			NewLimit: uint(newLimit),
		})
	}

	l.limit = newLimit
	return uint(l.limit)
}

func (l *adaptiveLimiter[R]) AcquirePermit() (Permit, error) {
	inflight := l.inflight.Load()
	if inflight >= int32(l.limit) {
		return nil, ErrExceeded
	}

	inflight = l.inflight.Add(1)
	return &permit[R]{
		limiter:         l,
		currentInflight: uint(inflight),
		startTime:       time.Now(),
	}, nil
}

func (l *adaptiveLimiter[R]) TryAcquirePermit() (Permit, bool) {
	p, err := l.AcquirePermit()
	return p, err == nil
}

func (l *adaptiveLimiter[R]) ToExecutor(_ R) any {
	ale := &executor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		adaptiveLimiter: l,
	}
	ale.Executor = ale
	return ale
}

type permit[R any] struct {
	limiter         *adaptiveLimiter[R]
	startTime       time.Time
	currentInflight uint
}

func (p *permit[R]) RecordSuccess() {
	p.limiter.inflight.Add(-1)
	p.limiter.recordSample(p.startTime, p.currentInflight, false)
}

func (p *permit[R]) RecordFailure() {
	p.limiter.inflight.Add(-1)
	p.limiter.recordSample(p.startTime, p.currentInflight, true)
}

func (p *permit[R]) Release() {
	p.limiter.inflight.Add(-1)
}
