package pidlimiter

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds the current limit.
// var ErrExceeded = errors.New("limit exceeded")

const warmupSamples = 10

type Info interface {
	// Limit returns the concurrent execution limit, as calculated by the adaptive limiter.
	Limit() int

	// Inflight returns the current number of inflight executions.
	Inflight() int

	// Blocked returns the current number of blocked executions.
	Blocked() int
}

// AdaptiveLimiter is a concurrency limiter that adjusts its limit up or down based on execution time trends:
//  - When recent execution times are trending up relative to longer term execution times, the concurrency limit is decreased.
//  - When recent execution times are trending down relative to longer term execution times, the concurrency limit is increased.
//
// To accomplish this, short-term average execution times are tracked and regularly compared to a weighted moving average of
// longer-term execution times. Limit increases are additionally controlled to ensure they don't increase execution times. Any
// executions in excess of the limit will be rejected with ErrExceeded.
//
// By default, an AdaptiveLimiter will converge on a concurrency limit that represents the capacity of the machine it's
// running on, and avoids having executions block. Since enforcing a limit without allowing for blocking is too strict in
// some cases and may cause unexpected rejections, optional blocking of executions when the limiter is full can be
// enabled by configuring a maxExecutionTime.
//
// When blocking is enabled and the limiter is full, execution block up to the configures maxExecutionTime based on an
// estimated execution time for incoming requests. Estimated execution time considers the current number of blocked
// requests, the current limit, and the long-term average execution time.
//
// R is the execution result type. This type is concurrency safe.
type AdaptiveLimiter[R any] interface {
	failsafe.Policy[R]
	Info

	// AcquirePermit attempts to acquire a permit to perform an execution via the limiter, waiting until one is
	// available or the execution is canceled. Returns [context.Canceled] if the ctx is canceled.
	// Callers must call Record or Drop to release a successfully acquired permit back to the limiter.
	// ctx may be nil.
	AcquirePermit(context.Context) (Permit, error)

	// TryAcquirePermit attempts to acquire a permit to perform an execution via the limiter, returning whether the
	// Permit was acquired or not. Callers must call Record or Drop to release a successfully acquired permit back
	// to the limiter.
	TryAcquirePermit() (Permit, bool)

	// CanAcquirePermit returns whether it's currently possible to acquire a permit.
	CanAcquirePermit() bool
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
	// WithShortWindow configures the size of a window that is used to determine current, short-term load on the system in
	// terms of the min and max duration of the window, and the min number of samples that must be recorded in the window.
	// The default values are 1s, 1s, and 1.
	WithShortWindow(minDuration time.Duration, maxDuration time.Duration, minSamples uint) Builder[R]

	// WithLongWindow configures the number of short-term execution measurements that will be stored in an exponentially
	// weighted moving average window, representing the long-term baseline execution time.
	// The default value is 60.
	WithLongWindow(size uint) Builder[R]

	// WithLimits configures min, max, and initial limits.
	// The default values are 1, 1, and 20.
	WithLimits(minLimit uint, maxLimit uint, initialLimit uint) Builder[R]

	// WithMaxLimitFactor configures a maxLimitFactor which cap the limit as some multiple of the current inflight executions.
	// The default value is 5, which means the limit will only rise to 5 times the inflight executions.
	WithMaxLimitFactor(maxLimitFactor float32) Builder[R]

	// WithSmoothing configures a smoothingFactor, from 0.0 to 1.0, which smoothes limit changes so that they are more gradual.
	// The default value is .1, which means each limit change only contributes 10% to the new limit.
	WithSmoothing(smoothingFactor float32) Builder[R]

	// WithMaxExecutionTime enables blocking of executions when the limiter is full, up to some max average execution time,
	// which includes the time spent while executions are blocked waiting for a permit. Enabling this allows short execution
	// spikes to be absorbed without strictly rejecting executions when the limiter is full.
	// This is disabled by default, which means no executions will block when the limiter is full.
	WithMaxExecutionTime(maxExecutionTime time.Duration) Builder[R]

	// WithVariationWindow configures the size of the window used to calculate coefficient of variation for execution time
	// measurements, which helps determine when execution times are stable.
	// The default value is 8.
	WithVariationWindow(size uint) Builder[R]

	// WithCorrelationWindow configures how many recent limit and execution time measurements are stored to detect whether increases
	// in limits correlate with increases in execution times, which will cause the limit to be adjusted down.
	// The default value is 20.
	WithCorrelationWindow(size uint) Builder[R]

	// WithLogger configures a logger which provides debug logging of limit adjustments.
	WithLogger(logger *slog.Logger) Builder[R]

	WithPID() Builder[R]

	// OnLimitChanged configures a listener to be called with the limit changes.
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
	correlationWindowSize  uint
	variationWindow        uint

	minLimit        float64
	maxLimit        float64
	initialLimit    uint
	maxLimitFactor  float64
	smoothingFactor float64

	maxExecutionTime     time.Duration
	limitChangedListener func(LimitChangedEvent)

	pid bool
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		shortWindowMinDuration: time.Second,
		shortWindowMaxDuration: time.Second,
		shortWindowMinSamples:  1,
		longWindowSize:         60,
		correlationWindowSize:  20,
		variationWindow:        8,
		minLimit:               1,
		maxLimit:               200,
		initialLimit:           20,
		maxLimitFactor:         5.0,
		smoothingFactor:        0.1,
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

func (c *config[R]) WithMaxExecutionTime(maxExecutionTime time.Duration) Builder[R] {
	c.maxExecutionTime = maxExecutionTime
	return c
}

func (c *config[R]) WithCorrelationWindow(size uint) Builder[R] {
	c.correlationWindowSize = size
	return c
}

func (c *config[R]) WithVariationWindow(size uint) Builder[R] {
	c.variationWindow = size
	return c
}

func (c *config[R]) WithLogger(logger *slog.Logger) Builder[R] {
	c.logger = logger
	return c
}

func (c *config[R]) WithPID() Builder[R] {
	c.pid = true
	return c
}

func (c *config[R]) OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R] {
	c.limitChangedListener = listener
	return c
}

