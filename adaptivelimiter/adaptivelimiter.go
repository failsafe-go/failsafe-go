package adaptivelimiter

import (
	"errors"
	"fmt"
	"log/slog"
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

	WithCovarianceWindow(covarianceWindowSize uint) Builder[R]

	WithLogger(logger *slog.Logger) Builder[R]

	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]

	// Build returns a new AdaptiveLimiter using the builder's configuration.
	Build() AdaptiveLimiter[R]
}

type config[R any] struct {
	logger            *slog.Logger
	minWindowDuration time.Duration
	maxWindowDuration time.Duration
	minWindowSamples  uint
	longWindow        uint
	covarianceWindow  uint

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
		covarianceWindow:  20,

		initialLimit: 20,
		minLimit:     1,
		maxLimit:     200,
		smoothing:    0.2,
	}
}

func (c *config[R]) WithShortWindow(minDuration time.Duration, maxDuration time.Duration, minWindowSize uint) Builder[R] {
	c.minWindowDuration = minDuration
	c.maxWindowDuration = maxDuration
	c.minWindowSamples = minWindowSize
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

func (c *config[R]) WithCovarianceWindow(covarianceWindowSize uint) Builder[R] {
	c.covarianceWindow = covarianceWindowSize
	return c
}

func (c *config[R]) WithLogger(logger *slog.Logger) Builder[R] {
	c.logger = logger
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
		covariance: newCovarianceWindow(c.covarianceWindow),
	}
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	inflight atomic.Int32

	mtx            sync.Mutex
	window         *averageSampleWindow
	longRTT        util.MovingAverage
	limit          float64
	nextUpdateTime time.Time
	covariance     *covarianceWindow
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

	// Compute the gradient as the rate of change between the long term and short term latencies
	gradient := longRTT / shortRTT

	// Adjust the gradient based on any covariance between concurrency and latency.
	// Covariance indicates whether increases in recent limits correlate to increases in recent latencies.
	// This is necessary to avoid situations where the gradient and limit rise indefinitely.
	covariance := l.covariance.Add(float64(inflight), shortRTT)
	if covariance > 0 {
		// Decrease if concurrency correlates with higher latency
		gradient = gradient * 0.9
		covariance = 1
	} else if covariance < 0 {
		// Increase if concurrency does not correlate with higher latency
		gradient = gradient * 1.1
		covariance = -1
	}

	// Cap the gradient
	gradient = max(0.5, min(1.5, gradient))

	// Adjust, smooth, and cap the limit based on the gradient
	newLimit := l.limit * gradient
	newLimit = util.Smooth(l.limit, newLimit, l.smoothing)
	newLimit = max(l.minLimit, min(l.maxLimit, newLimit))

	// Cap increases to limit relative to concurrency
	if newLimit > float64(inflight)*10 {
		return uint(l.limit)
	}

	if uint(l.limit) != uint(newLimit) && l.limitChangedListener != nil {
		l.limitChangedListener(LimitChangedEvent{
			OldLimit: uint(l.limit),
			NewLimit: uint(newLimit),
		})
	}
	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		l.logger.Debug(fmt.Sprintf("new limit=%d, inflight=%d, shortRTT=%0.2f, longRTT=%0.2f, covariance=%d, gradient=%0.2f", int(newLimit), inflight, shortRTT/1e6, longRTT/1e6, int(covariance), gradient))
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
	e := &executor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		adaptiveLimiter: l,
	}
	e.Executor = e
	return e
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
