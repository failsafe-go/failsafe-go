package adaptivelimiter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds the current limit.
var ErrExceeded = errors.New("limit exceeded")

// AdaptiveLimiter is a concurrency limiter that adjusts its limit up or down based on latency trends:
//  - When recent latencies are trending up relative to longer term latencies, the concurrency limit is decreased.
//  - When recent latencies are trending down relative to longer term latencies, the concurrency limit is increased.
//
// To accomplish this, recent average latencies are tracked and regularly compared to a weighted moving average of
// longer term latencies. Limit increases are additionally controlled to ensure they don't increase latency.
//
// An AdaptiveLimiter will converge on a concurrency limit that represents the capacity of the machine it's running on,
// and avoids having executions queue up. Since running a limit without allowing for queueing is too strict in some
// cases, and may cause unexpected rejections, it's recommended to wrap an AdaptiveLimiter with a BlockingLimiter, which
// will additionally allow for some executions to block, without being rejected, when the AdaptiveLimiter is at
// capacity.
//
// R is the execution result type. This type is concurrency safe.
type AdaptiveLimiter[R any] interface {
	failsafe.Policy[R]
	Limiter

	// Limit returns the concurrent execution limit, as calculated by the adaptive limiter.
	Limit() int

	// Inflight returns the current number of inflight executions.
	Inflight() int
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

// LimitChangedEvent indicates a AdaptiveLimiter's limit has changed.
type LimitChangedEvent struct {
	OldLimit uint
	NewLimit uint
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
		semaphore:  util.NewDynamicSemaphore(int64(c.initialLimit)),
		limit:      float64(c.initialLimit),
		window:     newSampleWindow(),
		longRTT:    util.NewEWMA(c.longWindow, 10),
		covariance: newCovarianceWindow(c.covarianceWindow),
	}
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	semaphore *util.DynamicSemaphore
	mu        sync.Mutex

	// Guarded by mu
	limit          float64
	window         *sampleWindow
	nextUpdateTime time.Time
	longRTT        util.MovingAverage
	covariance     *covarianceWindow
}

func (l *adaptiveLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := l.semaphore.Acquire(ctx); err != nil {
		return nil, err
	}
	return &permit[R]{
		limiter:         l,
		startTime:       time.Now(),
		currentInflight: l.semaphore.Inflight(),
	}, nil
}

func (l *adaptiveLimiter[R]) TryAcquirePermit() (Permit, bool) {
	if !l.semaphore.TryAcquire() {
		return nil, false
	}
	return &permit[R]{
		limiter:         l,
		startTime:       time.Now(),
		currentInflight: l.semaphore.Inflight(),
	}, true
}

func (l *adaptiveLimiter[R]) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int(l.limit)
}

func (l *adaptiveLimiter[R]) Inflight() int {
	return l.semaphore.Inflight()
}

// Records the timing of a completed execution, possibly updating the concurrency limit.
func (l *adaptiveLimiter[R]) record(startTime time.Time, inflight int, dropped bool) {
	l.semaphore.Release()
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	rtt := now.Sub(startTime)
	if !dropped {
		l.window = l.window.AddSample(rtt)
	}

	if now.After(l.nextUpdateTime) && l.window.count >= l.minWindowSamples {
		l.updateLimit(l.window.AverageRTT(), inflight)
		l.window = newSampleWindow()
		minWindowTime := max(l.window.minRTT*2, l.minWindowDuration)
		l.nextUpdateTime = now.Add(min(minWindowTime, l.maxWindowDuration))
	}
}

func (l *adaptiveLimiter[R]) updateLimit(rtt time.Duration, inflight int) {
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

	// Clamp the gradient
	gradient = max(0.5, min(1.5, gradient))

	// Adjust, smooth, and clamp the limit based on the gradient
	newLimit := l.limit * gradient
	newLimit = util.Smooth(l.limit, newLimit, l.smoothing)
	newLimit = max(l.minLimit, min(l.maxLimit, newLimit))

	// Clamp increases to limit relative to concurrency
	if newLimit > float64(inflight)*10 {
		return
	}

	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		l.logger.Debug(fmt.Sprintf("newLimit=%0.2f, oldLimit=%0.2f, inflight=%d, shortRTT=%0.2f, longRTT=%0.2f, covariance=%d, gradient=%0.2f", newLimit, l.limit, inflight, shortRTT/1e6, longRTT/1e6, int(covariance), gradient))
	}

	if uint(l.limit) != uint(newLimit) {
		if l.limitChangedListener != nil {
			l.limitChangedListener(LimitChangedEvent{
				OldLimit: uint(l.limit),
				NewLimit: uint(newLimit),
			})
		}
	}
	l.semaphore.SetSize(int64(newLimit))
	l.limit = newLimit
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
	currentInflight int
}

func (p *permit[R]) Record() {
	p.limiter.record(p.startTime, p.currentInflight, false)
}

func (p *permit[R]) Drop() {
	p.limiter.record(p.startTime, p.currentInflight, true)
}
