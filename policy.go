package failsafe

import (
	"errors"
	"reflect"
	"time"

	"failsafe/internal/util"
)

// Policy provides failure handling and resilience based on some configuration.
type Policy[R any] interface {
	ToExecutor() PolicyExecutor[R]
}

type ListenablePolicyBuilder[S any, R any] interface {
	OnSuccess(func(event ExecutionCompletedEvent[R])) S
	OnFailure(func(event ExecutionCompletedEvent[R])) S
}

// FailurePolicyBuilder handles failure results and conditions.
type FailurePolicyBuilder[S any, R any] interface {
	Handle(errors ...error) S
	HandleIf(errorPredicate func(error) bool) S
	HandleResult(result R) S
	HandleResultIf(resultPredicate func(R) bool) S
	HandleAllIf(predicate func(R, error) bool) S
}

type DelayFunction[R any] func(exec *Execution[R]) time.Duration

type DelayablePolicyBuilder[S any, R any] interface {
	// WithDelay configures the time to Delay between execution Attempts.
	WithDelay(delay time.Duration) S
	// WithDelayFn accepts a function that configures the time to Delay before the next execution attempt.
	WithDelayFn(delayFn DelayFunction[R]) S
}

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

// ComputeDelay returns a computed delay else -1if no delay could be computed.
func (d *BaseDelayablePolicy[R]) ComputeDelay(exec *Execution[R]) time.Duration {
	if exec != nil && d.DelayFn != nil {
		return d.DelayFn(exec)
	}
	return -1
}
