package adaptivethrottler

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

// ErrExceeded is returned when an execution exceeds the current limit.
var ErrExceeded = errors.New("limit exceeded")

// AdaptiveThrottler throttles load probabalistically based on recent failures.
// This approach is described in the Google SRE book: https://sre.google/sre-book/handling-overload/#client-side-throttling-a7sYUg
type AdaptiveThrottler[R any] interface {
	failsafe.Policy[R]
	Metrics

	// AcquirePermit attempts to acquire a permit to perform an execution via the throttler, returning ErrExceeded if one
	// could not be acquired.
	AcquirePermit() error

	// CanAcquirePermit returns true if an execution could be performed via the throttler.
	CanAcquirePermit() bool

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
	RejectionRate() float32
}

/*
Builder builds AdaptiveThrottler instances.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	failsafe.FailurePolicyBuilder[Builder[R], R]

	// WithFailureRateThreshold configures the failure rate threshold and thresholding period for the throttler.
	// The throttler will increase rejection probability when the failure rate exceeds this threshold over the
	// specified time period.
	WithFailureRateThreshold(failureRateThreshold float32, thresholdingPeriod time.Duration) Builder[R]

	// WithMaxRejectionRate configures the max allowed rejection rate, which defaults to .9.
	WithMaxRejectionRate(maxRejectionRate float32) Builder[R]

	// Build returns a new AdaptiveThrottler using the builder's configuration.
	Build() AdaptiveThrottler[R]
}

type config[R any] struct {
	*policy.BaseFailurePolicy[R]

	maxRejectionRate     float32
	successRateThreshold float32
	requestPadding       float32
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
		requestPadding:       1,
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

func (c *config[R]) WithFailureRateThreshold(failureRateThreshold float32, thresholdingPeriod time.Duration) Builder[R] {
	c.successRateThreshold = min(1, max(0, 1-failureRateThreshold))
	c.thresholdingPeriod = thresholdingPeriod
	return c
}

func (c *config[R]) WithMaxRejectionRate(maxRejectionRate float32) Builder[R] {
	c.maxRejectionRate = maxRejectionRate
	return c
}

func (c *config[R]) Build() AdaptiveThrottler[R] {
	return &adaptiveThrottler[R]{
		config:         c,
		ExecutionStats: util.NewTimedStats(20, c.thresholdingPeriod, util.NewClock()),
	}
}

type adaptiveThrottler[R any] struct {
	*config[R]
	mu sync.Mutex

	// Guarded by mu
	util.ExecutionStats
	rejectionRate float32
}

func (t *adaptiveThrottler[R]) AcquirePermit() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	requests := float32(t.ExecutionCount())
	accepts := float32(t.SuccessCount())
	t.rejectionRate = computeRejectionRate(requests, accepts, t.successRateThreshold, t.maxRejectionRate, t.requestPadding)

	// Check for successful acquisition
	if t.rejectionRate <= rand.Float32() {
		return nil
	}

	return ErrExceeded
}

func computeRejectionRate(requests, accepts, successRateThreshold, maxRejectionRate, requestPadding float32) float32 {
	rejectionRate := max(0, requests-accepts/successRateThreshold) / (requests + requestPadding)
	return min(rejectionRate, maxRejectionRate)
}

func (t *adaptiveThrottler[R]) CanAcquirePermit() bool {
	return t.AcquirePermit() == nil
}

func (t *adaptiveThrottler[R]) RejectionRate() float32 {
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
