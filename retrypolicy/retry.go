package retrypolicy

import (
	"errors"
	"reflect"
	"time"

	"failsafe"
	"failsafe/internal/util"
)

const defaultMaxRetries = 2

type RetryPolicy[R any] interface {
	failsafe.Policy[R]
}

/*
RetryPolicyBuilder builds RetryPolicy instances.

This type is not threadsafe.
*/
type RetryPolicyBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[RetryPolicyBuilder[R], R]
	failsafe.FailurePolicyBuilder[RetryPolicyBuilder[R], R]
	failsafe.DelayablePolicyBuilder[RetryPolicyBuilder[R], R]
	WithMaxAttempts(maxAttempts int) RetryPolicyBuilder[R]
	WithMaxRetries(maxRetries int) RetryPolicyBuilder[R]
	WithMaxDuration(maxDuration time.Duration) RetryPolicyBuilder[R]
	WithBackoff(delay time.Duration, maxDelay time.Duration) RetryPolicyBuilder[R]
	WithBackoffFactor(delay time.Duration, maxDelay time.Duration, delayFactor float32) RetryPolicyBuilder[R]
	WithJitter(jitter time.Duration) RetryPolicyBuilder[R]
	WithJitterFactor(jitterFactor float32) RetryPolicyBuilder[R]
	OnAbort(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R]
	OnFailedAttempt(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R]
	OnRetriesExceeded(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R]
	OnRetryScheduled(listener func(failsafe.ExecutionScheduledEvent[R])) RetryPolicyBuilder[R]
	OnRetry(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R]
	Build() RetryPolicy[R]
}

type retryPolicyConfig[R any] struct {
	*failsafe.BaseListenablePolicy[R]
	*failsafe.BaseFailurePolicy[R]
	*failsafe.BaseDelayablePolicy[R]

	delayMin     time.Duration
	delayMax     time.Duration
	delayFactor  float32
	maxDelay     time.Duration
	jitter       time.Duration
	jitterFactor float32
	maxDuration  time.Duration
	maxRetries   int
	// Conditions that determine whether retries should be aborted
	abortConditions []func(result R, err error) bool

	abortListener           func(failsafe.ExecutionCompletedEvent[R])
	failedAttemptListener   func(failsafe.ExecutionAttemptedEvent[R])
	retriesExceededListener func(failsafe.ExecutionCompletedEvent[R])
	retryListener           func(failsafe.ExecutionAttemptedEvent[R])
	retryScheduledListener  func(failsafe.ExecutionScheduledEvent[R])
}

var _ RetryPolicyBuilder[any] = &retryPolicyConfig[any]{}

type retryPolicy[R any] struct {
	config *retryPolicyConfig[R]
}

func OfDefaults[R any]() RetryPolicy[R] {
	return BuilderForResult[R]().Build()
}

func Builder() RetryPolicyBuilder[any] {
	return BuilderForResult[any]()
}

func BuilderForResult[R any]() RetryPolicyBuilder[R] {
	return &retryPolicyConfig[R]{
		BaseListenablePolicy: &failsafe.BaseListenablePolicy[R]{},
		BaseFailurePolicy:    &failsafe.BaseFailurePolicy[R]{},
		BaseDelayablePolicy:  &failsafe.BaseDelayablePolicy[R]{},
		maxRetries:           defaultMaxRetries,
	}
}

func (c *retryPolicyConfig[R]) Build() RetryPolicy[R] {
	rpCopy := *c
	return &retryPolicy[R]{
		config: &rpCopy, // TODO copy base fields
	}
}

func (c *retryPolicyConfig[R]) AbortOn(errs ...error) RetryPolicyBuilder[R] {
	for _, err := range errs {
		c.abortConditions = append(c.abortConditions, func(result R, actualErr error) bool {
			return errors.Is(actualErr, err)
		})
	}
	return c
}

func (c *retryPolicyConfig[R]) AbortIf(predicate func(error) bool) RetryPolicyBuilder[R] {
	c.abortConditions = append(c.abortConditions, func(result R, err error) bool {
		if err == nil {
			return false
		}
		return predicate(err)
	})
	return c
}

func (c *retryPolicyConfig[R]) AbortWhen(result R) RetryPolicyBuilder[R] {
	c.abortConditions = append(c.abortConditions, func(r R, err error) bool {
		return reflect.DeepEqual(r, result)
	})
	return c
}

func (c *retryPolicyConfig[R]) Handle(errs ...error) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.Handle(errs)
	return c
}

