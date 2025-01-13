package adaptivelimiter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/influxdata/tdigest"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds the current limit.
var ErrExceeded = errors.New("limit exceeded")

const warmupSamples = 10

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
	AdaptiveLimiterInfo[R]

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

// Info about the adaptive limiter.
//
// R is the execution result type. This type is concurrency safe.
type AdaptiveLimiterInfo[R any] interface {
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

type Builder[R any] interface {
	BaseBuilder[R]

	// Build returns a new AdaptiveLimiter using the builder's configuration.
	Build() AdaptiveLimiter[R]
}

/*
BaseBuilder defines base behavior for building AdaptiveLimiter instances.

This type is not concurrency safe.
*/
type BaseBuilder[R any] interface {
	// WithShortWindow configures the size of a window that is used to determine current, short-term load on the system in
	// terms of the min and max duration of the window, and the min number of samples that must be recorded in the window.
	// The default values are 1s, 1s, and 1.
	WithShortWindow(minDuration time.Duration, maxDuration time.Duration, minSamples uint) Builder[R]

	// WithLongWindow configures the number of short-term execution measurements that will be stored in an exponentially
	// weighted moving average window, representing the long-term baseline execution time.
	// The default value is 60.
	WithLongWindow(size uint) Builder[R]

	// WithSampleQuantile configures the quantile ot recorded response times to consider when adjusting the concurrency limit.
	// Defaults to .9 which uses p90 samples.
	WithSampleQuantile(quantile float32) Builder[R]

	// WithLimits configures min, max, and initial limits.
	// The default values are 1, 1, and 20.
	WithLimits(minLimit uint, maxLimit uint, initialLimit uint) Builder[R]

	// WithMaxLimitFactor configures a maxLimitFactor which cap the limit as some multiple of the current inflight executions.
	// The default value is 5, which means the limit will only rise to 5 times the inflight executions.
	WithMaxLimitFactor(maxLimitFactor float32) Builder[R]

	// WithCorrelationWindow configures how many recent limit and execution time measurements are stored to detect whether increases
	// in limits correlate with increases in execution times, which will cause the limit to be adjusted down.
	// The default value is 20.
	WithCorrelationWindow(size uint) Builder[R]

	// WithStabilizationWindow configures the size of the windows tracking recent response times and inflight numbers to
	// determine when recent response times are stable.
	// The default value is 10.
	WithStabilizationWindow(size uint) Builder[R]

	// WithBlocking enables blocking of executions when the limiter is full, up to some max average execution time,
	// which includes the time spent while executions are blocked waiting for a permit. Enabling this allows short execution
	// spikes to be absorbed without strictly rejecting executions when the limiter is full.
	// This is disabled by default, which means no executions will block when the limiter is full.
	WithBlocking(maxExecutionTime time.Duration) Builder[R]

	// WithPrioritization enables prioritized blocking of executions when the limiter is full, up to some max average
	// execution time, which includes the time spent while executions are blocked waiting for a permit. Enabling this allows
	// short execution spikes to be absorbed without strictly rejecting executions when the limiter is full. This is
	// disabled by default, which means no executions will block when the limiter is full.
	WithPrioritization(prioritizer Prioritizer[R], maxExecutionTime time.Duration) PriorityLimiterBuilder[R]

	// WithLogger configures a logger which provides debug logging of limit adjustments.
	WithLogger(logger *slog.Logger) Builder[R]

	// OnLimitChanged configures a listener to be called with the limit changes.
	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]
}

// LimitChangedEvent indicates an AdaptiveLimiter's limit has changed.
type LimitChangedEvent struct {
	OldLimit uint
	NewLimit uint
}

