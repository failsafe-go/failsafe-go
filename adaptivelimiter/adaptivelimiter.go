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

const (
	warmupSamples   = 10
	smoothedSamples = 5
)

// AdaptiveLimiter is a concurrency limiter that adjusts its limit up or down based on execution time trends:
//  - When recent execution times are trending up relative to longer term execution times, the concurrency limit is decreased.
//  - When recent execution times are trending down relative to longer term execution times, the concurrency limit is increased.
//
// To accomplish this, short-term execution times are tracked and regularly compared to a weighted moving average of
// longer-term execution times. Limit increases are additionally controlled to ensure they don't increase execution times. Any
// executions in excess of the limit will be rejected with ErrExceeded.
//
// By default, an AdaptiveLimiter will converge on a concurrency limit that represents the capacity of the machine it's
// running on, and avoids having executions queue. Since enforcing a limit without allowing for queueing is too strict
// in some cases and may cause unexpected rejections, optional queueing of executions when the limiter is full can be
// enabled via WithQueueing. Queueing with prioritized rejections can be enabled via BuildPrioritized, which accepts a
// Prioritizer that can be shared across multiple limiters.
//
// R is the execution result type. This type is concurrency safe.
type AdaptiveLimiter[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit to perform an execution via the limiter, waiting until one is
	// available or the execution is canceled. Returns [context.Canceled] if the ctx is canceled.
	// Callers must call Record or Drop to release a successfully acquired permit back to the limiter.
	// ctx may be nil.
	AcquirePermit(context.Context) (Permit, error)

	// AcquirePermitWithMaxWait attempts to acquire a permit to perform an execution via the limiter, waiting until one is
	// available, the execution is canceled, or the maxWaitTime is exceeded.
	AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) (Permit, error)

	// TryAcquirePermit attempts to acquire a permit to perform an execution via the limiter, returning whether the Permit
	// was acquired or not. This method will not block if the limiter is full. Callers must call Record or Drop to release a
	// successfully acquired permit back to the limiter.
	TryAcquirePermit() (Permit, bool)

	// CanAcquirePermit returns whether it's currently possible to acquire a permit.
	CanAcquirePermit() bool

	// Reset resets the limiter to its initial limit.
	Reset()
}

// Metrics provides info about the adaptive limiter.
//
// R is the execution result type. This type is concurrency safe.
type Metrics interface {
	// Limit returns the concurrent execution limit, as calculated by the adaptive limiter.
	Limit() int

	// Inflight returns the current number of inflight executions.
	Inflight() int

	// Queued returns the current number of queyed executions when the limiter is full.
	Queued() int
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
Builder defines base behavior for building AdaptiveLimiter instances.

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

	// WithSampleQuantile configures the quantile of recorded response times to consider when adjusting the concurrency limit.
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

	// WithQueueing enables additional queueing of executions when the limiter is full. Queueing allows short execution
	// spikes to be absorbed without strictly rejecting executions.
	//
	// When queueing is enabled and the limiter is full, requests are queued up to the current limit times the
	// initialRejectionFactor, after which they will gradually start to be rejected up to the limit times the
	// maxRejectionFactor. This allows rejection to gradually adjust based on how many requests are queued, relative to the
	// limit. Queueing allows short execution spikes to be absorbed without strictly rejecting executions.
	//
	// WithQueueing is disabled by default, which means no executions will queue when the limiter is full.
	WithQueueing(initialRejectionFactor, maxRejectionFactor float32) Builder[R]

	// WithMaxWaitTime configures the maxWaitTime to wait for a permit to be available, when the limiter is full.
	WithMaxWaitTime(maxWaitTime time.Duration) Builder[R]

	// OnLimitExceeded registers the listener to be called when the limit is exceeded.
	OnLimitExceeded(listener func(event failsafe.ExecutionEvent[R])) Builder[R]

	// OnLimitChanged configures a listener to be called with the limit changes.
	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]

	// WithLogger configures a logger which provides debug logging of limit adjustments.
	WithLogger(logger *slog.Logger) Builder[R]

	// Build returns a new AdaptiveLimiter using the builder's configuration.
	Build() AdaptiveLimiter[R]

	// BuildPrioritized returns a new PrioritizedLimiter using the builder's configuration. This enables queueing and
	// prioritized rejections of executions when the limiter is full, where executions block while waiting for a permit.
	// Enabling this allows short execution spikes to be absorbed without strictly rejecting executions when the limiter is
	// full. Rejections are performed using the Prioritizer, which sets a rejection threshold baesd on the most overloaded
	// limiters being used by the Prioritizer. The amount of queueing can be configured via WithQueueing, and defaults to an
	// initialRejectionFactor of 2 times the current limit and a maxRejectionFactor of 3 times the current limit.
	//
	// Prioritized rejection is disabled by default, which means no executions will block when the limiter is full.
	BuildPrioritized(prioritizer Prioritizer) PriorityLimiter[R]
}

