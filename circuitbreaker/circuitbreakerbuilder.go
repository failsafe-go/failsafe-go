package circuitbreaker

import (
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/policy"
)

/*
Builder builds CircuitBreaker instances.

  - By default, any error is considered a failure and will be handled by the policy. You can override this by specifying
    your own handle conditions. The default error handling condition will only be overridden by another condition that handles
    error such as HandleErrors or HandleIf. Specifying a condition that only handles results, such as HandleResult or
    HandleResultIf will not replace the default error handling condition.
  - If multiple handle conditions are specified, any condition that matches an execution result or error will trigger policy handling.

This type is not concurrency safe.
*/
type Builder[R any] interface {
	failsafe.FailurePolicyBuilder[Builder[R], R]
	failsafe.DelayablePolicyBuilder[Builder[R], R]

	// OnStateChanged calls the listener when the CircuitBreaker state changes.
	OnStateChanged(listener func(StateChangedEvent)) Builder[R]

	// OnClose calls the listener when the CircuitBreaker state changes to closed.
	OnClose(listener func(StateChangedEvent)) Builder[R]

	// OnOpen calls the listener when the CircuitBreaker state changes to open.
	OnOpen(listener func(StateChangedEvent)) Builder[R]

	// OnHalfOpen calls the listener when the CircuitBreaker state changes to half-open.
	OnHalfOpen(listener func(StateChangedEvent)) Builder[R]

	// WithFailureThreshold configures count based failure thresholding by setting the number of consecutive failures that
	// must occur when in a ClosedState in order to open the circuit.
	//
	// If WithSuccessThreshold is not configured, the failureThreshold will also be used when the circuit breaker is in a
	// HalfOpenState to determine whether to transition back to OpenState or ClosedState.
	WithFailureThreshold(failureThreshold uint) Builder[R]

	// WithFailureThresholdRatio configures count based failure thresholding by setting the ratio of failures to executions
	// that must occur when in a ClosedState in order to open the circuit. For example: 5, 10 would open the circuit if 5 out
	// of the last 10 executions result in a failure.
	//
	// If WithSuccessThreshold is not configured, the failureThreshold and failureThresholdingCapacity will also be used when
	// the circuit breaker is in a HalfOpenState to determine whether to transition back to OpenState or ClosedState.
	WithFailureThresholdRatio(failureThreshold uint, failureThresholdingCapacity uint) Builder[R]

	// WithFailureThresholdPeriod configures time based failure thresholding by setting the number of failures that must
	// occur within the failureThresholdingPeriod when in a ClosedState in order to open the circuit.
	//
	// If WithSuccessThreshold is not configured, the failureThreshold will also be used when the circuit breaker is in a
	// HalfOpenState to determine whether to transition back to OpenState or ClosedState.
	WithFailureThresholdPeriod(failureThreshold uint, failureThresholdingPeriod time.Duration) Builder[R]

	// WithFailureRateThreshold configures time based failure rate thresholding by setting the percentage rate of failures,
	// from 1 to 100, that must occur within the rolling failureThresholdingPeriod when in a ClosedState in order to open the
	// circuit. The number of executions must also exceed the failureExecutionThreshold within the failureThresholdingPeriod
	// before the circuit will be opened.
	//
	// If WithSuccessThreshold is not configured, the failureExecutionThreshold will also be used when the circuit breaker is
	// in a HalfOpenSttate state to determine whether to transition back to open or closed.
	WithFailureRateThreshold(failureRateThreshold uint, failureExecutionThreshold uint, failureThresholdingPeriod time.Duration) Builder[R]

	// WithDelay configures the delay to wait in OpenState before transitioning to HalfOpenState.
	WithDelay(delay time.Duration) Builder[R]

	// WithDelayFunc configures a function that provides the delay to wait in OpenState before transitioning to HalfOpenState.
	WithDelayFunc(delayFunc failsafe.DelayFunc[R]) Builder[R]

	// WithSuccessThreshold configures count based success thresholding by setting the number of consecutive successful
	// executions that must occur when in a HalfOpenState in order to close the circuit, else the circuit is re-opened when a
	// failure occurs.
	WithSuccessThreshold(successThreshold uint) Builder[R]

	// WithSuccessThresholdRatio configures count based success thresholding by setting the ratio of successful executions
	// that must occur when in a HalfOpenState in order to close the circuit. For example: 5, 10 would close the circuit if 5
	// out of the last 10 executions were successful.
	WithSuccessThresholdRatio(successThreshold uint, successThresholdingCapacity uint) Builder[R]

	// Build returns a new CircuitBreaker using the builder's configuration.
	Build() CircuitBreaker[R]
}

