package retrypolicy

import (
	"errors"
	"reflect"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/spi"
)

const defaultMaxRetries = 2

// RetryPolicy is a policy that defines when retries should be performed. See RetryPolicyBuilder for configuration options.
//
// This type is concurrency safe.
type RetryPolicy[R any] interface {
	failsafe.Policy[R]
}

/*
RetryPolicyBuilder builds RetryPolicy instances.

  - By default, a RetryPolicy will retry up to 2 times when any error is returned, with no delay between retry attempts.
  - You can change the default number of retry attempts and delay between retries by using the with configuration methods.
  - By default, any error is considered a failure and will be handled by the policy. You can override this by specifying your own HandleErrors
    conditions. The default error handling condition will only be overridden by another condition that handles error such as HandleErrors or
    HandleIf. Specifying a condition that only handles results, such as HandleResult or HandleResultIf will not replace the default error
    handling condition.
  - If multiple HandleErrors conditions are specified, any condition that matches an execution result or error will trigger policy handling.
  - The AbortOn, AbortWhen and AbortIf methods describe when retries should be aborted.

This class extends failsafe.ListenablePolicyBuilder, failsafe.FailurePolicyBuilder and failsafe.DelayablePolicyBuilder which offer
additional configuration.

This type is not concurrency safe.
*/
type RetryPolicyBuilder[R any] interface {
	failsafe.ListenablePolicyBuilder[RetryPolicyBuilder[R], R]
	failsafe.FailurePolicyBuilder[RetryPolicyBuilder[R], R]
	failsafe.DelayablePolicyBuilder[RetryPolicyBuilder[R], R]

	// AbortOnErrors specifies that retries should be aborted if the execution error matches any of the errs using errors.Is.
	AbortOnErrors(errs ...error) RetryPolicyBuilder[R]

	// AbortOnResult wpecifies that retries should be aborted if the execution result matches the result using reflect.DeepEqual.
	AbortOnResult(result R) RetryPolicyBuilder[R]

	// AbortIf specifies that retries should be aborted if the predicate matches the result or error.
	AbortIf(predicate func(R, error) bool) RetryPolicyBuilder[R]

	// WithMaxAttempts sets the max number of execution attempts to perform. -1 indicates no limit. This method has the same effect as
	// setting 1 more than WithMaxRetries. For example, 2 retries equal 3 attempts.
	WithMaxAttempts(maxAttempts int) RetryPolicyBuilder[R]

	// WithMaxRetries sets the max number of retries to perform when an execution attempt fails. -1 indicates no limit. This method has the
	// same effect as setting 1 less than WithMaxAttempts. For example, 2 retries equal 3 attempts.
	WithMaxRetries(maxRetries int) RetryPolicyBuilder[R]

	// WithMaxDuration sets the max duration to perform retries for, else the execution will be failed.
	WithMaxDuration(maxDuration time.Duration) RetryPolicyBuilder[R]

	// WithBackoff wets the delay between retries, exponentially backing off to the maxDelay and multiplying consecutive delays by a factor
	// of 2. Replaces any previously configured fixed or random delays.
	WithBackoff(delay time.Duration, maxDelay time.Duration) RetryPolicyBuilder[R]

	// WithBackoffFactor sets the delay between retries, exponentially backing off to the maxDelay and multiplying consecutive delays by the
	// delayFactor. Replaces any previously configured fixed or random delays.
	WithBackoffFactor(delay time.Duration, maxDelay time.Duration, delayFactor float32) RetryPolicyBuilder[R]

	// WithJitter sets the jitter to randomly vary retry delays by. For each retry delay, a random portion of the jitter will be added or
	// subtracted to the delay. For example: a jitter of 100 milliseconds will randomly add between -100 and 100 milliseconds to each retry
	// delay. Replaces any previously configured jitter factor.
	//
	// Jitter should be combined with fixed, random, or exponential backoff delays. If no delays are configured, this setting is ignored.
	WithJitter(jitter time.Duration) RetryPolicyBuilder[R]

	// WithJitterFactor sets the jitterFactor to randomly vary retry delays by. For each retry delay, a random portion of the delay
	// multiplied by the jitterFactor will be added or subtracted to the delay. For example: a retry delay of 100 milliseconds and a
	// jitterFactor of .25 will result in a random retry delay between 75 and 125 milliseconds. Replaces any previously configured jitter
	// duration.
	//
	// Jitter should be combined with fixed, random, or exponential backoff delays. If no delays are configured, this setting is ignored.
	WithJitterFactor(jitterFactor float32) RetryPolicyBuilder[R]

	// OnAbort registers the listener to be called when an execution is aborted.
	OnAbort(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R]

	// OnFailedAttempt registers the listener to be called when an execution attempt fails. You can also use onFailure to determine when
	// the execution attempt fails and all retries have failed.
	OnFailedAttempt(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R]

	// OnRetriesExceeded registers the listener to be called when an execution fails and the max retry attempts or max duration are exceeded.
	OnRetriesExceeded(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R]

	// OnRetryScheduled registers the listener to be called when a retry is about to be scheduled. This method differs from OnRetry since it
	// is called when a retry is initially scheduled but before any configured delay, whereas OnRetry is called after a delay, just before
	// the retry attempt takes place.
	OnRetryScheduled(listener func(failsafe.ExecutionScheduledEvent[R])) RetryPolicyBuilder[R]

	// OnRetry registers the listener to be called when a retry is about to be attempted.
	OnRetry(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R]

	// Build returns a new RetryPolicy using the builder's configuration.
	Build() RetryPolicy[R]
}

