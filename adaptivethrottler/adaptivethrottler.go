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

// AdaptiveThrottler throttles load probabalistically based on recent failures as described in https://sre.google/sre-book/handling-overload/
type AdaptiveThrottler[R any] interface {
	failsafe.Policy[R]

	AcquirePermit() error

	TryAcquirePermit() bool

	ThrottleProbability() float32
}

/*
Builder builds AdaptiveThrottler instances.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	failsafe.FailurePolicyBuilder[Builder[R], R]

	WithFailureRateThreshold(failureRateThreshold float32, thresholdingPeriod time.Duration) Builder[R]

	// Build returns a new AdaptiveThrottler using the builder's configuration.
	Build() AdaptiveThrottler[R]
}

type config[R any] struct {
	*policy.BaseFailurePolicy[R]

	maxRejectionProbability float32
	successRateThreshold    float32
	requestPadding          float32
	thresholdingPeriod      time.Duration
}

var _ Builder[any] = &config[any]{}

func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		BaseFailurePolicy:       &policy.BaseFailurePolicy[R]{},
		maxRejectionProbability: .9,
		successRateThreshold:    .9,
		requestPadding:          1,
		thresholdingPeriod:      time.Minute,
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

func (c *config[R]) Build() AdaptiveThrottler[R] {
	return &adaptiveThrottler[R]{
		config: c,
		Stats:  util.NewTimedStats(20, c.thresholdingPeriod, util.NewClock()),
	}
}

type adaptiveThrottler[R any] struct {
	*config[R]

	mtx                 sync.Mutex
	util.Stats                  // Guarded by mtx
	throttleProbability float32 // Guarded by mtx
}

func (t *adaptiveThrottler[R]) AcquirePermit() error {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	requests := float32(t.ExecutionCount())
	accepts := float32(t.SuccessCount())

	rejectProbability := max(0, requests-accepts/t.successRateThreshold) / (requests + t.requestPadding)
	t.throttleProbability = min(rejectProbability, t.maxRejectionProbability)
	if t.throttleProbability <= rand.Float32() {
		return nil
	}

	t.RecordFailure()
	return ErrExceeded
}

func (t *adaptiveThrottler[R]) TryAcquirePermit() bool {
	return t.AcquirePermit() == nil
}

func (t *adaptiveThrottler[R]) ThrottleProbability() float32 {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.throttleProbability
}

func (t *adaptiveThrottler[R]) ToExecutor(_ R) any {
	ate := &executor[R]{
		BaseExecutor:      &policy.BaseExecutor[R]{},
		adaptiveThrottler: t,
	}
	ate.Executor = ate
	return ate
}
