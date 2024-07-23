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

type fallbackConfig[R any] struct {
	*policy.BaseFailurePolicy[FallbackBuilder[R], R]
	fn                 func(failsafe.Execution[R]) (R, error)
	onFallbackExecuted func(failsafe.ExecutionDoneEvent[R])
}

var _ FallbackBuilder[any] = &fallbackConfig[any]{}

type fallback[R any] struct {
	config *fallbackConfig[R]
}

// WithResult returns a Fallback for execution result type R that returns the result when an execution fails.
func WithResult[R any](result R) Fallback[R] {
	return BuilderWithResult[R](result).Build()
}

// WithError returns a Fallback for execution result type R that returns the err when an execution fails.
func WithError[R any](err error) Fallback[R] {
	return BuilderWithError[R](err).Build()
}

// WithFunc returns a Fallback for execution result type R that uses fallbackFunc to handle a failed execution.
func WithFunc[R any](fallbackFunc func(exec failsafe.Execution[R]) (R, error)) Fallback[R] {
	return BuilderWithFunc(fallbackFunc).Build()
}

// BuilderWithResult returns a FallbackBuilder for execution result type R which builds Fallbacks that return the result
// when an execution fails.
func BuilderWithResult[R any](result R) FallbackBuilder[R] {
	return BuilderWithFunc(func(exec failsafe.Execution[R]) (R, error) {
		return result, nil
	})
}

// BuilderWithError returns a FallbackBuilder for execution result type R which builds Fallbacks that return the error
// when an execution fails.
func BuilderWithError[R any](err error) FallbackBuilder[R] {
	return BuilderWithFunc(func(exec failsafe.Execution[R]) (R, error) {
		return *new(R), err
	})
}

// BuilderWithFunc returns a FallbackBuilder for execution result type R which builds Fallbacks that use the fallbackFn to
// handle failed executions.
func BuilderWithFunc[R any](fallbackFunc func(exec failsafe.Execution[R]) (R, error)) FallbackBuilder[R] {
	b := &fallbackConfig[R]{
		BaseFailurePolicy: &policy.BaseFailurePolicy[FallbackBuilder[R], R]{},
		fn:                fallbackFunc,
	}
	b.BaseFailurePolicy.Self = b
	return b
}

func (c *fallbackConfig[R]) OnFallbackExecuted(listener func(event failsafe.ExecutionDoneEvent[R])) FallbackBuilder[R] {
	c.onFallbackExecuted = listener
	return c
}

func (c *fallbackConfig[R]) Build() Fallback[R] {
	fbCopy := *c
	return &fallback[R]{
		config: &fbCopy, // TODO copy base fields
	}
}

func (fb *fallback[R]) ToExecutor(_ R) any {
	fbe := &fallbackExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[FallbackBuilder[R], R]{
			BaseFailurePolicy: fb.config.BaseFailurePolicy,
		},
		fallback: fb,
	}
	fbe.Executor = fbe
	return fbe
}