// LimitChangedEvent indicates an AdaptiveLimiter's limit has changed.
type LimitChangedEvent struct {
	OldLimit uint
	NewLimit uint
}

type config[R any] struct {
	maxWaitTime            time.Duration
	logger                 *slog.Logger
	shortWindowMinDuration time.Duration
	shortWindowMaxDuration time.Duration
	shortWindowMinSamples  uint
	longWindowSize         uint
	quantile               float64
	correlationWindowSize  uint

	minLimit, maxLimit float64
	initialLimit       uint
	maxLimitFactor     float64

	// Rejection config
	initialRejectionFactor float64
	maxRejectionFactor     float64

	alphaFunc, betaFunc        func(int) int
	increaseFunc, decreaseFunc func(int) int

	onExceeded           func(failsafe.ExecutionEvent[R])
	limitChangedListener func(LimitChangedEvent)
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		shortWindowMinDuration: time.Second,
		shortWindowMaxDuration: time.Second,
		shortWindowMinSamples:  50,
		longWindowSize:         60,
		quantile:               0.9,
		correlationWindowSize:  50,

		minLimit:       1,
		maxLimit:       200,
		initialLimit:   20,
		maxLimitFactor: 5.0,

		alphaFunc:    util.Log10Func(3),
		betaFunc:     util.Log10Func(6),
		increaseFunc: util.Log10Func(1),
		decreaseFunc: util.Log10Func(1),
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
	c.minLimit = float64(max(1, minLimit))
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

func (c *config[R]) WithQueueing(initialRejectionFactor, maxRejectionFactor float32) Builder[R] {
	c.initialRejectionFactor = float64(initialRejectionFactor)
	c.maxRejectionFactor = float64(maxRejectionFactor)
	return c
}

func (c *config[R]) WithMaxWaitTime(maxWaitTime time.Duration) Builder[R] {
	c.maxWaitTime = maxWaitTime
	return c
}

func (c *config[R]) WithLogger(logger *slog.Logger) Builder[R] {
	c.logger = logger
	return c
}

func (c *config[R]) OnLimitExceeded(listener func(event failsafe.ExecutionEvent[R])) Builder[R] {
	c.onExceeded = listener
	return c
}

func (c *config[R]) OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R] {
	c.limitChangedListener = listener
	return c
}

func (c *config[R]) Build() AdaptiveLimiter[R] {
	limiter := &adaptiveLimiter[R]{
		config:                c,
		semaphore:             util.NewDynamicSemaphore(int(c.initialLimit)),
		limit:                 float64(c.initialLimit),
		shortRTT:              &TDigestSample{TDigest: tdigest.NewWithCompression(100)},
		medianFilter:          util.NewMedianFilter(smoothedSamples),
		smoothedShortRTT:      util.NewEwma(smoothedSamples, warmupSamples),
		longRTT:               util.NewEwma(c.longWindowSize, warmupSamples),
		nextUpdateTime:        time.Now(),
		rttCorrelation:        util.NewCorrelationWindow(c.correlationWindowSize, warmupSamples),
		throughputCorrelation: util.NewCorrelationWindow(c.correlationWindowSize, warmupSamples),
	}
	if c.initialRejectionFactor != 0 && c.maxRejectionFactor != 0 {
		return &queueingLimiter[R]{adaptiveLimiter: limiter}
	}
	return limiter
}

func (c *config[R]) BuildPrioritized(prioritizer Prioritizer) PriorityLimiter[R] {
	if c.initialRejectionFactor == 0 && c.maxRejectionFactor == 0 {
		c.initialRejectionFactor = 2
		c.maxRejectionFactor = 3
	}
	limiter := &priorityLimiter[R]{
		queueingLimiter: c.Build().(*queueingLimiter[R]),
		prioritizer:     prioritizer,
	}
	prioritizer.register(limiter)
	return limiter
}

type limitChange int

const (
	increase limitChange = iota
	decrease
	hold
)

type TDigestSample struct {
	MinRTT      time.Duration
	MaxInflight int
	Size        uint
	*tdigest.TDigest
}

func (td *TDigestSample) Add(rtt time.Duration, inflight int) {
	if td.Size == 0 {
		td.MinRTT = rtt
		td.MaxInflight = inflight
	} else {
		td.MinRTT = min(td.MinRTT, rtt)
		td.MaxInflight = max(td.MaxInflight, inflight)
	}
	td.Size++
	td.TDigest.Add(float64(rtt), 1)
}

