package fallback

import (
	"failsafe"
)

type Fallback[R any] interface {
	failsafe.Policy[R]
}

/*
FallbackBuilder builds Fallback instances.

This type is not threadsafe.
*/
type FallbackBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[FallbackBuilder[R], R]
	failsafe.FailurePolicyBuilder[FallbackBuilder[R], R]
	OnFailedAttempt(listener func(failsafe.ExecutionAttemptedEvent[R])) FallbackBuilder[R]
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

func WithResult[R any](result R) Fallback[R] {
	return BuilderWithResult[R](result).Build()
}

func WithError[R any](err error) Fallback[R] {
	return BuilderWithError[R](err).Build()
}

func WithErrorFn[R any](errorFn func(error) error) Fallback[R] {
	return BuilderWithErrorFn[R](errorFn).Build()
}

func WithRunFn(fallbackFn func(event failsafe.ExecutionAttemptedEvent[any]) error) Fallback[any] {
	return BuilderWithRunFn(fallbackFn).Build()
}

func WithGetFn[R any](fallbackFn func(event failsafe.ExecutionAttemptedEvent[R]) (R, error)) Fallback[R] {
	return BuilderWithGetFn(fallbackFn).Build()
}

func BuilderWithResult[R any](result R) FallbackBuilder[R] {
	return BuilderWithGetFn(func(event failsafe.ExecutionAttemptedEvent[R]) (R, error) {
		return result, nil
	})
}

func BuilderWithError[R any](err error) FallbackBuilder[R] {
	return BuilderWithGetFn(func(event failsafe.ExecutionAttemptedEvent[R]) (R, error) {
		return *(new(R)), err
	})
}

func BuilderWithErrorFn[R any](errorFn func(error) error) FallbackBuilder[R] {
	return BuilderWithGetFn(func(event failsafe.ExecutionAttemptedEvent[R]) (R, error) {
		return *(new(R)), errorFn(event.LastErr)
	})
}

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
