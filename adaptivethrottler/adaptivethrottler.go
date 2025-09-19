package adaptivethrottler

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
	"github.com/failsafe-go/failsafe-go/priority"
)

// ErrExceeded is returned when an execution exceeds the current failure rate.
var ErrExceeded = errors.New("failure rate exceeded")

const executionPadding = 1

// AdaptiveThrottler throttles load probabalistically based on recent failures.
// This approach is described in the Google SRE book: https://sre.google/sre-book/handling-overload/#client-side-throttling-a7sYUg
type AdaptiveThrottler[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit to perform an execution via the throttler, returning ErrExceeded if one
	// could not be acquired.
	AcquirePermit() error

	// TryAcquirePermit attempts to acquire a permit to perform an execution via the throttler, returning whether one could be acquired.
	TryAcquirePermit() bool

	// RecordResult records an execution result as a success or failure based on the failure handling configuration.
	RecordResult(result R)

	// RecordError records an error as a success or failure based on the failure handling configuration.
	RecordError(err error)

	// RecordSuccess records an execution success.
	RecordSuccess()

	// RecordFailure records an execution failure.
	RecordFailure()
}

type Metrics interface {
	// RejectionRate returns the current rate, from 0 to 1, at which executions will be rejected, based on recent failures.
	RejectionRate() float64
}

