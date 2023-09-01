package fallback

import (
	"failsafe"
)

// Fallback is a Policy that handles failures using a fallback function, result, or error.
//
// This type is concurrency safe.
type Fallback[R any] interface {
	failsafe.Policy[R]
}

/*
FallbackBuilder builds Fallback instances.
  - By default, any error is considered a failure and will be handled by the policy. You can override this by specifying your own handle
    conditions. The default error handling condition will only be overridden by another condition that handles errors such as Handle
    or HandleIf. Specifying a condition that only handles results, such as HandleResult or HandleResultIf will not replace the default
    error handling condition.
  - If multiple handle conditions are specified, any condition that matches an execution result or error will trigger policy handling.

This type is not concurrency safe.
*/
type FallbackBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[FallbackBuilder[R], R]
	failsafe.FailurePolicyBuilder[FallbackBuilder[R], R]

	// OnFailedAttempt registers the listener to be called when the last execution attempt prior to the fallback failed. You can also use
	// OnFailure to handle a failure in a fallback function itself.
	OnFailedAttempt(listener func(failsafe.ExecutionAttemptedEvent[R])) FallbackBuilder[R]

	// Build returns new Fallback using the builder's configuration.
	Build() Fallback[R]
}

type fallbackConfig[R any] struct {
	*failsafe.BaseListenablePolicy[R]
	*failsafe.BaseFailurePolicy[R]
	fn                    func(event failsafe.ExecutionAttemptedEvent[R]) (R, error)
	failedAttemptListener func(failsafe.ExecutionAttemptedEvent[R])
}

var _ FallbackBuilder[any] = &fallbackConfig[any]{}

type fallback[R any] struct {
	config *fallbackConfig[R]
}

// OfResult returns a Fallback that returns the result when an execution fails.
func OfResult[R any](result R) Fallback[R] {
	return BuilderWithResult[R](result).Build()
}

// OfError returns a Fallback that returns the err when an execution fails.
func OfError[R any](err error) Fallback[R] {
	return BuilderWithError[R](err).Build()
}

// OfErrorFn returns a Fallback that uses errorFn to handle a failed execution.
func OfErrorFn[R any](errorFn func(error) error) Fallback[R] {
	return BuilderWithErrorFn[R](errorFn).Build()
}

// OfRunFn returns a Fallback that uses fallbackFn to handle a failed execution.
func OfRunFn(fallbackFn func(event failsafe.ExecutionAttemptedEvent[any]) error) Fallback[any] {
	return BuilderWithRunFn(fallbackFn).Build()
}

// OfGetFn returns a Fallback that uses fallbackFn to handle a failed execution.
func OfGetFn[R any](fallbackFn func(event failsafe.ExecutionAttemptedEvent[R]) (R, error)) Fallback[R] {
	return BuilderWithGetFn(fallbackFn).Build()
}

// BuilderWithResult returns a FallbackBuilder which builds Fallbacks that return the result when an execution fails.
func BuilderWithResult[R any](result R) FallbackBuilder[R] {
	return BuilderWithGetFn(func(event failsafe.ExecutionAttemptedEvent[R]) (R, error) {
		return result, nil
	})
}

// BuilderWithError returns a FallbackBuilder which builds Fallbacks that return the error when an execution fails.
func BuilderWithError[R any](err error) FallbackBuilder[R] {
	return BuilderWithGetFn(func(event failsafe.ExecutionAttemptedEvent[R]) (R, error) {
		return *(new(R)), err
	})
}

// BuilderWithErrorFn returns a FallbackBuilder which builds Fallbacks that use the errorFn to handle failed executions.
func BuilderWithErrorFn[R any](errorFn func(error) error) FallbackBuilder[R] {
	return BuilderWithGetFn(func(event failsafe.ExecutionAttemptedEvent[R]) (R, error) {
		return *(new(R)), errorFn(event.LastErr)
	})
}

// BuilderWithRunFn returns a FallbackBuilder which builds Fallbacks that use the fallbackFn to handle failed executions.
func BuilderWithRunFn(fallbackFn func(event failsafe.ExecutionAttemptedEvent[any]) error) FallbackBuilder[any] {
	return &fallbackConfig[any]{
		BaseListenablePolicy: &failsafe.BaseListenablePolicy[any]{},
		BaseFailurePolicy:    &failsafe.BaseFailurePolicy[any]{},
		fn: func(event failsafe.ExecutionAttemptedEvent[any]) (any, error) {
			err := fallbackFn(event)
			return *(new(any)), err
		},
	}
}

// BuilderWithGetFn returns a FallbackBuilder which builds Fallbacks that use the fallbackFn to handle failed executions.
func BuilderWithGetFn[R any](fallbackFn func(event failsafe.ExecutionAttemptedEvent[R]) (R, error)) FallbackBuilder[R] {
	return &fallbackConfig[R]{
		BaseListenablePolicy: &failsafe.BaseListenablePolicy[R]{},
		BaseFailurePolicy:    &failsafe.BaseFailurePolicy[R]{},
		fn:                   fallbackFn,
	}
}

func (c *fallbackConfig[R]) Handle(errs ...error) FallbackBuilder[R] {
	c.BaseFailurePolicy.Handle(errs)
	return c
}

func (c *fallbackConfig[R]) HandleIf(predicate func(error) bool) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *fallbackConfig[R]) HandleResult(result R) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *fallbackConfig[R]) HandleResultIf(resultPredicate func(R) bool) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleResultIf(resultPredicate)
	return c
}

func (c *fallbackConfig[R]) HandleAllIf(predicate func(R, error) bool) FallbackBuilder[R] {
	c.BaseFailurePolicy.HandleAllIf(predicate)
	return c
}

// OnFailedAttempt registers the listener to be called when an execution attempt fails.
func (c *fallbackConfig[R]) OnFailedAttempt(listener func(failsafe.ExecutionAttemptedEvent[R])) FallbackBuilder[R] {
	c.failedAttemptListener = listener
	return c
}

func (c *fallbackConfig[R]) OnSuccess(listener func(event failsafe.ExecutionCompletedEvent[R])) FallbackBuilder[R] {
	c.BaseListenablePolicy.OnSuccess(listener)
	return c
}

func (c *fallbackConfig[R]) OnFailure(listener func(event failsafe.ExecutionCompletedEvent[R])) FallbackBuilder[R] {
	c.BaseListenablePolicy.OnFailure(listener)
	return c
}

func (c *fallbackConfig[R]) Build() Fallback[R] {
	fbCopy := *c
	return &fallback[R]{
		config: &fbCopy, // TODO copy base fields
	}
}

func (fb *fallback[R]) ToExecutor() failsafe.PolicyExecutor[R] {
	rpe := fallbackExecutor[R]{
		BasePolicyExecutor: &failsafe.BasePolicyExecutor[R]{
			BaseListenablePolicy: fb.config.BaseListenablePolicy,
			BaseFailurePolicy:    fb.config.BaseFailurePolicy,
		},
		fallback: fb,
	}
	rpe.PolicyExecutor = &rpe
	return &rpe
}