type config[R any] struct {
	logger                  *slog.Logger
	shortWindowMinDuration  time.Duration
	shortWindowMaxDuration  time.Duration
	shortWindowMinSamples   uint
	longWindowSize          uint
	quantile                float64
	correlationWindowSize   uint
	stabilizationWindowSize uint

	minLimit       float64
	maxLimit       float64
	initialLimit   uint
	maxLimitFactor float64

	alphaFunc    func(int) int
	betaFunc     func(int) int
	increaseFunc func(int) int
	decreaseFunc func(int) int

	maxExecutionTime     time.Duration
	limitChangedListener func(LimitChangedEvent)
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		shortWindowMinDuration:  time.Second,
		shortWindowMaxDuration:  time.Second,
		shortWindowMinSamples:   1,
		longWindowSize:          60,
		quantile:                0.9,
		correlationWindowSize:   20,
		stabilizationWindowSize: 10,

		minLimit:       1,
		maxLimit:       200,
		initialLimit:   20,
		maxLimitFactor: 5.0,

		alphaFunc:    util.Log10RootFunction(3),
		betaFunc:     util.Log10RootFunction(6),
		increaseFunc: util.Log10RootFunction(0),
		decreaseFunc: util.Log10RootFunction(0),
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

func (c *config[R]) WithSampleQuantile(quantile float32) Builder[R] {
	c.quantile = float64(quantile)
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

func (c *config[R]) WithCorrelationWindow(size uint) Builder[R] {
	c.correlationWindowSize = size
	return c
}

func (c *config[R]) WithStabilizationWindow(size uint) Builder[R] {
	c.stabilizationWindowSize = size
	return c
}

func (c *config[R]) WithBlocking(maxExecutionTime time.Duration) Builder[R] {
	c.maxExecutionTime = maxExecutionTime
	return c
}

func (c *config[R]) WithPrioritization(prioritizer Prioritizer[R], maxExecutionTime time.Duration) PriorityLimiterBuilder[R] {
	c.maxExecutionTime = maxExecutionTime
	return &priorityConfig[R]{
		config:      c,
		prioritizer: prioritizer,
	}
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
		config:                c,
		semaphore:             util.NewDynamicSemaphore(int64(c.initialLimit)),
		limit:                 float64(c.initialLimit),
		shortRTT:              &td{TDigest: tdigest.NewWithCompression(100)},
		longRTT:               util.NewEWMA(c.longWindowSize, warmupSamples),
		nextUpdateTime:        time.Now(),
		rttCorrelation:        newCorrelationWindow(c.correlationWindowSize, warmupSamples),
		throughputCorrelation: newCorrelationWindow(c.correlationWindowSize, warmupSamples),
		rttWindow:             newRollingSum(c.stabilizationWindowSize),
		inflightWindow:        newRollingSum(c.stabilizationWindowSize),
	}
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	semaphore *util.DynamicSemaphore
	mu        sync.Mutex

	// Guarded by mu
	limit          float64            // The current concurrency limit
	shortRTT       *td                // Short term execution times in milliseconds
	longRTT        util.MovingAverage // Tracks long term average execution time in milliseconds
	nextUpdateTime time.Time          // Tracks when the limit can next be updated

	throughputCorrelation *correlationWindow // Tracks the correlation between concurrency and throughput
	rttCorrelation        *correlationWindow // Tracks the correlation between concurrency and round trip times (RTT)
	rttWindow             *rollingSum        // Tracks the variation of recent RTT
	inflightWindow        *rollingSum        // Tracks the slope of recent inflight executions
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
	if !dropped {
		rtt := now.Sub(startTime) / 1e6
		l.shortRTT.add(rtt)
	}

	if now.After(l.nextUpdateTime) && l.shortRTT.size >= l.shortWindowMinSamples {
		l.updateLimit(l.shortRTT.Quantile(l.quantile), inflight)
		minRTT := l.shortRTT.minRTT
		l.shortRTT.reset()
		minWindowTime := max(minRTT*2, l.shortWindowMinDuration)
		l.nextUpdateTime = now.Add(min(minWindowTime, l.shortWindowMaxDuration))
	}
}

// updateLimit updates the concurrency limit based on the gradient between the shortRTT and historical longRTT.
// A stability check prevents unnecessary decreases during steady state.
// A correlation adjustment prevents upward drift during overload.
func (l *adaptiveLimiter[R]) updateLimit(shortRTT float64, inflight int) {
	// Update long term RTT and calculate the queue size
	// This is the primary signal that we threshold off of to detect overload
	longRTT := l.longRTT.Add(shortRTT)
	gradient := longRTT / shortRTT
	queueSize := int(math.Ceil(float64(inflight) * (1 - gradient)))

	// Calculate throughput correlation, throughput slope, throughput CV, and rtt correlation
	// These are the secondary signals that we threshold off of to detect overload
	throughput := float64(inflight) / (shortRTT)
	throughputCorr, _, throughputCV := l.throughputCorrelation.add(float64(inflight), throughput)
	throughputSlope := l.throughputCorrelation.ySamples.calculateSlope()
	rttCorr, _, _ := l.rttCorrelation.add(float64(inflight), shortRTT)

	// Calculate the rtt CV and inflight slope
	// These are used to detect when rtt has stabilized after a recent decrease
	l.rttWindow.add(shortRTT)
	rttCV, _, _ := l.rttWindow.calculateCV()
	l.inflightWindow.add(float64(inflight))
	inflightSlope := l.inflightWindow.calculateSlope()

	newLimit := l.limit
	alpha := l.alphaFunc(int(l.limit)) // alpha is the queueSize threshold below which we increase
	beta := l.betaFunc(int(l.limit))   // beta is the queueSize threshold above which we decrease
	var direction, reason string
	decrease := false

	if queueSize > beta {
		// This condition handles severe overload where recent RTT significantly exceeds the baseline
		reason = "queue"
		decrease = true
	} else if throughputCorr < 0 && throughputSlope < 0 {
		// This condition handles moderate overload where inflight is increasing but throughput is decreasing
		reason = "thrptCorr"
		decrease = true
	} else if throughputCV < .1 && rttCorr > .7 {
		// This condition handles moderate overload where throughputCorr is positive and stable but rttCorr is high
		// This indicates overload since latency is increasing with inflight, but throughput is not
		reason = "thrptCV"
		decrease = true
	} else if queueSize < alpha {
		// If our queue size is sufficiently small, increase until we detect overload
		direction = "increased"
		reason = "queue"
		newLimit = l.limit + float64(l.increaseFunc(int(l.limit)))
	} else {
		// If queueSize is between alpha and beta, leave the limit unchanged
		direction = "leaving"
		reason = "queue"
	}

	if decrease {
		// Consider the limit stable if RTT is stable and inflight recently decreased
		if rttCV < 0.05 && inflightSlope < 0 {
			direction = "leaving"
			reason = "stable"
		} else {
			direction = "decreased"
			newLimit = l.limit - float64(l.decreaseFunc(int(l.limit)))
		}
	}

	// Clamp the limit based on the gradient
	newLimit = max(l.minLimit, min(l.maxLimit, newLimit))

	// Don't increase the limit beyond the max limit factor
	if newLimit > float64(inflight)*l.maxLimitFactor {
		direction = "decreased"
		reason = "maxed"
		newLimit = l.limit - float64(l.decreaseFunc(int(l.limit)))
	}

	l.logLimit(direction, reason, newLimit, gradient, queueSize, inflight, shortRTT, longRTT, inflightSlope, rttCorr, rttCV, throughput, throughputCorr, throughputSlope, throughputCV)

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

func (l *adaptiveLimiter[R]) logLimit(direction, reason string, limit float64, gradient float64, queueSize, inflight int, shortRTT, longRTT, inflightSlope, rttCorr, rttCV, throughput, throughputCorr, throughputSlope, throughputCV float64) {
	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		l.logger.Debug("limit update",
			"direction", direction,
			"reason", reason,
			"limit", fmt.Sprintf("%.2f", limit),
			"gradient", fmt.Sprintf("%.2f", gradient),
			"queueSize", fmt.Sprintf("%d", queueSize),
			"inflight", inflight,
			"shortRTT", fmt.Sprintf("%.2f", shortRTT),
			"longRTT", fmt.Sprintf("%.2f", longRTT),
			"thrpt", fmt.Sprintf("%.2f", throughput),
			"thrptCorr", fmt.Sprintf("%.2f", throughputCorr),
			"thrptSlp", fmt.Sprintf("%.2f", throughputSlope),
			"rttCV", fmt.Sprintf("%.3f", rttCV),
			"inflightSlp", fmt.Sprintf("%.2f", inflightSlope),
			"thrptCV", fmt.Sprintf("%.2f", throughputCV),
			"rttCorr", fmt.Sprintf("%.2f", rttCorr))
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
