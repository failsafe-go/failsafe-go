package adaptivelimiter2

import (
	"context"
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
// var ErrExceeded = errors.New("limit exceeded")

const warmupSamples = 10

// AdaptiveLimiter2 is a concurrency limiter that adjusts its limit up or down based on execution time trends:
//  - When recent execution times are trending up relative to longer term execution times, the concurrency limit is decreased.
//  - When recent execution times are trending down relative to longer term execution times, the concurrency limit is increased.
//
// To accomplish this, short-term average execution times are tracked and regularly compared to a weighted moving average of
// longer-term execution times. Limit increases are additionally controlled to ensure they don't increase execution times. Any
// executions in excess of the limit will be rejected with ErrExceeded.
//
// By default, an AdaptiveLimiter2 will converge on a concurrency limit that represents the capacity of the machine it's
// running on, and avoids having executions block. Since enforcing a limit without allowing for blocking is too strict in
// some cases and may cause unexpected rejections, optional blocking of executions when the limiter is full can be
// enabled by configuring a maxExecutionTime.
//
// When blocking is enabled and the limiter is full, execution block up to the configures maxExecutionTime based on an
// estimated execution time for incoming requests. Estimated execution time considers the current number of blocked
// requests, the current limit, and the long-term average execution time.
//
// R is the execution result type. This type is concurrency safe.
type AdaptiveLimiter2[R any] interface {
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

	// CanAcquirePermit returns whether it's currently possible to acquire a permit.
	CanAcquirePermit() bool

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
Builder builds AdaptiveLimiter2 instances.

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

	// OnLimitChanged configures a listener to be called with the limit changes.
	OnLimitChanged(listener func(event LimitChangedEvent)) Builder[R]

	// Build returns a new AdaptiveLimiter2 using the builder's configuration.
	Build() AdaptiveLimiter2[R]
}

// LimitChangedEvent indicates an AdaptiveLimiter2's limit has changed.
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
	variationWindowSize    uint

	minLimit        float64
	maxLimit        float64
	initialLimit    uint
	maxLimitFactor  float64
	smoothingFactor float64

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
		shortWindowMinDuration: time.Second,
		shortWindowMaxDuration: time.Second,
		shortWindowMinSamples:  1,
		longWindowSize:         60,
		correlationWindowSize:  20,
		variationWindowSize:    8,
		minLimit:               1,
		maxLimit:               200,
		initialLimit:           20,
		maxLimitFactor:         5.0,
		smoothingFactor:        0.1,

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
	c.variationWindowSize = size
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

func (c *config[R]) Build() AdaptiveLimiter2[R] {
	adaptive := &adaptiveLimiter2[R]{
		config:    c,
		semaphore: util.NewDynamicSemaphore(int64(c.initialLimit)),
		limit:     float64(c.initialLimit),
		// shortRTT:         newRTTWindow(),
		shortRTT:              &td{TDigest: tdigest.NewWithCompression(100)},
		longRTT:               util.NewEWMA(c.longWindowSize, warmupSamples),
		nextUpdateTime:        time.Now(),
		rttVariation:          newVariationWindow(c.variationWindowSize),
		rttCorrelation:        newCorrelationWindow(c.correlationWindowSize, warmupSamples),
		throughputCorrelation: newCorrelationWindow(c.correlationWindowSize, warmupSamples),
	}
	if c.maxExecutionTime != 0 {
		return &blockingLimiter[R]{
			adaptiveLimiter2: adaptive,
		}
	}
	return adaptive
}

type adaptiveLimiter2[R any] struct {
	*config[R]

	// Mutable state
	semaphore *util.DynamicSemaphore
	mu        sync.Mutex

	// Guarded by mu
	limit float64 // The current concurrency limit
	// shortRTT         *rttWindow         // Tracks short term average execution time (round trip time)
	shortRTT *td
	longRTT  util.MovingAverage // Tracks long term average execution time
	// targetRTT             float64
	nextUpdateTime        time.Time          // Tracks when the limit can next be updated
	rttVariation          *variationWindow   // Tracks the variation of execution times
	rttCorrelation        *correlationWindow // Tracks the correlation between concurrency and execution times
	throughputCorrelation *correlationWindow // Tracks the correlation between concurrency and throughput

	remainingAdjustments uint
}

func (l *adaptiveLimiter2[R]) AcquirePermit(ctx context.Context) (Permit, error) {
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

func (l *adaptiveLimiter2[R]) TryAcquirePermit() (Permit, bool) {
	if !l.semaphore.TryAcquire() {
		return nil, false
	}
	return &recordingPermit[R]{
		limiter:         l,
		startTime:       time.Now(),
		currentInflight: l.semaphore.Inflight(),
	}, true
}

func (l *adaptiveLimiter2[R]) CanAcquirePermit() bool {
	return !l.semaphore.IsFull()
}

func (l *adaptiveLimiter2[R]) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int(l.limit)
}

func (l *adaptiveLimiter2[R]) Inflight() int {
	return l.semaphore.Inflight()
}

func (l *adaptiveLimiter2[R]) Blocked() int {
	return 0
}

// Records the duration of a completed execution, updating the concurrency limit if the short shortRTT window is full.
func (l *adaptiveLimiter2[R]) record(startTime time.Time, inflight int, dropped bool) {
	l.semaphore.Release()
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	rtt := now.Sub(startTime)
	if !dropped {
		l.shortRTT.add(rtt)
	}

	if now.After(l.nextUpdateTime) && l.shortRTT.size >= l.shortWindowMinSamples {
		l.updateLimit(l.shortRTT.Quantile(.9), inflight)
		minRTT := l.shortRTT.minRTT
		l.shortRTT.reset()
		minWindowTime := max(minRTT*2, l.shortWindowMinDuration)
		l.nextUpdateTime = now.Add(min(minWindowTime, l.shortWindowMaxDuration))
	}
}

// updateLimit updates the concurrency limit based on the gradient between the shortRTT and historical longRTT.
// A stability check prevents unnecessary decreases during steady state.
// A correlation adjustment prevents upward drift during overload.
func (l *adaptiveLimiter2[R]) updateLimit(shortRTT float64, inflight int) {
	// if l.remainingAdjustments == 0 {
	// 	l.targetRTT = shortRTT
	// 	l.remainingAdjustments = 20
	// }
	// l.remainingAdjustments--

	// Update long term RTT and calculate the initial gradient
	longRTT := l.longRTT.Add(shortRTT)
	// longRTT := l.targetRTT
	gradient := longRTT / shortRTT
	queueSize := int(math.Ceil(float64(inflight) * (1 - gradient)))

	// Calculate RTT variation and correlation with inflight
	rttVariation := l.rttVariation.add(shortRTT)

	rttCorr, _, _ := l.rttCorrelation.add(float64(inflight), shortRTT/1e6)
	throughput := float64(inflight) / (shortRTT / 1e6)
	throughputCorr, _, throughputVariation := l.throughputCorrelation.add(float64(inflight), throughput)
	// throughputStalled := throughputVariation < .05 || throughputCorr < 0
	// throughputVariation := l.throughputCorrelation.xSamples

	alpha := l.alphaFunc(int(l.limit))
	beta := l.betaFunc(int(l.limit))
	newLimit := l.limit
	direction := "leaving"

	// Clamp the gradient
	gradient = max(0.5, min(1.5, gradient))

	decrease := false

	if queueSize > beta { // severe overload
		direction = "decreasing beta"
		decrease = true
		// If gradient would decrease limit and either the adjustment is small or RTT is stable, maintain the current limit
		// if rttVariation < 0.05 {
		// 	l.logLimit("limit stable", l.limit, "holding", queueSize, inflight, shortRTT, longRTT, throughput, rttVariation, rttCorr, throughputCorr, gradient)
		// 	return
		// }
	} else if rttCorr > .7 && (throughputVariation < .1 || throughputCorr < 0) { // else if throughputCorr != 0 && throughputCorr < 0 { // Moderate overload
		// TODO check that throughputCorr < 0 && rttVariation < .1
		// Sustained overload, throughput degrading - decrease aggressively
		direction = "decreasing thru"
		decrease = true
		// } else if rttCorr > 0.7 {
		// 	// Early overload, latency increasing - decrease normally
		// 	direction = "decreasing rtt"
		// 	decrease = true
	} else if queueSize < alpha {
		direction = "increasing"
		newLimit = l.limit + float64(l.increaseFunc(int(l.limit)))
	}

	if decrease {
		// Adjust, smooth, and clamp the limit based on the gradient
		//	newLimit = l.limit * gradient
		//	newLimit = util.Smooth(float64(l.limit), newLimit, l.smoothingFactor)
		newLimit = l.limit - float64(l.decreaseFunc(int(l.limit)))
	}

	// Clamp the limit based on the gradient
	newLimit = max(l.minLimit, min(l.maxLimit, newLimit))

	// Don't increase the limit beyond the max limit factor
	if float64(newLimit) > float64(inflight)*l.maxLimitFactor {
		direction = "maxed"
		l.logLimit("limit maxed", l.limit, direction, queueSize, beta, inflight, shortRTT, longRTT, throughput, rttVariation, rttCorr, throughputVariation, throughputCorr, gradient)
		return
	}

	l.logLimit("limit updated", newLimit, direction, queueSize, beta, inflight, shortRTT, longRTT, throughput, rttVariation, rttCorr, throughputVariation, throughputCorr, gradient)

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

func (l *adaptiveLimiter2[R]) logLimit(msg string, limit float64, direction string, queueSize, beta, inflight int, shortRTT, longRTT float64, throughput float64, rttVariation, rttCorr float64, throughputVariation, throughputCorr float64, gradient float64) {
	if l.logger != nil && l.logger.Enabled(nil, slog.LevelDebug) {
		l.logger.Debug(msg,
			"limit", fmt.Sprintf("%.2f", limit),
			"direction", direction,
			// "initialQueueSize", fmt.Sprintf("%d", queueSize),
			"queueSize", fmt.Sprintf("%d", queueSize),
			//	"beta", fmt.Sprintf("%d", beta),
			"inflight", inflight,
			"shortRTT", fmt.Sprintf("%.2f", shortRTT/1e6),
			"longRTT", fmt.Sprintf("%.2f", longRTT/1e6),
			"rttVar", fmt.Sprintf("%.3f", rttVariation),
			"rttCorr", fmt.Sprintf("%.2f", rttCorr),
			"throughput", fmt.Sprintf("%.2f", throughput),
			"throughputVar", fmt.Sprintf("%.2f", throughput),
			"throughputCorr", fmt.Sprintf("%.2f", throughputCorr),
			"gradient", fmt.Sprintf("%.2f", gradient))
	}
}

func (l *adaptiveLimiter2[R]) ToExecutor(_ R) any {
	e := &adaptiveExecutor[R]{
		BaseExecutor:     &policy.BaseExecutor[R]{},
		adaptiveLimiter2: l,
	}
	e.Executor = e
	return e
}

type recordingPermit[R any] struct {
	limiter         *adaptiveLimiter2[R]
	startTime       time.Time
	currentInflight int
}

func (p *recordingPermit[R]) Record() {
	p.limiter.record(p.startTime, p.currentInflight, false)
}

func (p *recordingPermit[R]) Drop() {
	p.limiter.record(p.startTime, p.currentInflight, true)
}

type rttWindow struct {
	minRTT time.Duration
	rttSum time.Duration
	size   uint
}

func newRTTWindow() *rttWindow {
	return &rttWindow{
		minRTT: 24 * time.Hour,
	}
}

func (w *rttWindow) add(rtt time.Duration) {
	w.minRTT = min(w.minRTT, rtt)
	w.rttSum += rtt
	w.size++
}

func (w *rttWindow) average() time.Duration {
	if w.size == 0 {
		return 0
	}
	return w.rttSum / time.Duration(w.size)
}

type td struct {
	minRTT time.Duration
	size   uint
	*tdigest.TDigest
}

func (td *td) add(rtt time.Duration) {
	td.Add(float64(rtt), 1)
	td.minRTT = min(td.minRTT, rtt)
	td.size++
}

func (td *td) reset() {
	td.Reset()
	td.minRTT = 0
	td.size = 0
}

type rollingSum struct {
	samples    []float64
	size       int
	index      int
	sum        float64
	sumSquares float64
}

// add adds the value to the window if it's non-zero, updates the sum and sumSquares, and returns the mean, variance,
// and coefficient of variation for the samples in the window, along with the old value, and whether the window was full.
// Returns NaN for the cv if there are < 2 samples, the variance is < 0, or the mean is 0.
func (w *rollingSum) addToSum(value float64) (mean, variance, cv, oldValue float64, full bool) {
	if value != 0 {
		if w.size < len(w.samples) {
			w.size++
		} else {
			// Remove old value
			oldValue = w.samples[w.index]
			w.sum -= oldValue
			w.sumSquares -= oldValue * oldValue
			full = true
		}

		// Add new value
		w.samples[w.index] = value
		w.sum += value
		w.sumSquares += value * value

		w.index = (w.index + 1) % len(w.samples)
	}

	// Require at least 2 values to return a result
	if w.size < 2 {
		return math.NaN(), math.NaN(), math.NaN(), oldValue, full
	}

	mean = w.sum / float64(w.size)
	variance = (w.sumSquares / float64(w.size)) - (mean * mean)
	if variance < 0 || mean == 0 {
		return math.NaN(), math.NaN(), math.NaN(), oldValue, full
	}

	// Calculate coefficient of variation (relative variance), which gives us variance as a percentage of the mean
	cv = math.Sqrt(variance) / mean
	return mean, variance, cv, oldValue, full
}

type variationWindow struct {
	*rollingSum
}

func newVariationWindow(capacity uint) *variationWindow {
	return &variationWindow{
		rollingSum: &rollingSum{samples: make([]float64, capacity)},
	}
}

// add adds the value to the window if it's non-zero and returns the coefficient of variation for the samples in the window.
// Returns 1 if there are < 2 samples, the variance is < 0, or the mean is 0.
func (w *variationWindow) add(value float64) float64 {
	_, _, cv, _, _ := w.addToSum(value)
	if math.IsNaN(cv) {
		return 1.0
	}
	return cv
}

type correlationWindow struct {
	warmupSamples uint8
	xSamples      *rollingSum
	ySamples      *rollingSum
	sumXY         float64
}

func newCorrelationWindow(capacity uint, warmupSamples uint8) *correlationWindow {
	return &correlationWindow{
		warmupSamples: warmupSamples,
		xSamples:      &rollingSum{samples: make([]float64, capacity)},
		ySamples:      &rollingSum{samples: make([]float64, capacity)},
	}
}

// add adds the values to the window and returns the current correlation coefficient.
// Returns a value between 0 and 1 when a correlation between increasing x and y values is present.
// Returns a value between -1 and 0 when a correlation between increasing x and decreasing y values is present.
// Returns 0 if < warmup or low variation (< .01)
func (w *correlationWindow) add(x, y float64) (correlation, cvX, cvY float64) {
	meanX, varX, cvX, oldX, full := w.xSamples.addToSum(x)
	meanY, varY, cvY, oldY, _ := w.ySamples.addToSum(y)
	size := w.xSamples.size

	if full {
		// Remove old value
		w.sumXY -= oldX * oldY
	}

	// Add new value
	w.sumXY += x * y

	if math.IsNaN(cvX) || math.IsNaN(cvY) {
		return 0, 0, 0
	}

	// Ignore weak correlations
	if w.xSamples.size < int(w.warmupSamples) {
		return 0, 0, 0
	}

	// Ignore measurements that vary by less than 1%
	minCV := 0.01
	if cvX < minCV || cvY < minCV {
		return 0, cvX, cvY
	}

	covariance := (w.sumXY / float64(size)) - (meanX * meanY)
	correlation = covariance / (math.Sqrt(varX) * math.Sqrt(varY))

	return correlation, cvX, cvY
}