type config[R any] struct {
	*policy.BaseFailurePolicy[R]
	*policy.BaseDelayablePolicy[R]
	clock                util.Clock
	stateChangedListener func(StateChangedEvent)
	openListener         func(StateChangedEvent)
	halfOpenListener     func(StateChangedEvent)
	closeListener        func(StateChangedEvent)

	// Failure config
	failureThreshold            uint
	failureRateThreshold        uint
	failureThresholdingCapacity uint
	failureExecutionThreshold   uint
	failureThresholdingPeriod   time.Duration

	// Success config
	successThreshold            uint
	successThresholdingCapacity uint
}

var _ Builder[any] = &config[any]{}

// NewWithDefaults creates a count based CircuitBreaker for execution result type R that opens after a single failure,
// closes after a single success, and has a 1 minute delay by default. To configure additional options on a
// CircuitBreaker, use NewBuilder() instead.
func NewWithDefaults[R any]() CircuitBreaker[R] {
	return NewBuilder[R]().Build()
}

// NewBuilder creates a Builder for execution result type R which by default will build a count based circuit
// breaker that opens after a single failure, closes after a single success, and has a 1 minute delay, unless configured
// otherwise.
func NewBuilder[R any]() Builder[R] {
	return &config[R]{
		BaseFailurePolicy: &policy.BaseFailurePolicy[R]{},
		BaseDelayablePolicy: &policy.BaseDelayablePolicy[R]{
			Delay: time.Minute,
		},
		clock:                       util.NewClock(),
		failureThreshold:            1,
		failureThresholdingCapacity: 1,
	}
}

func (c *config[R]) Build() CircuitBreaker[R] {
	breaker := &circuitBreaker[R]{
		config: c, // TODO copy base fields
	}
	breaker.state = newClosedState[R](breaker)
	return breaker
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

func (c *config[R]) WithFailureThreshold(failureThreshold uint) Builder[R] {
	return c.WithFailureThresholdRatio(failureThreshold, failureThreshold)
}

func (c *config[R]) WithFailureThresholdRatio(failureThreshold uint, failureThresholdingCapacity uint) Builder[R] {
	c.failureThreshold = failureThreshold
	c.failureThresholdingCapacity = failureThresholdingCapacity
	return c
}

func (c *config[R]) WithFailureThresholdPeriod(failureThreshold uint, failureThresholdingPeriod time.Duration) Builder[R] {
	c.failureThreshold = failureThreshold
	c.failureThresholdingCapacity = failureThreshold
	c.failureExecutionThreshold = failureThreshold
	c.failureThresholdingPeriod = failureThresholdingPeriod
	return c
}

func (c *config[R]) WithFailureRateThreshold(failureRateThreshold uint, failureExecutionThreshold uint, failureThresholdingPeriod time.Duration) Builder[R] {
	c.failureRateThreshold = failureRateThreshold
	c.failureExecutionThreshold = failureExecutionThreshold
	c.failureThresholdingPeriod = failureThresholdingPeriod
	return c
}

func (c *config[R]) WithSuccessThreshold(successThreshold uint) Builder[R] {
	return c.WithSuccessThresholdRatio(successThreshold, successThreshold)
}

func (c *config[R]) WithSuccessThresholdRatio(successThreshold uint, successThresholdingCapacity uint) Builder[R] {
	c.successThreshold = successThreshold
	c.successThresholdingCapacity = successThresholdingCapacity
	return c
}

func (c *config[R]) WithDelay(delay time.Duration) Builder[R] {
	c.BaseDelayablePolicy.WithDelay(delay)
	return c
}

func (c *config[R]) WithDelayFunc(delayFunc failsafe.DelayFunc[R]) Builder[R] {
	c.BaseDelayablePolicy.WithDelayFunc(delayFunc)
	return c
}

func (c *config[R]) OnStateChanged(listener func(event StateChangedEvent)) Builder[R] {
	c.stateChangedListener = listener
	return c
}

func (c *config[R]) OnClose(listener func(event StateChangedEvent)) Builder[R] {
	c.closeListener = listener
	return c
}

func (c *config[R]) OnOpen(listener func(event StateChangedEvent)) Builder[R] {
	c.openListener = listener
	return c
}

func (c *config[R]) OnHalfOpen(listener func(event StateChangedEvent)) Builder[R] {
	c.halfOpenListener = listener
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
