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

const warmupSamples = 10

// AdaptiveLimiter is a concurrency limiter that adjusts its limit up or down based on latency trends:
//  - When recent latencies are trending up relative to longer term latencies, the concurrency limit is decreased.
//  - When recent latencies are trending down relative to longer term latencies, the concurrency limit is increased.
//
// To accomplish this, recent average latencies are tracked and regularly compared to a weighted moving average of
// longer term latencies. Limit increases are additionally controlled to ensure they don't increase latency. Any
// executions in excess of the limit will be rejected with ErrExceeded.
//
// By default, an AdaptiveLimiter will converge on a concurrency limit that represents the capacity of the machine it's
// running on, and avoids having executions queue up. Since running a limit without allowing for queueing is too strict
// in some cases and may cause unexpected rejections, optional blocking of requests when the limiter is full can be
// enabled by configuring a maxLatency.
//
// When blocking is enabled and the limiter is full, requests block up to some max latency based on an estimated latency
// for incoming requests. Estimated latency considers the current number of blocked requests, the current limit, and the
// average request processing time.
//
// R is the execution result type. This type is concurrency safe.
type AdaptiveLimiter[R any] interface {
	failsafe.Policy[R]

	// AcquirePermit attempts to acquire a permit to perform an execution via the limiter, waiting until one is
	// available or the execution is canceled. Returns [context.Canceled] if the ctx is canceled.
	// Callers must call Record or Drop to release a successfully acquired permit back to the limiter.
	// ctx may be nil.
	AcquirePermit(context.Context) (Permit, error)

	// TryAcquirePermit attempts to acquire a permit to perform an execution via the limiter, returning whether the
	// Permit was acquired or not. Callers must call Record or Drop to release a successfully acquired permit back
	// to the limiter.
	TryAcquirePermit() (Permit, bool)

	// Limit returns the concurrent execution limit, as calculated by the adaptive limiter.
	Limit() int

	// Inflight returns the current number of inflight executions.
	Inflight() int

	// Blocked returns the current number of blocked executions.
	Blocked() int
}

// Permit is a permit to perform an execution that must be completed by calling Record or Drop.
type Permit interface {
	// Record records an execution completion and releases a permit back to the limiter. The execution duration will be used
	// to influence the limiter.
	Record()

	// Drop releases an execution permit back to the limiter without recording a completion. This should be used when an
	// execution completes prematurely, such as via a timeout, and we don't want the execution duration to influence the
	// limiter.
	Drop()
}