/*
Builder builds AdaptiveThrottler instances.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	failsafe.FailurePolicyBuilder[Builder[R], R]

	// WithFailureRateThreshold configures the failure rate threshold and thresholding period for the throttler. The
	// throttler will increase rejection probability when the failure rate exceeds this threshold over the specified time
	// period. The number of executions must also exceed the executionThreshold within the thresholdingPeriod
	// before any executions will be rejected.
	// Panics if failureRateThreshold < 0 or > 1.
	WithFailureRateThreshold(failureRateThreshold float64, executionThreshold uint, thresholdingPeriod time.Duration) Builder[R]

	// WithMaxRejectionRate configures the max allowed rejection rate, which defaults to .9.
	// Panics if maxRejectionRate < 0 or > 1.
	WithMaxRejectionRate(maxRejectionRate float64) Builder[R]

	// Build returns a new AdaptiveThrottler using the builder's configuration.
	Build() AdaptiveThrottler[R]

	// BuildPrioritized returns a new PrioritizedThrottler using the builder's configuration. This prioritizes rejections of
	// executions when throttling occurs. Rejections are performed using the Prioritizer, which sets a rejection threshold
	// based on all the throttlers being used by the Prioritizer. The Prioritizer can and should be shared across all
	// throttler instances that need to coordinate prioritization.
	//
	// Prioritized rejection is disabled by default, which means no executions will block when the throttler is full.
	BuildPrioritized(prioritizer priority.Prioritizer) PriorityThrottler[R]
}

type config[R any] struct {
	*policy.BaseFailurePolicy[R]

	maxRejectionRate     float64
	successRateThreshold float64
	executionThreshold   uint
	thresholdingPeriod   time.Duration
}

var _ Builder[any] = &config[any]{}

// NewWithDefaults returns a new AdaptiveThrottler with a failureRateThreshold of .1, a thresholdingPeriod of 1 minute,
// and a maxRejectionRate of .9. To configure additional options on am AdaptiveThrottler, use NewBuilder() instead.
func NewWithDefaults[R any]() AdaptiveThrottler[R] {
	return NewBuilder[R]().Build()
}

// NewBuilder returns an AdaptiveThrottler builder that defaults to a failureRateThreshold of .1, a thresholdingPeriod of 1 minute,
// and a maxRejectionRate of .9.
func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		BaseFailurePolicy:    &policy.BaseFailurePolicy[R]{},
		maxRejectionRate:     .9,
		successRateThreshold: .9,
		thresholdingPeriod:   time.Minute,
	}
}

func (c *config[R]) HandleErrors(errs ...error) Builder[R] {
	c.BaseFailurePolicy.HandleErrors(errs...)
	return c
}

func (c *config[R]) HandleErrorTypes(errs ...any) Builder[R] {
	c.BaseFailurePolicy.HandleErrorTypes(errs...)
	return c
}

func (c *config[R]) HandleResult(result R) Builder[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *config[R]) HandleIf(predicate func(R, error) bool) Builder[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *config[R]) OnSuccess(listener func(event failsafe.ExecutionEvent[R])) Builder[R] {
	c.BaseFailurePolicy.OnSuccess(listener)
	return c
}

func (c *config[R]) OnFailure(listener func(event failsafe.ExecutionEvent[R])) Builder[R] {
	c.BaseFailurePolicy.OnFailure(listener)
	return c
}

func (c *config[R]) WithFailureRateThreshold(failureRateThreshold float64, executionThreshold uint, thresholdingPeriod time.Duration) Builder[R] {
	util.Assert(failureRateThreshold >= 0 && failureRateThreshold <= 1, "failureRateThreshold must be between 0 and 1")
	c.successRateThreshold = min(1, max(0, 1-failureRateThreshold))
	c.executionThreshold = executionThreshold
	c.thresholdingPeriod = thresholdingPeriod
	return c
}

func (c *config[R]) WithMaxRejectionRate(maxRejectionRate float64) Builder[R] {
	util.Assert(maxRejectionRate >= 0 && maxRejectionRate <= 1, "maxRejectionFactor must be between 0 and 1")
	c.maxRejectionRate = maxRejectionRate
	return c
}

func (c *config[R]) Build() AdaptiveThrottler[R] {
	return &adaptiveThrottler[R]{
		config:         *c,
		ExecutionStats: util.NewTimedStats(20, c.thresholdingPeriod, util.WallClock),
	}
}

func (c *config[R]) BuildPrioritized(p priority.Prioritizer) PriorityThrottler[R] {
	throttler := &priorityThrottler[R]{
		adaptiveThrottler: c.Build().(*adaptiveThrottler[R]),
		prioritizer:       p.(*priority.BasePrioritizer[*throttlerStats]),
	}
	throttler.prioritizer.Register(throttler.getThrottlerStats)
	return throttler
}

type adaptiveThrottler[R any] struct {
	config[R]
	mu sync.Mutex

	// Guarded by mu
	util.ExecutionStats
	rejectionRate float64
}

func (t *adaptiveThrottler[R]) AcquirePermit() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rejectionRate = computeRejectionRate(
		float64(t.ExecutionCount()),
		float64(t.SuccessCount()),
		t.successRateThreshold,
		t.maxRejectionRate,
		t.executionThreshold)

	// Check for successful acquisition
	if t.rejectionRate == 0 {
		return nil
	}
	if t.rejectionRate >= 1 || t.rejectionRate >= rand.Float64() {
		return ErrExceeded
	}
	return nil
}

func (t *adaptiveThrottler[R]) TryAcquirePermit() bool {
	return t.AcquirePermit() == nil
}

func (t *adaptiveThrottler[R]) RejectionRate() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rejectionRate
}

func (t *adaptiveThrottler[R]) RecordFailure() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ExecutionStats.RecordFailure()
}

func (t *adaptiveThrottler[R]) RecordError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recordResult(*new(R), err)
}

func (t *adaptiveThrottler[R]) RecordResult(result R) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recordResult(result, nil)
}

func (t *adaptiveThrottler[R]) RecordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ExecutionStats.RecordSuccess()
}

// Requires external locking.
func (t *adaptiveThrottler[R]) recordResult(result R, err error) {
	if t.IsFailure(result, err) {
		t.ExecutionStats.RecordFailure()
	} else {
		t.ExecutionStats.RecordSuccess()
	}
}

func (t *adaptiveThrottler[R]) ToExecutor(_ R) any {
	ate := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{
			BaseFailurePolicy: t.BaseFailurePolicy,
		},
		adaptiveThrottler: t,
	}
	ate.Executor = ate
	return ate
}

// Computes a rejection rate as described in the SRE book: https://sre.google/sre-book/handling-overload/#client-side-throttling-a7sYUg
// The rejection rate ramps up rejections once the success rate falls below a threshold.
func computeRejectionRate(executions, successes, successRateThreshold, maxRejectionRate float64, executionThreshold uint) float64 {
	if uint(executions) < executionThreshold {
		return 0
	}

	// The max number of executions we should receive, given the successes and expected success rate threshold
	maxAllowedExecutions := successes / successRateThreshold
	// How many extra executions we processed beyond the max allowed
	excessExecutions := max(0, executions-maxAllowedExecutions)
	rejectionRate := excessExecutions / (executions + executionPadding)
	return min(rejectionRate, maxRejectionRate)
}
