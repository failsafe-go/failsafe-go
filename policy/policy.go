package policy

import (
	"errors"
	"reflect"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
)

// BaseFailurePolicy provides a base for implementing FailurePolicyBuilder.
type BaseFailurePolicy[S any, R any] struct {
	Self S
	// Indicates whether errors are checked by a configured failure condition
	errorsChecked bool
	// Conditions that determine whether an execution is a failure
	failureConditions []func(result R, err error) bool
	onSuccess         func(failsafe.ExecutionEvent[R])
	onFailure         func(failsafe.ExecutionEvent[R])
}

func (p *BaseFailurePolicy[S, R]) HandleErrors(errs ...error) S {
	for _, target := range errs {
		t := target
		p.failureConditions = append(p.failureConditions, func(r R, actualErr error) bool {
			return errors.Is(actualErr, t)
		})
	}
	p.errorsChecked = true
	return p.Self
}

func (p *BaseFailurePolicy[S, R]) HandleResult(result R) S {
	p.failureConditions = append(p.failureConditions, func(r R, err error) bool {
		return reflect.DeepEqual(r, result)
	})
	return p.Self
}

func (p *BaseFailurePolicy[S, R]) HandleIf(predicate func(R, error) bool) S {
	p.failureConditions = append(p.failureConditions, predicate)
	p.errorsChecked = true
	return p.Self
}

func (p *BaseFailurePolicy[S, R]) OnSuccess(listener func(event failsafe.ExecutionEvent[R])) S {
	p.onSuccess = listener
	return p.Self
}

func (p *BaseFailurePolicy[S, R]) OnFailure(listener func(event failsafe.ExecutionEvent[R])) S {
	p.onFailure = listener
	return p.Self
}

func (p *BaseFailurePolicy[S, R]) IsFailure(result R, err error) bool {
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
type BaseDelayablePolicy[S any, R any] struct {
	Self      S
	Delay     time.Duration
	DelayFunc failsafe.DelayFunc[R]
}

func (d *BaseDelayablePolicy[S, R]) WithDelay(delay time.Duration) S {
	d.Delay = delay
	return d.Self
}

func (d *BaseDelayablePolicy[S, R]) WithDelayFunc(delayFunc failsafe.DelayFunc[R]) S {
	d.DelayFunc = delayFunc
	return d.Self
}

// ComputeDelay returns a computed delay else -1 if no delay could be computed.
func (d *BaseDelayablePolicy[S, R]) ComputeDelay(exec failsafe.ExecutionAttempt[R]) time.Duration {
	if exec != nil && d.DelayFunc != nil {
		return d.DelayFunc(exec)
	}
	return -1
}

// BaseAbortablePolicy provides a base for implementing policies that can be aborted or canceled.
type BaseAbortablePolicy[S any, R any] struct {
	Self S
	// Conditions that determine whether the policy should be aborted
	abortConditions []func(result R, err error) bool
}

func (c *BaseAbortablePolicy[S, R]) AbortOnResult(result R) S {
	c.abortConditions = append(c.abortConditions, func(r R, err error) bool {
		return reflect.DeepEqual(r, result)
	})
	return c.Self
}

func (c *BaseAbortablePolicy[S, R]) AbortOnErrors(errs ...error) S {
	for _, target := range errs {
		t := target
		c.abortConditions = append(c.abortConditions, func(result R, actualErr error) bool {
			return errors.Is(actualErr, t)
		})
	}
	return c.Self
}

func (c *BaseAbortablePolicy[S, R]) AbortIf(predicate func(R, error) bool) S {
	c.abortConditions = append(c.abortConditions, func(result R, err error) bool {
		return predicate(result, err)
	})
	return c.Self
}

func (c *BaseAbortablePolicy[S, R]) IsConfigured() bool {
	return len(c.abortConditions) > 0
}

func (c *BaseAbortablePolicy[S, R]) IsAbortable(result R, err error) bool {
	return util.AppliesToAny(c.abortConditions, result, err)
}