/*
Builder builds AdaptiveLimiter instances.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	WithShortWindow(minDuration time.Duration, maxDuration time.Duration, minSamples uint) Builder[R]

	WithLongWindow(size uint) Builder[R]

	WithCovarianceWindow(size uint) Builder[R]

	WithLimits(minLimit uint, maxLimit uint, initialLimit uint) Builder[R]

	WithMaxLimitFactor(maxLimitFactor float32) Builder[R]

	WithSmoothing(smoothingFactor float32) Builder[R]

	WithMaxLatency(maxLatency time.Duration) Builder[R]

	WithLogger(logger *slog.Logger) Builder[R]

	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]

	// Build returns a new AdaptiveLimiter using the builder's configuration.
	Build() AdaptiveLimiter[R]
}

// LimitChangedEvent indicates an AdaptiveLimiter's limit has changed.
type LimitChangedEvent struct {
	OldLimit uint
	NewLimit uint
}

type config[R any] struct {
	logger                 *slog.Logger
	shortWindowMinDuration time.Duration
	shortWindowMaxDuration time.Duration
	shortWindowMinSamples  uint
	longWindowSize         uint
	covarianceWindowSize   uint

	minLimit        float64
	maxLimit        float64
	initialLimit    uint
	maxLimitFactor  float64
	smoothingFactor float64

	maxLatency           time.Duration
	limitChangedListener func(LimitChangedEvent)
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		shortWindowMinDuration: time.Second,
		shortWindowMaxDuration: time.Second,
		shortWindowMinSamples:  1,
		longWindowSize:         60,
		covarianceWindowSize:   20,
		minLimit:               1,
		maxLimit:               200,
		initialLimit:           20,
		maxLimitFactor:         5.0,
		smoothingFactor:        0.2,
	}
}

func (c *config[R]) WithShortWindow(minDuration time.Duration, maxDuration time.Duration, minSamples uint) Builder[R] {
	c.shortWindowMinDuration = minDuration
	c.shortWindowMaxDuration = maxDuration
	c.shortWindowMinSamples = minSamples
	return c
}

func (c *config[R]) WithLongWindow(size uint) Builder[R] {
	c.longWindowSize = size
	return c
}

func (c *config[R]) WithCovarianceWindow(size uint) Builder[R] {
	c.covarianceWindowSize = size
	return c
}

func (c *config[R]) WithLimits(minLimit uint, maxLimit uint, initialLimit uint) Builder[R] {
	c.minLimit = float64(minLimit)
	c.maxLimit = float64(maxLimit)
	c.initialLimit = initialLimit
	return c
}

func (c *config[R]) WithMaxLimitFactor(maxLimitFactor float32) Builder[R] {
	c.maxLimitFactor = float64(maxLimitFactor)
	return c
}

func (c *config[R]) WithSmoothing(smoothingFactor float32) Builder[R] {
	c.smoothingFactor = float64(smoothingFactor)
	return c
}

func (c *config[R]) WithMaxLatency(maxLatency time.Duration) Builder[R] {
	c.maxLatency = maxLatency
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
	adaptive := &adaptiveLimiter[R]{
		config:          c,
		semaphore:       util.NewDynamicSemaphore(int64(c.initialLimit)),
		limit:           float64(c.initialLimit),
		shortRTT:        newRTTWindow(),
		longRTT:         util.NewEWMA(c.longWindowSize, warmupSamples),
		rttSamples:      newVariationWindow(8),
		inflightSamples: newVariationWindow(8),
		covariance:      newCovarianceWindow(c.covarianceWindowSize),
	}
	if c.maxLatency != 0 {
		return &blockingLimiter[R]{
			adaptiveLimiter: adaptive,
		}
	}
	return adaptive
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	semaphore *util.DynamicSemaphore
	mu        sync.Mutex

	// Guarded by mu
	limit           float64            // The current concurrency limit
	shortRTT        *rttWindow         // Tracks short term average latency
	longRTT         util.MovingAverage // Tracks long term average latency
	nextUpdateTime  time.Time          // Tracks when the limit can next be updated
	rttSamples      *variationWindow
	inflightSamples *variationWindow
	covariance      *covarianceWindow // Tracks the correlation between concurrency latency
}

func (l *adaptiveLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := l.semaphore.Acquire(ctx); err != nil {
		return nil, err
	}
	return &recordingPermit[R]{
		limiter:         l,
		startTime:       time.Now(),
		currentInflight: l.semaphore.Inflight(),
	}, nil
}

func (l *adaptiveLimiter[R]) TryAcquirePermit() (Permit, bool) {
	if !l.semaphore.TryAcquire() {
		return nil, false
	}
	return &recordingPermit[R]{
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

func (l *adaptiveLimiter[R]) Blocked() int {
	return 0
}

// Records the round-trip time of a completed execution, updating the concurrency limit if the short shortRTT is full.
func (l *adaptiveLimiter[R]) record(startTime time.Time, inflight int, dropped bool) {
	l.semaphore.Release()
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	rtt := now.Sub(startTime)
	if !dropped {
		l.shortRTT.add(rtt)
	}

	if now.After(l.nextUpdateTime) && l.shortRTT.size >= l.shortWindowMinSamples {
		l.updateLimit(l.shortRTT.average(), inflight)
		l.shortRTT = newRTTWindow()
		minWindowTime := max(l.shortRTT.minRTT*2, l.shortWindowMinDuration)
		l.nextUpdateTime = now.Add(min(minWindowTime, l.shortWindowMaxDuration))
	}
}

// Stability check prevents unnecessary decreases during steady state
// Covariance prevents upward drift during overload
func (l *adaptiveLimiter[R]) updateLimit(rtt time.Duration, inflight int) {
	// Update short and long term latency
	shortRTT := float64(rtt)
	longRTT := l.longRTT.Add(float64(rtt))

	// Calculate stability metrics
	rttVariation := l.rttSamples.add(shortRTT)
	inflightVariation := l.inflightSamples.add(float64(inflight))

	// Calculate latency gradient
	gradient := longRTT / shortRTT

	// If system is stable and gradient would decrease limit, maintain current limit
	isStable := rttVariation < 0.05 && inflightVariation < 0.05
	if isStable && gradient < 1.0 {
		if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
			l.logger.Debug("stable rtt and inflight",
				"rttVariation", fmt.Sprintf("%.3f", rttVariation),
				"inflightVariation", fmt.Sprintf("%.3f", inflightVariation),
				"gradient", fmt.Sprintf("%.3f", gradient))
		}
		return
	}

	// Adjust the gradient based on any covariance between concurrency and latency.
	// Covariance indicates whether increases in recent limits correlate to increases in recent latencies.
	// This is necessary to avoid situations where the gradient and limit rise indefinitely.
	// Get correlation between limit and latency
	correlation := l.covariance.add(float64(inflight), shortRTT)
	if correlation != 0 {
		// Use a gentler adjustment factor
		adjustment := 1.0 - (correlation * 0.05) // Max ±5% adjustment instead of ±10%
		gradient *= adjustment
	}

	// Clamp the gradient
	gradient = max(0.5, min(1.5, gradient))

	// Adjust, smooth, and clamp the limit based on the gradient
	newLimit := l.limit * gradient
	newLimit = util.Smooth(l.limit, newLimit, l.smoothingFactor)
	newLimit = max(l.minLimit, min(l.maxLimit, newLimit))

	// Don't increase the limit beyond the max limit factor
	if newLimit > float64(inflight)*l.maxLimitFactor {
		return
	}

	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
			l.logger.Debug("updated limit",
				"newLimit", fmt.Sprintf("%.2f", newLimit),
				"oldLimit", fmt.Sprintf("%.2f", l.limit),
				"inflight", inflight,
				"shortRTT", fmt.Sprintf("%.2f", shortRTT/1e6),
				"longRTT", fmt.Sprintf("%.2f", longRTT/1e6),
				"gradient", fmt.Sprintf("%.2f", gradient),
				"correlation", fmt.Sprintf("%.2f", correlation))
		}
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
	e := &adaptiveExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		adaptiveLimiter: l,
	}
	e.Executor = e
	return e
}

type recordingPermit[R any] struct {
	limiter         *adaptiveLimiter[R]
	startTime       time.Time
	currentInflight int
}

func (p *recordingPermit[R]) Record() {
	p.limiter.record(p.startTime, p.currentInflight, false)
}

func (p *recordingPermit[R]) Drop() {
	p.limiter.record(p.startTime, p.currentInflight, true)
}