func (c *config[R]) Build() AdaptiveLimiter[R] {
	adaptive := &adaptiveLimiter[R]{
		config:            c,
		semaphore:         util.NewDynamicSemaphore(int64(c.initialLimit)),
		limit:             float64(c.initialLimit),
		shortRTT:          newRTTWindow(),
		longRTT:           util.NewEWMA(c.longWindowSize, warmupSamples),
		nextUpdateTime:    time.Now(),
		rttVariation:      newVariationWindow(8),
		correlationWindow: newCovarianceWindow(c.correlationWindowSize, warmupSamples),
	}
	// if c.pid {
	// 	result := newPIDLimiter(adaptive)
	// 	result.ScheduleCalibrations(context.Background(), time.Second)
	// 	return result
	// }
	if c.maxExecutionTime != 0 {
		return &blockingLimiter[R]{
			adaptiveLimiter: adaptive,
		}
	}
	// if c.priorized {
	// 	return &priorityBlockingLimiter[R]{
	// 		adaptiveLimiter:   adaptive,
	// 		maxExecutionTime:  maxExecutionTime,
	// 		priorityThreshold: PriorityLowest,
	// 		kp:                0.1, // Gradual response to spikes
	// 		ki:                1.4, // Aggressive response to sustained load
	// 		calibrations: &calibrationWindow{
	// 			window:       make([]calibrationPeriod, 30), // 30 second history
	// 			integralEWMA: util.NewEWMA(30, 5),           // 30 samples, 5 warmup
	// 		},
	// 	}
	// }
	return adaptive
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	semaphore *util.DynamicSemaphore
	mu        sync.Mutex

	// Guarded by mu
	limit             float64            // The current concurrency limit
	shortRTT          *rttWindow         // Tracks short term average execution time (round trip time)
	longRTT           util.MovingAverage // Tracks long term average execution time
	nextUpdateTime    time.Time          // Tracks when the limit can next be updated
	rttVariation      *variationWindow   // Tracks the variation of execution times
	correlationWindow *correlationWindow // Tracks the correlation between concurrency and execution times
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

func (l *adaptiveLimiter[R]) CanAcquirePermit() bool {
	return !l.semaphore.IsFull()
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

// Records the duration of a completed execution, updating the concurrency limit if the short shortRTT window is full.
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
		l.updateLimit(float64(l.shortRTT.average()), inflight)
		minRTT := l.shortRTT.minRTT
		l.shortRTT = newRTTWindow()
		minWindowTime := max(minRTT*2, l.shortWindowMinDuration)
		l.nextUpdateTime = now.Add(min(minWindowTime, l.shortWindowMaxDuration))
	}
}

