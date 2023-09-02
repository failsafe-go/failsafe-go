package circuitbreaker

import (
	"time"

	"failsafe"
	"failsafe/internal/util"
	"failsafe/spi"
)

/*
CircuitBreakerBuilder builds CircuitBreaker instances.

  - By default, any error is considered a failure and will be handled by the policy. You can override this by specifying your own handle
    conditions. The default error handling condition will only be overridden by another condition that handles error such as
    Handle or HandleIf. Specifying a condition that only handles results, such as HandleResult or HandleResultIf will not replace the default
    error handling condition.
  - If multiple handle conditions are specified, any condition that matches an execution result or error will trigger policy handling.

This type is not concurrency safe.
*/
type CircuitBreakerBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[CircuitBreakerBuilder[R], R]
	failsafe.FailurePolicyBuilder[CircuitBreakerBuilder[R], R]
	failsafe.DelayablePolicyBuilder[CircuitBreakerBuilder[R], R]

	// OnClose calls the listener when the CircuitBreaker is closed.
	OnClose(listener func(StateChangedEvent)) CircuitBreakerBuilder[R]

	// OnOpen calls the listener when the CircuitBreaker is opened.
	OnOpen(listener func(StateChangedEvent)) CircuitBreakerBuilder[R]

	// OnHalfOpen calls the listener when the CircuitBreaker is half-opened.
	OnHalfOpen(listener func(StateChangedEvent)) CircuitBreakerBuilder[R]

	// WithFailureThreshold configures failure thresholds that must be exceeded when in a CLOSED state in order to open the circuit.
	//
	// If a success threshold is not configured, the failureThreshold will also be used when the circuit breaker is in a HalfOpenState to
	// determine whether to transition back to OpenState or ClosedState.
	WithFailureThreshold(failureThreshold ThresholdConfig) CircuitBreakerBuilder[R]

	// WithSuccessThreshold configures count based success thresholding by setting the number of consecutive successful executions that
	// must occur when in a HalfOpenState in order to close the circuit, else the circuit is re-opened when a failure occurs.
	WithSuccessThreshold(successThreshold uint, successThresholdingCapacity uint) CircuitBreakerBuilder[R]

	// Build returns a new CircuitBreaker using the builder's configuration.
	Build() CircuitBreaker[R]
}

type circuitBreakerConfig[R any] struct {
	*spi.BaseListenablePolicy[R]
	*spi.BaseFailurePolicy[R]
	*spi.BaseDelayablePolicy[R]
	clock                  util.Clock
	openListener           func(StateChangedEvent)
	halfOpenListener       func(StateChangedEvent)
	closeListener          func(StateChangedEvent)
	failureThresholdConfig *thresholdConfig

	// Success config
	successThreshold            uint
	successThresholdingCapacity uint
}

type ThresholdConfig interface {
	WithExecutionThreshold(executionThreshold uint) ThresholdConfig
	getConfig() *thresholdConfig
}

type thresholdConfig struct {
	threshold            uint
	rateThreshold        uint
	thresholdingCapacity uint
	executionThreshold   uint
	thresholdingPeriod   time.Duration
}

func (c *thresholdConfig) WithExecutionThreshold(executionThreshold uint) ThresholdConfig {
	c.executionThreshold = executionThreshold
	return c
}

func (c *thresholdConfig) getConfig() *thresholdConfig {
	return c
}

var _ CircuitBreakerBuilder[any] = &circuitBreakerConfig[any]{}

func NewCountBasedThreshold(threshold uint, thresholdingCapacity uint) ThresholdConfig {
	return &thresholdConfig{
		threshold:            threshold,
		thresholdingCapacity: thresholdingCapacity,
	}
}

func NewTimeBasedThreshold(threshold uint, thresholdingPeriod time.Duration) ThresholdConfig {
	return &thresholdConfig{
		threshold:            threshold,
		thresholdingCapacity: threshold,
		executionThreshold:   threshold,
		thresholdingPeriod:   thresholdingPeriod,
	}
}

func NewRateBasedThreshold(rateThreshold uint, executionThreshold uint, thresholdingPeriod time.Duration) ThresholdConfig {
	return &thresholdConfig{
		rateThreshold:      rateThreshold,
		executionThreshold: executionThreshold,
		thresholdingPeriod: thresholdingPeriod,
	}
}

