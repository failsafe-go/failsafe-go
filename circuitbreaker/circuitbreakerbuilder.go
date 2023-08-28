package circuitbreaker

import (
	"time"

	"failsafe"
)

/*
CircuitBreakerBuilder builds CircuitBreaker instances.

This type is not threadsafe.
*/
type CircuitBreakerBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[CircuitBreakerBuilder[R], R]
	failsafe.FailurePolicyBuilder[CircuitBreakerBuilder[R], R]
	failsafe.DelayablePolicyBuilder[CircuitBreakerBuilder[R], R]
	OnClose(func(StateChangedEvent)) CircuitBreakerBuilder[R]
	OnOpen(func(StateChangedEvent)) CircuitBreakerBuilder[R]
	OnHalfOpen(func(StateChangedEvent)) CircuitBreakerBuilder[R]
	WithFailureThreshold(ThresholdConfig) CircuitBreakerBuilder[R]
	WithSuccessThreshold(ThresholdConfig) CircuitBreakerBuilder[R]
	Build() CircuitBreaker[R]
}

var _ CircuitBreakerBuilder[any] = &circuitBreakerConfig[any]{}

func NewCountBasedThreshold(threshold uint, thresholdingCapacity uint) ThresholdConfig {
	return ThresholdConfig{
		threshold:            threshold,
		thresholdingCapacity: thresholdingCapacity,
	}
}

func NewTimeBasedThreshold(threshold uint, thresholdingPeriod time.Duration) ThresholdConfig {
	return ThresholdConfig{
		threshold:          threshold,
		executionThreshold: threshold,
		thresholdingPeriod: thresholdingPeriod,
	}
}

func NewRateBasedThreshold(rateThreshold uint, executionThreshold uint, thresholdingPeriod time.Duration) ThresholdConfig {
	return ThresholdConfig{
		rateThreshold:      rateThreshold,
		executionThreshold: executionThreshold,
		thresholdingPeriod: thresholdingPeriod,
	}
}

func OfDefaults[R any]() CircuitBreaker[R] {
	return BuilderForResult[R]().Build()
}

func Builder() CircuitBreakerBuilder[any] {
	return BuilderForResult[any]()
}

func BuilderForResult[R any]() CircuitBreakerBuilder[R] {
	return &circuitBreakerConfig[R]{
		BaseListenablePolicy: &failsafe.BaseListenablePolicy[R]{},
		BaseFailurePolicy:    &failsafe.BaseFailurePolicy[R]{},
		BaseDelayablePolicy: &failsafe.BaseDelayablePolicy[R]{
			Delay: 1 * time.Minute,
		},
		failureThresholdConfig: ThresholdConfig{
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
	c.failureThresholdConfig = thresholdConfig
	return c
}

func (c *circuitBreakerConfig[R]) WithSuccessThreshold(thresholdConfig ThresholdConfig) CircuitBreakerBuilder[R] {
	c.successThresholdConfig = thresholdConfig
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