func (c *retryPolicyConfig[R]) HandleIf(predicate func(error) bool) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *retryPolicyConfig[R]) HandleResult(result R) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *retryPolicyConfig[R]) HandleResultIf(resultPredicate func(R) bool) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.HandleResultIf(resultPredicate)
	return c
}

func (c *retryPolicyConfig[R]) HandleAllIf(predicate func(R, error) bool) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.HandleAllIf(predicate)
	return c
}

func (c *retryPolicyConfig[R]) WithMaxAttempts(maxAttempts int) RetryPolicyBuilder[R] {
	c.maxRetries = maxAttempts - 1
	return c
}

// WithMaxRetries configures the max number of retries to perform. A non-positive maxRetries will disable retries.
func (c *retryPolicyConfig[R]) WithMaxRetries(maxRetries int) RetryPolicyBuilder[R] {
	c.maxRetries = maxRetries
	return c
}

func (c *retryPolicyConfig[R]) WithMaxDuration(maxDuration time.Duration) RetryPolicyBuilder[R] {
	c.maxDuration = maxDuration
	return c
}

// WithDelay configures the time to delay between execution attempts.
func (c *retryPolicyConfig[R]) WithDelay(delay time.Duration) RetryPolicyBuilder[R] {
	c.BaseDelayablePolicy.WithDelay(delay)
	return c
}

func (c *retryPolicyConfig[R]) WithDelayFn(delayFn failsafe.DelayFunction[R]) RetryPolicyBuilder[R] {
	c.BaseDelayablePolicy.WithDelayFn(delayFn)
	return c
}

func (c *retryPolicyConfig[R]) WithBackoff(delay time.Duration, maxDelay time.Duration) RetryPolicyBuilder[R] {
	c.BaseDelayablePolicy.WithDelay(delay)
	c.delayMax = maxDelay
	c.delayFactor = 2
	return c
}

func (c *retryPolicyConfig[R]) WithBackoffFactor(delay time.Duration, maxDelay time.Duration, delayFactor float32) RetryPolicyBuilder[R] {
	c.BaseDelayablePolicy.WithDelay(delay)
	c.delayMax = maxDelay
	c.delayFactor = delayFactor
	return c
}

func (c *retryPolicyConfig[R]) WithJitter(jitter time.Duration) RetryPolicyBuilder[R] {
	c.jitter = jitter
	return c
}

func (c *retryPolicyConfig[R]) WithJitterFactor(jitterFactor float32) RetryPolicyBuilder[R] {
	c.jitterFactor = jitterFactor
	return c
}

func (c *retryPolicyConfig[R]) OnSuccess(listener func(event failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R] {
	c.BaseListenablePolicy.OnSuccess(listener)
	return c
}

func (c *retryPolicyConfig[R]) OnFailure(listener func(event failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R] {
	c.BaseListenablePolicy.OnFailure(listener)
	return c
}

// OnAbort is called when reties are aborted.
func (c *retryPolicyConfig[R]) OnAbort(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R] {
	c.abortListener = listener
	return c
}

// OnFailedAttempt registers the listener to be called when an execution attempt fails.
func (c *retryPolicyConfig[R]) OnFailedAttempt(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R] {
	c.failedAttemptListener = listener
	return c
}

// OnRetriesExceeded registers the listener to be called when an execution attempt fails and all retries have been exceeded.
func (c *retryPolicyConfig[R]) OnRetriesExceeded(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R] {
	c.retriesExceededListener = listener
	return c
}

// OnRetry registers the listener to be called when a retry is about to be attempted.
func (c *retryPolicyConfig[R]) OnRetry(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R] {
	c.retryListener = listener
	return c
}

// OnRetryScheduled registers the listener to be called when a retry is scheduled for execution.
func (c *retryPolicyConfig[R]) OnRetryScheduled(listener func(failsafe.ExecutionScheduledEvent[R])) RetryPolicyBuilder[R] {
	c.retryScheduledListener = listener
	return c
}

func (c *retryPolicyConfig[R]) allowsRetries() bool {
	return c.maxRetries == -1 || c.maxRetries > 0
}

func (c *retryPolicyConfig[R]) isAbortable(result R, err error) bool {
	return util.AppliesToAny(c.abortConditions, result, err)
}

func (rp *retryPolicy[R]) ToExecutor() failsafe.PolicyExecutor[R] {
	rpe := retryPolicyExecutor[R]{
		BasePolicyExecutor: &failsafe.BasePolicyExecutor[R]{
			BaseListenablePolicy: rp.config.BaseListenablePolicy,
			BaseFailurePolicy:    rp.config.BaseFailurePolicy,
		},
		retryPolicy: rp,
	}
	rpe.PolicyExecutor = &rpe
	return &rpe
}