// OfDefaults creates a count based CircuitBreaker that opens after a single failure, closes after a single success, and has a 1 minute
// delay by default. To configure additional options on a CircuitBreaker, use Builder() instead.
//
// Type parameter R represents the execution result type.
func OfDefaults[R any]() CircuitBreaker[R] {
	return Builder[R]().Build()
}

// Builder creates a CircuitBreakerBuilder which by default will build a count based circuit breaker that opens after a single failure,
// closes after a single success, and has a 1 minute delay, unless configured otherwise.
//
// Type parameter R represents the execution result type.
func Builder[R any]() CircuitBreakerBuilder[R] {
	return &circuitBreakerConfig[R]{
		BaseListenablePolicy: &spi.BaseListenablePolicy[R]{},
		BaseFailurePolicy:    &spi.BaseFailurePolicy[R]{},
		BaseDelayablePolicy: &spi.BaseDelayablePolicy[R]{
			Delay: 1 * time.Minute,
		},
		clock: util.NewClock(),
		failureThresholdConfig: &thresholdConfig{
			threshold:            1,
			thresholdingCapacity: 1,
		},
	}
}

func (c *circuitBreakerConfig[R]) Build() CircuitBreaker[R] {
	breaker := &circuitBreaker[R]{
		config: c,
	}
	breaker.state = newClosedState[R](breaker)
	return breaker
}

func (c *circuitBreakerConfig[R]) Handle(errs ...error) CircuitBreakerBuilder[R] {
	c.BaseFailurePolicy.Handle(errs)
	return c
}

func (c *circuitBreakerConfig[R]) HandleIf(predicate func(error) bool) CircuitBreakerBuilder[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *circuitBreakerConfig[R]) HandleResult(result R) CircuitBreakerBuilder[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *circuitBreakerConfig[R]) HandleResultIf(resultPredicate func(R) bool) CircuitBreakerBuilder[R] {
	c.BaseFailurePolicy.HandleResultIf(resultPredicate)
	return c
}

func (c *circuitBreakerConfig[R]) HandleAllIf(predicate func(R, error) bool) CircuitBreakerBuilder[R] {
	c.BaseFailurePolicy.HandleAllIf(predicate)
	return c
}

func (c *circuitBreakerConfig[R]) WithFailureThreshold(thresholdConfig ThresholdConfig) CircuitBreakerBuilder[R] {
	c.failureThresholdConfig = thresholdConfig.getConfig()
	return c
}

func (c *circuitBreakerConfig[R]) WithSuccessThreshold(successThreshold uint, successThresholdingCapacity uint) CircuitBreakerBuilder[R] {
	c.successThreshold = successThreshold
	c.successThresholdingCapacity = successThresholdingCapacity
	return c
}

func (c *circuitBreakerConfig[R]) WithDelay(delay time.Duration) CircuitBreakerBuilder[R] {
	c.BaseDelayablePolicy.WithDelay(delay)
	return c
}

func (c *circuitBreakerConfig[R]) WithDelayFn(delayFn failsafe.DelayFunction[R]) CircuitBreakerBuilder[R] {
	c.BaseDelayablePolicy.WithDelayFn(delayFn)
	return c
}

func (c *circuitBreakerConfig[R]) OnClose(listener func(event StateChangedEvent)) CircuitBreakerBuilder[R] {
	c.closeListener = listener
	return c
}

func (c *circuitBreakerConfig[R]) OnOpen(listener func(event StateChangedEvent)) CircuitBreakerBuilder[R] {
	c.openListener = listener
	return c
}

func (c *circuitBreakerConfig[R]) OnHalfOpen(listener func(event StateChangedEvent)) CircuitBreakerBuilder[R] {
	c.halfOpenListener = listener
	return c
}

func (c *circuitBreakerConfig[R]) OnSuccess(listener func(event failsafe.ExecutionCompletedEvent[R])) CircuitBreakerBuilder[R] {
	c.BaseListenablePolicy.OnSuccess(listener)
	return c
}

func (c *circuitBreakerConfig[R]) OnFailure(listener func(event failsafe.ExecutionCompletedEvent[R])) CircuitBreakerBuilder[R] {
	c.BaseListenablePolicy.OnFailure(listener)
	return c
}