func (td *TDigestSample) Reset() {
	td.TDigest.Reset()
	td.MinRTT = 0
	td.MaxInflight = 0
	td.Size = 0
}

type adaptiveLimiter[R any] struct {
	*config[R]

	// Mutable state
	semaphore *util.DynamicSemaphore
	mu        sync.RWMutex

	// Guarded by mu
	limit                 float64        // The current concurrency limit
	shortRTT              *TDigestSample // Short term execution times
	medianFilter          *util.MedianFilter
	smoothedShortRTT      util.Ewma
	longRTT               util.Ewma               // Tracks long term average execution time
	nextUpdateTime        time.Time               // Tracks when the limit can next be updated
	throughputCorrelation *util.CorrelationWindow // Tracks the correlation between concurrency and throughput
	rttCorrelation        *util.CorrelationWindow // Tracks the correlation between concurrency and round trip times (RTT)
}

func (l *adaptiveLimiter[R]) AcquirePermit(ctx context.Context) (Permit, error) {
	if err := l.semaphore.Acquire(ctx); err != nil {
		return nil, err
	}
	return l.newPermit(), nil
}

func (l *adaptiveLimiter[R]) AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) (Permit, error) {
	if err := l.semaphore.AcquireWithMaxWait(ctx, maxWaitTime); err != nil {
		if errors.Is(err, util.ErrWaitExceeded) {
			err = ErrExceeded
		}
		return nil, err
	}
	return l.newPermit(), nil
}

func (l *adaptiveLimiter[R]) TryAcquirePermit() (Permit, bool) {
	if !l.semaphore.TryAcquire() {
		return nil, false
	}
	return l.newPermit(), true
}

func (l *adaptiveLimiter[R]) newPermit() Permit {
	return &recordingPermit[R]{
		limiter:         l,
		startTime:       time.Now(),
		currentInflight: l.semaphore.Used(),
	}
}

func (l *adaptiveLimiter[R]) CanAcquirePermit() bool {
	return !l.semaphore.IsFull()
}

func (l *adaptiveLimiter[R]) Limit() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return int(l.limit)
}

func (l *adaptiveLimiter[R]) Inflight() int {
	return l.semaphore.Used()
}

func (l *adaptiveLimiter[R]) Queued() int {
	return l.semaphore.Waiters()
}

func (l *adaptiveLimiter[R]) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.semaphore.SetSize(int(l.config.initialLimit))
	l.limit = float64(l.config.initialLimit)
	l.shortRTT.Reset()
	l.medianFilter.Reset()
	l.smoothedShortRTT.Reset()
	l.longRTT.Reset()
	l.nextUpdateTime = time.Now()
	l.rttCorrelation.Reset()
	l.throughputCorrelation.Reset()
}

// Records the duration of a completed execution, updating the concurrency limit if the short shortRTT window is full.
func (l *adaptiveLimiter[R]) record(now time.Time, rtt time.Duration, inflight int, dropped bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !dropped {
		l.shortRTT.Add(rtt, inflight)
	}

	if now.After(l.nextUpdateTime) && l.shortRTT.Size >= l.shortWindowMinSamples {
		quantile := l.shortRTT.Quantile(l.quantile)
		filteredRTT := l.medianFilter.Add(quantile)
		smoothedRTT := l.smoothedShortRTT.Add(filteredRTT)
		l.updateLimit(smoothedRTT, l.shortRTT.MaxInflight)
		minRTT := l.shortRTT.MinRTT
		l.shortRTT.Reset()
		minWindowTime := max(minRTT*2, l.shortWindowMinDuration)
		l.nextUpdateTime = now.Add(min(minWindowTime, l.shortWindowMaxDuration))
	}

	l.semaphore.Release()
}