type retryPolicyConfig[R any] struct {
	*spi.BaseListenablePolicy[R]
	*spi.BaseFailurePolicy[R]
	*spi.BaseDelayablePolicy[R]

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

// WithDefaults creates a RetryPolicy for execution result type R that allows 3 execution attempts max with no delay. To configure additional
// options on a RetryPolicy, use Builder instead.
func WithDefaults[R any]() RetryPolicy[R] {
	return Builder[R]().Build()
}

// Builder creates a RetryPolicyBuilder for execution result type R, which by default will build a RetryPolicy that allows 3 execution
// attempts max with no delay, unless configured otherwise.
func Builder[R any]() RetryPolicyBuilder[R] {
	return &retryPolicyConfig[R]{
		BaseListenablePolicy: &spi.BaseListenablePolicy[R]{},
		BaseFailurePolicy:    &spi.BaseFailurePolicy[R]{},
		BaseDelayablePolicy:  &spi.BaseDelayablePolicy[R]{},
		maxRetries:           defaultMaxRetries,
	}
}

func (c *retryPolicyConfig[R]) Build() RetryPolicy[R] {
	rpCopy := *c
	return &retryPolicy[R]{
		config: &rpCopy, // TODO copy base fields
	}
}

func (c *retryPolicyConfig[R]) AbortOnErrors(errs ...error) RetryPolicyBuilder[R] {
	for _, target := range errs {
		c.abortConditions = append(c.abortConditions, func(result R, actualErr error) bool {
			return errors.Is(actualErr, target)
		})
	}
	return c
}

func (c *retryPolicyConfig[R]) AbortOnResult(result R) RetryPolicyBuilder[R] {
	c.abortConditions = append(c.abortConditions, func(r R, err error) bool {
		return reflect.DeepEqual(r, result)
	})
	return c
}

func (c *retryPolicyConfig[R]) AbortIf(predicate func(R, error) bool) RetryPolicyBuilder[R] {
	c.abortConditions = append(c.abortConditions, func(result R, err error) bool {
		return predicate(result, err)
	})
	return c
}

func (c *retryPolicyConfig[R]) HandleErrors(errs ...error) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.HandleErrors(errs...)
	return c
}

func (c *retryPolicyConfig[R]) HandleResult(result R) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.HandleResult(result)
	return c
}

func (c *retryPolicyConfig[R]) HandleIf(predicate func(R, error) bool) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.HandleIf(predicate)
	return c
}

func (c *retryPolicyConfig[R]) WithMaxAttempts(maxAttempts int) RetryPolicyBuilder[R] {
	c.maxRetries = maxAttempts - 1
	return c
}

func (c *retryPolicyConfig[R]) WithMaxRetries(maxRetries int) RetryPolicyBuilder[R] {
	c.maxRetries = maxRetries
	return c
}

func (c *retryPolicyConfig[R]) WithMaxDuration(maxDuration time.Duration) RetryPolicyBuilder[R] {
	c.maxDuration = maxDuration
	return c
}

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

func (c *retryPolicyConfig[R]) OnAbort(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R] {
	c.abortListener = listener
	return c
}

func (c *retryPolicyConfig[R]) OnFailedAttempt(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R] {
	c.failedAttemptListener = listener
	return c
}

func (c *retryPolicyConfig[R]) OnRetriesExceeded(listener func(failsafe.ExecutionCompletedEvent[R])) RetryPolicyBuilder[R] {
	c.retriesExceededListener = listener
	return c
}

func (c *retryPolicyConfig[R]) OnRetry(listener func(failsafe.ExecutionAttemptedEvent[R])) RetryPolicyBuilder[R] {
	c.retryListener = listener
	return c
}

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

func (rp *retryPolicy[R]) ToExecutor(policyIndex int) failsafe.PolicyExecutor[R] {
	rpe := &retryPolicyExecutor[R]{
		BasePolicyExecutor: &spi.BasePolicyExecutor[R]{
			BaseListenablePolicy: rp.config.BaseListenablePolicy,
			BaseFailurePolicy:    rp.config.BaseFailurePolicy,
			PolicyIndex:          policyIndex,
		},
		retryPolicy: rp,
	}
	rpe.PolicyExecutor = rpe
	return rpe
}
