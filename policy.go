package failsafe

import (
	"errors"
	"reflect"
	"time"

	"failsafe/internal/util"
)

// Policy handles execution failures.
type Policy[R any] interface {
	// ToExecutor returns a PolicyExecutor capable of handling an execution for the Policy.
	ToExecutor() PolicyExecutor[R]
}

// ListenablePolicyBuilder configures listeners for a Policy execution result.
type ListenablePolicyBuilder[S any, R any] interface {
	// OnSuccess registers the listener to be called when the policy succeeds in handling an execution. This means that the supplied
	// execution either succeeded, or if it failed, the policy was able to produce a successful result.
	OnSuccess(func(event ExecutionCompletedEvent[R])) S

	// OnFailure registers the listener to be called when the policy fails to handle an error. This means that not only was the supplied
	// execution considered a failure by the policy, but that the policy was unable to produce a successful result.
	OnFailure(func(event ExecutionCompletedEvent[R])) S
}

/*
FailurePolicyBuilder builds a Policy that allows configurable conditions to determine whether an execution is a failure.
  - By default, any error is considered a failure and will be handled by the policy. You can override this by specifying your own handle
    conditions. The default error handling condition will only be overridden by another condition that handles errors such as Handle
    or HandleIf. Specifying a condition that only handles results, such as HandleResult or HandleResultIf will not replace the default
    error handling condition.
  - If multiple handle conditions are specified, any condition that matches an execution result or error will trigger policy handling.
*/
type FailurePolicyBuilder[S any, R any] interface {
	// Handle specifies the errors to handle as failures. Any errors that evaluate to true for errors.Is and the execution error will be handled.
	Handle(errors ...error) S

	// HandleIf specifies that a failure has occurred if the failurePredicate matches the error.
	HandleIf(errorPredicate func(error) bool) S

	// HandleResult specifies the results to handle as failures. Any result that evaluates to true for reflect.DeepEqual and the execution
	// result will be handled. This method is only considered when a result is returned from an execution, not when an error is returned.
	HandleResult(result R) S

	// HandleResultIf specifies that a failure has occurred if the resultPredicate matches the execution result. This method is only
	// considered when a result is returned from an execution, not when an error is returned. To handle results or errors with the same
	// condition, use HandleAllIf.
	HandleResultIf(resultPredicate func(R) bool) S

	// HandleAllIf specifies that a failure has occurred if the predicate matches the execution result.
	HandleAllIf(predicate func(R, error) bool) S
}

// DelayFunction returns a duration to delay for, given an Execution.
type DelayFunction[R any] func(exec *Execution[R]) time.Duration

// DelayablePolicyBuilder builds policies that can be delayed between executions.
type DelayablePolicyBuilder[S any, R any] interface {
	// WithDelay configures the time to delay between execution attempts.
	WithDelay(delay time.Duration) S

	// WithDelayFn accepts a function that configures the time to delay before the next execution attempt.
	WithDelayFn(delayFn DelayFunction[R]) S
}

// BaseListenablePolicy provides a base for implementing ListenablePolicyBuilder.
//
// Part of the Failsafe-go SPI.
type BaseListenablePolicy[R any] struct {
	successListener func(ExecutionCompletedEvent[R])
	failureListener func(ExecutionCompletedEvent[R])
}

func (bp *BaseListenablePolicy[R]) OnSuccess(listener func(event ExecutionCompletedEvent[R])) {
	bp.successListener = listener
}

func (bp *BaseListenablePolicy[R]) OnFailure(listener func(event ExecutionCompletedEvent[R])) {
	bp.failureListener = listener
}

// BaseFailurePolicy provides a base for implementing FailurePolicyBuilder.
//
// Part of the Failsafe-go SPI.
type BaseFailurePolicy[R any] struct {
	// Indicates whether errors are checked by a configured failure condition
	errorsChecked bool
	// Conditions that determine whether an execution is a failure
	failureConditions []func(result R, err error) bool
}

func (p *BaseFailurePolicy[R]) Handle(errs []error) {
	for _, err := range errs {
		p.failureConditions = append(p.failureConditions, func(r R, actualErr error) bool {
			return errors.Is(actualErr, err)
		})
	}
	p.errorsChecked = true
}

func (p *BaseFailurePolicy[R]) HandleIf(predicate func(error) bool) {
	p.failureConditions = append(p.failureConditions, func(r R, err error) bool {
		if err == nil {
			return false
		}
		return predicate(err)
	})
	p.errorsChecked = true
}

func (p *BaseFailurePolicy[R]) HandleResult(result R) {
	p.failureConditions = append(p.failureConditions, func(r R, err error) bool {
		return reflect.DeepEqual(r, result)
	})
}

func (p *BaseFailurePolicy[R]) HandleResultIf(resultPredicate func(R) bool) {
	p.failureConditions = append(p.failureConditions, func(r R, err error) bool {
		return resultPredicate(r)
	})
}

func (p *BaseFailurePolicy[R]) HandleAllIf(predicate func(R, error) bool) {
	p.failureConditions = append(p.failureConditions, predicate)
	p.errorsChecked = true
}

func (p *BaseFailurePolicy[R]) IsFailure(result R, err error) bool {
	if len(p.failureConditions) == 0 {
		return err != nil
	}
	if util.AppliesToAny(p.failureConditions, result, err) {
		return true
	}

	// Fail by default if an error exists and was not checked by a condition
	return err != nil && !p.errorsChecked
}

// BaseDelayablePolicy provides a base for implementing DelayablePolicyBuilder.
//
// Part of the Failsafe-go SPI.
type BaseDelayablePolicy[R any] struct {
	Delay   time.Duration
	DelayFn DelayFunction[R]
}

func (d *BaseDelayablePolicy[R]) WithDelay(delay time.Duration) {
	d.Delay = delay
}

func (d *BaseDelayablePolicy[R]) WithDelayFn(delayFn DelayFunction[R]) {
	d.DelayFn = delayFn
}

// ComputeDelay returns a computed delay else -1 if no delay could be computed.
func (d *BaseDelayablePolicy[R]) ComputeDelay(exec *Execution[R]) time.Duration {
	if exec != nil && d.DelayFn != nil {
		return d.DelayFn(exec)
	}
	return -1
}