// updateLimit updates the concurrency limit based on the gradient between the shortRTT and historical longRTT.
// A stability check prevents unnecessary decreases during steady state.
// A correlation adjustment prevents upward drift during overload.
func (l *adaptiveLimiter[R]) updateLimit(shortRTT float64, inflight int) {
	// Update long term RTT and calculate the queue size
	// This is the primary signal that we threshold off of to detect overload
	longRTT := l.longRTT.Add(shortRTT)
	gradient := longRTT / shortRTT
	queueSize := int(math.Ceil(l.limit * (1 - gradient)))

	// Calculate throughput correlation, throughput CV, and RTT correlation
	// These are the secondary signals that we threshold off of to detect overload
	throughput := float64(inflight) / (shortRTT / 1e9) // Convert to RPS
	throughputCorr, _, throughputCV := l.throughputCorrelation.Add(float64(inflight), throughput)
	rttCorr, _, _ := l.rttCorrelation.Add(float64(inflight), shortRTT)

	// Additional values for thresholding the limit
	overloaded := l.semaphore.IsFull()
	alpha := l.alphaFunc(int(l.limit)) // alpha is the queueSize threshold below which we increase
	beta := l.betaFunc(int(l.limit))   // beta is the queueSize threshold above which we decrease

	change, reason := computeChange(queueSize, alpha, beta, overloaded, throughputCorr, throughputCV, rttCorr)

	newLimit := l.limit
	var direction string
	switch change {
	case decrease:
		direction = "decrease"
		newLimit = l.limit - float64(l.decreaseFunc(int(l.limit)))
	case increase:
		direction = "increase"
		newLimit = l.limit + float64(l.increaseFunc(int(l.limit)))
	default:
		direction = "hold"
	}

	// Decrease the limit if needed, based on the max limit factor
	if newLimit > float64(inflight)*l.maxLimitFactor {
		direction = "decrease"
		reason = "max"
		newLimit = l.limit - float64(l.decreaseFunc(int(l.limit)))
	}

	// Clamp the limit
	if newLimit > l.maxLimit {
		if l.limit == l.maxLimit {
			direction = "hold"
			reason = "max"
		}
		newLimit = l.maxLimit
	} else if newLimit < l.minLimit {
		if l.limit == l.minLimit {
			direction = "hold"
			reason = "min"
		}
		newLimit = l.minLimit
	}

	l.logLimit(direction, reason, newLimit, gradient, queueSize, inflight, shortRTT, longRTT, rttCorr, throughput, throughputCorr, throughputCV)

	if uint(l.limit) != uint(newLimit) && l.limitChangedListener != nil {
		l.limitChangedListener(LimitChangedEvent{
			OldLimit: uint(l.limit),
			NewLimit: uint(newLimit),
		})
	}

	l.semaphore.SetSize(int(newLimit))
	l.limit = newLimit
}

func computeChange(queueSize, alpha, beta int, overloaded bool, throughputCorr, throughputCV, rttCorr float64) (change limitChange, reason string) {
	if queueSize > beta {
		// This condition handles severe overload where recent RTT significantly exceeds the baseline
		return decrease, "queue"
	} else if overloaded && throughputCorr < 0 {
		// This condition prevents runaway limit increases during moderate overload where inflight is increasing but throughput is decreasing
		return decrease, "thrptCorr"
	} else if overloaded && throughputCorr < .3 && rttCorr > .5 {
		// This condition prevents runaway limit increases during moderate overload where throughputCorr is weak and rttCorr is high
		// This indicates overload since latency is increasing with inflight, but throughput is not
		return decrease, "thrptCorrRtt"
	} else if overloaded && throughputCV < .2 && rttCorr > .5 {
		// This condition prevents runaway limit increases during moderate overload where throughputCV low and rttCorr is high
		// This indicates overload since latency is increasing with inflight, but throughput is not
		return decrease, "thrptCV"
	} else if queueSize < alpha {
		// If our queue size is sufficiently small, increase until we detect overload
		return increase, "queue"
	} else {
		// If queueSize is between alpha and beta, leave the limit unchanged
		return hold, "queue"
	}
}

func (l *adaptiveLimiter[R]) logLimit(direction, reason string, limit float64, gradient float64, queueSize, inflight int, shortRTT, longRTT, rttCorr, throughput, throughputCorr, throughputCV float64) {
	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		l.logger.Debug("limit update",
			"direction", direction,
			"reason", reason,
			"limit", fmt.Sprintf("%.2f", limit),
			"gradient", fmt.Sprintf("%.2f", gradient),
			"queueSize", fmt.Sprintf("%d", queueSize),
			"inflight", inflight,
			"shortRTT", time.Duration(shortRTT).Round(time.Microsecond),
			"longRTT", time.Duration(longRTT).Round(time.Microsecond),
			"thrpt", fmt.Sprintf("%.2f", throughput),
			"thrptCorr", fmt.Sprintf("%.2f", throughputCorr),
			"thrptCV", fmt.Sprintf("%.2f", throughputCV),
			"rttCorr", fmt.Sprintf("%.2f", rttCorr))
	}
}

func (l *adaptiveLimiter[R]) ToExecutor(_ R) any {
	e := &adaptiveExecutor[R]{
		BaseExecutor:    &policy.BaseExecutor[R]{},
		AdaptiveLimiter: l,
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
	now := time.Now()
	p.limiter.record(now, now.Sub(p.startTime), p.currentInflight, false)
}

func (p *recordingPermit[R]) Drop() {
	now := time.Now()
	p.limiter.record(now, now.Sub(p.startTime), p.currentInflight, true)
}
