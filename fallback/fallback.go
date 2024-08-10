package fallback

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

// Fallback is a Policy that handles failures using a fallback function, result, or error.
//
// This type is concurrency safe.
type Fallback[R any] interface {
	failsafe.Policy[R]
}

/*
FallbackBuilder builds Fallback instances.
  - By default, any error is considered a failure and will be handled by the policy. You can override this by specifying
    your own handle conditions. The default error handling condition will only be overridden by another condition that
    handles errors such as HandleErrors or HandleIf. Specifying a condition that only handles results, such as HandleResult
    will not replace the default error handling condition.
  - If multiple handle conditions are specified, any condition that matches an execution result or error will trigger
    policy handling.

This type is not concurrency safe.
*/
type FallbackBuilder[R any] interface {
	failsafe.FailurePolicyBuilder[FallbackBuilder[R], R]

	// OnFallbackExecuted registers the listener to be called when a Fallback has executed. The provided event will contain
	// the execution result and error returned by the Fallback.
	OnFallbackExecuted(listener func(event failsafe.ExecutionDoneEvent[R])) FallbackBuilder[R]

	// Build returns a new Fallback using the builder's configuration.
	Build() Fallback[R]
}

type config[R any] struct {
	*policy.BaseFailurePolicy[R]
	fn                 func(failsafe.Execution[R]) (R, error)
	onFallbackExecuted func(failsafe.ExecutionDoneEvent[R])
}

var _ FallbackBuilder[any] = &config[any]{}

type fallback[R any] struct {
	*config[R]
}

// NewWithResult returns a Fallback for execution result type R that returns the result when an execution fails.
func NewWithResult[R any](result R) Fallback[R] {
	return NewBuilderWithResult[R](result).Build()
}

// NewWithError returns a Fallback for execution result type R that returns the err when an execution fails.
func NewWithError[R any](err error) Fallback[R] {
	return NewBuilderWithError[R](err).Build()
}

// NewWithFunc returns a Fallback for execution result type R that uses fallbackFunc to handle a failed execution.
func NewWithFunc[R any](fallbackFunc func(exec failsafe.Execution[R]) (R, error)) Fallback[R] {
	return NewBuilderWithFunc(fallbackFunc).Build()
}

// NewBuilderWithResult returns a FallbackBuilder for execution result type R which builds Fallbacks that return the result
// when an execution fails.
func NewBuilderWithResult[R any](result R) FallbackBuilder[R] {
	return NewBuilderWithFunc(func(exec failsafe.Execution[R]) (R, error) {
		return result, nil
	})
}

// NewBuilderWithError returns a FallbackBuilder for execution result type R which builds Fallbacks that return the error
// when an execution fails.
func NewBuilderWithError[R any](err error) FallbackBuilder[R] {
	return NewBuilderWithFunc(func(exec failsafe.Execution[R]) (R, error) {
		return *new(R), err
	})
}

// NewBuilderWithFunc returns a FallbackBuilder for execution result type R which builds Fallbacks that use the fallbackFn to
// handle failed executions.
func NewBuilderWithFunc[R any](fallbackFunc func(exec failsafe.Execution[R]) (R, error)) FallbackBuilder[R] {
	return &config[R]{
		BaseFailurePolicy: &policy.BaseFailurePolicy[R]{},
		fn:                fallbackFunc,
	}
}

func (c *config[R]) HandleErrors(errs ...error) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleErrors(errs...)
	return c
}

func (c *config[R]) HandleErrorTypes(errs ...any) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleErrorTypes(errs...)
	return c
}

func (c *config[R]) HandleResult(result R) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *config[R]) HandleIf(predicate func(R, error) bool) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *config[R]) OnSuccess(listener func(event failsafe.ExecutionEvent[R])) FallbackBuilder[R] {
	c.BaseFailurePolicy.OnSuccess(listener)
	return c
}

func (c *config[R]) OnFailure(listener func(event failsafe.ExecutionEvent[R])) FallbackBuilder[R] {
	c.BaseFailurePolicy.OnFailure(listener)
	return c
}

func (c *config[R]) OnFallbackExecuted(listener func(event failsafe.ExecutionDoneEvent[R])) FallbackBuilder[R] {
	c.onFallbackExecuted = listener
	return c
}

func (c *config[R]) Build() Fallback[R] {
	fbCopy := *c
	return &fallback[R]{
		config: &fbCopy, // TODO copy base fields
	}
}

func (fb *fallback[R]) ToExecutor(_ R) any {
	fbe := &executor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{
			BaseFailurePolicy: fb.BaseFailurePolicy,
		},
		fallback: fb,
	}
	fbe.Executor = fbe
	return fbe
}