// updateLimit updates the concurrency limit based on the gradient between the shortRTT and historical longRTT.
// A stability check prevents unnecessary decreases during steady state.
// A correlation adjustment prevents upward drift during overload.
func (l *adaptiveLimiter[R]) updateLimit(shortRTT float64, inflight int) {
	// Update long term RTT and calculate the initial gradient
	longRTT := l.longRTT.Add(shortRTT)
	gradient := longRTT / shortRTT
	queueSize := int(math.Ceil(float64(inflight) * (1 - gradient)))

	// Calculate RTT variation and correlation with inflight
	rttVariation := l.rttVariation.add(shortRTT)
	correlation := l.correlationWindow.add(float64(inflight), shortRTT)

	// If gradient would decrease limit and either the adjustment is small or RTT is stable, maintain the current limit
	if gradient < 1.0 && (gradient > .8 || rttVariation < 0.05) {
		l.logLimit("limit stable", l.limit, inflight, shortRTT, longRTT, queueSize, rttVariation, correlation, gradient)
		return
	}

	// Adjust the gradient based on any correlation between increases in concurrency and RTT by tracking their correlationWindow.
	// This is necessary to guard against situations where the limit rises too far while RTT is unstable.
	// if correlation != 0 {
	// 	// Adjust by up to 5%
	// 	adjustment := 1.0 - (correlation * 0.05)
	// 	gradient *= adjustment
	// 	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
	// 		l.logger.Debug("adjusting",
	// 			"adjustment", adjustment,
	// 			"gradient", fmt.Sprintf("%.2f", gradient))
	// 	}
	// }

	// Clamp the gradient
	gradient = max(0.5, min(1.5, gradient))

	// Adjust, smooth, and clamp the limit based on the gradient
	newLimit := l.limit * gradient
	newLimit = util.Smooth(l.limit, newLimit, l.smoothingFactor)
	newLimit = max(l.minLimit, min(l.maxLimit, newLimit))

	// Don't increase the limit beyond the max limit factor
	if newLimit > float64(inflight)*l.maxLimitFactor {
		l.logLimit("limit maxed", l.limit, inflight, shortRTT, longRTT, queueSize, rttVariation, correlation, gradient)
		return
	}

	l.logLimit("limit updated", newLimit, inflight, shortRTT, longRTT, queueSize, rttVariation, correlation, gradient)

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

func (l *adaptiveLimiter[R]) logLimit(msg string, limit float64, inflight int, shortRTT, longRTT float64, queueSize int, rttVariation, correlation, gradient float64) {
	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		l.logger.Debug(msg,
			"limit", fmt.Sprintf("%.2f", limit),
			"inflight", inflight,
			"shortRTT", fmt.Sprintf("%.2f", shortRTT/1e6),
			"longRTT", fmt.Sprintf("%.2f", longRTT/1e6),
			//	"queueSize", fmt.Sprintf("%d", queueSize),
			"rttVariation", fmt.Sprintf("%.3f", rttVariation),
			"correlation", fmt.Sprintf("%.2f", correlation),
			"gradient", fmt.Sprintf("%.2f", gradient))
	}
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
