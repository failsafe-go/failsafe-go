package loadlimiter

import (
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

type Signal interface{}

type FailureSignal[R any] interface {
	Signal
	failsafe.FailurePolicyBuilder[FailureSignal[R], R]
	WithFailureRateThreshold(failureRateThreshold uint, failureExecutionThreshold uint, failureThresholdingPeriod time.Duration) FailureSignal[R]
}

type failureSignal[R any] struct {
	*policy.BaseFailurePolicy[R]
	failureRateThreshold      uint
	failureExecutionThreshold uint
	failureThresholdingPeriod time.Duration
}

func (c *failureSignal[R]) HandleErrors(errs ...error) FailureSignal[R] {
	c.BaseFailurePolicy.HandleErrors(errs...)
	return c
}

func (c *failureSignal[R]) HandleResult(result R) FailureSignal[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *failureSignal[R]) HandleIf(predicate func(R, error) bool) FailureSignal[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *failureSignal[R]) OnSuccess(listener func(failsafe.ExecutionEvent[R])) FailureSignal[R] {
	c.BaseFailurePolicy.OnSuccess(listener)
	return c
}

func (c *failureSignal[R]) OnFailure(listener func(failsafe.ExecutionEvent[R])) FailureSignal[R] {
	c.BaseFailurePolicy.OnFailure(listener)
	return c
}

func (c *failureSignal[R]) WithFailureRateThreshold(failureRateThreshold uint, failureExecutionThreshold uint, failureThresholdingPeriod time.Duration) FailureSignal[R] {
	c.failureRateThreshold = failureRateThreshold
	c.failureExecutionThreshold = failureExecutionThreshold
	c.failureThresholdingPeriod = failureThresholdingPeriod
	return c
}

var _ FailureSignal[any] = &failureSignal[any]{}

func NewFailureSignal[R any]() FailureSignal[R] {
	return &failureSignal[R]{}
}
