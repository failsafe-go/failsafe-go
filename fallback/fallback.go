package fallback

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/spi"
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

	// OnComplete registers the listener to be called when a Fallback has completed handling a failure.
	OnComplete(listener func(event failsafe.ExecutionCompletedEvent[R])) FallbackBuilder[R]

	// Build returns a new Fallback using the builder's configuration.
	Build() Fallback[R]
}

type fallbackConfig[R any] struct {
	*spi.BaseFailurePolicy[R]
	fn         func(failsafe.Execution[R]) (R, error)
	onComplete func(failsafe.ExecutionCompletedEvent[R])
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

// WithFn returns a Fallback for execution result type R that uses fallbackFn to handle a failed execution.
func WithFn[R any](fallbackFn func(exec failsafe.Execution[R]) (R, error)) Fallback[R] {
	return BuilderWithFn(fallbackFn).Build()
}

// BuilderWithResult returns a FallbackBuilder for execution result type R which builds Fallbacks that return the result
// when an execution fails.
func BuilderWithResult[R any](result R) FallbackBuilder[R] {
	return BuilderWithFn(func(exec failsafe.Execution[R]) (R, error) {
		return result, nil
	})
}

// BuilderWithError returns a FallbackBuilder for execution result type R which builds Fallbacks that return the error
// when an execution fails.
func BuilderWithError[R any](err error) FallbackBuilder[R] {
	return BuilderWithFn(func(exec failsafe.Execution[R]) (R, error) {
		return *(new(R)), err
	})
}

// BuilderWithFn returns a FallbackBuilder for execution result type R which builds Fallbacks that use the fallbackFn to
// handle failed executions.
func BuilderWithFn[R any](fallbackFn func(exec failsafe.Execution[R]) (R, error)) FallbackBuilder[R] {
	return &fallbackConfig[R]{
		BaseFailurePolicy: &spi.BaseFailurePolicy[R]{},
		fn:                fallbackFn,
	}
}

func (c *fallbackConfig[R]) HandleErrors(errs ...error) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleErrors(errs...)
	return c
}

func (c *fallbackConfig[R]) HandleResult(result R) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *fallbackConfig[R]) HandleIf(predicate func(R, error) bool) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *fallbackConfig[R]) OnSuccess(listener func(event failsafe.ExecutionEvent[R])) FallbackBuilder[R] {
	c.BaseFailurePolicy.OnSuccess(listener)
	return c
}

func (c *fallbackConfig[R]) OnFailure(listener func(event failsafe.ExecutionEvent[R])) FallbackBuilder[R] {
	c.BaseFailurePolicy.OnFailure(listener)
	return c
}

func (c *fallbackConfig[R]) OnComplete(listener func(event failsafe.ExecutionCompletedEvent[R])) FallbackBuilder[R] {
	c.onComplete = listener
	return c
}

func (c *fallbackConfig[R]) Build() Fallback[R] {
	fbCopy := *c
	return &fallback[R]{
		config: &fbCopy, // TODO copy base fields
	}
}

func (fb *fallback[R]) ToExecutor(policyIndex int) any {
	fbe := &fallbackExecutor[R]{
		BasePolicyExecutor: &spi.BasePolicyExecutor[R]{
			BaseFailurePolicy: fb.config.BaseFailurePolicy,
			PolicyIndex:       policyIndex,
		},
		fallback: fb,
	}
	fbe.PolicyExecutor = fbe
	return fbe
}
