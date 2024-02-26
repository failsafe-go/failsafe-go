package retrypolicy

import (
	"fmt"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/policy"
)

const defaultMaxRetries = 2

// ErrExceeded is an empty ExceededError instance, useful for building policies that want to handle this error.
var ErrExceeded = &ExceededError{}

// ExceededError is returned when a RetryPolicy's max attempts or max duration are exceeded.
type ExceededError struct {
	lastResult any
	lastError  error
}

// LastResult returns the last result that caused the ExceededError.
func (e *ExceededError) LastResult() any {
	return e.lastResult
}

// LastError returns the last error that caused the ExceededError.
func (e *ExceededError) LastError() error {
	return e.lastError
}

func (e *ExceededError) Error() string {
	return fmt.Sprintf("retries exceeded. last result: %v, last error: %v", e.lastResult, e.lastError)
}

func (e *ExceededError) Unwrap() error {
	if e.lastError != nil {
		return e.lastError
	}
	return fmt.Errorf("failure: %v", e.lastResult)
}

// Is returns whether err is of the type ExceededError.
func (e *ExceededError) Is(err error) bool {
	_, ok := err.(*ExceededError)
	return ok
}

// RetryPolicy is a policy that defines when retries should be performed. See RetryPolicyBuilder for configuration
// options.
//
// This type is concurrency safe.
type RetryPolicy[R any] interface {
	failsafe.Policy[R]
}

/*
RetryPolicyBuilder builds RetryPolicy instances.

  - By default, a RetryPolicy will retry a failed execution up to 2 times when any error is returned, with no delay between
    retry attempts. If retries are exceeded, ExceededError is returned by default. Alternatively, ReturnLastFailure
    can be used to configure the policy to return the last execution failure.
  - You can change the default number of retry attempts and delay between retries by using the with configuration methods.
  - By default, any error is considered a failure and will be handled by the policy. You can override this by specifying
    your own HandleErrors conditions. The default error handling condition will only be overridden by another condition
    that handles error such as HandleErrors or HandleIf. Specifying a condition that only handles results, such as
    HandleResult or HandleResultIf will not replace the default error handling condition.
  - If multiple HandleErrors conditions are specified, any condition that matches an execution result or error will
    trigger policy handling.
  - The AbortOn, AbortWhen and AbortIf methods describe when retries should be aborted.

This class extends failsafe.ListenablePolicyBuilder, failsafe.FailurePolicyBuilder and failsafe.DelayablePolicyBuilder
which offer additional configuration.

This type is not concurrency safe.
*/
type RetryPolicyBuilder[R any] interface {
	failsafe.FailurePolicyBuilder[RetryPolicyBuilder[R], R]
	failsafe.DelayablePolicyBuilder[RetryPolicyBuilder[R], R]

	// AbortOnResult specifies that retries should be aborted if the execution result matches the result using
	// reflect.DeepEqual.
	AbortOnResult(result R) RetryPolicyBuilder[R]

	// AbortOnErrors specifies that retries should be aborted if the execution error matches any of the errs using errors.Is.
	AbortOnErrors(errs ...error) RetryPolicyBuilder[R]

	// AbortIf specifies that retries should be aborted if the predicate matches the result or error.
	AbortIf(predicate func(R, error) bool) RetryPolicyBuilder[R]

	// ReturnLastFailure configures the policy to return the last failure result or error after attempts are exceeded,
	// rather than returning ExceededError.
	ReturnLastFailure() RetryPolicyBuilder[R]

	// WithMaxAttempts sets the max number of execution attempts to perform. -1 indicates no limit. This method has the same
	// effect as setting 1 more than WithMaxRetries. For example, 2 retries equal 3 attempts.
	WithMaxAttempts(maxAttempts int) RetryPolicyBuilder[R]

	// WithMaxRetries sets the max number of retries to perform when an execution attempt fails. -1 indicates no limit. This
	// method has the same effect as setting 1 less than WithMaxAttempts. For example, 2 retries equal 3 attempts.
	WithMaxRetries(maxRetries int) RetryPolicyBuilder[R]

	// WithMaxDuration sets the max duration to perform retries for, else the execution will be failed.
	WithMaxDuration(maxDuration time.Duration) RetryPolicyBuilder[R]

	// WithBackoff wets the delay between retries, exponentially backing off to the maxDelay and multiplying consecutive
	// delays by a factor of 2. Replaces any previously configured fixed or random delays.
	WithBackoff(delay time.Duration, maxDelay time.Duration) RetryPolicyBuilder[R]

	// WithBackoffFactor sets the delay between retries, exponentially backing off to the maxDelay and multiplying
	// consecutive delays by the delayFactor. Replaces any previously configured fixed or random delays.
	WithBackoffFactor(delay time.Duration, maxDelay time.Duration, delayFactor float32) RetryPolicyBuilder[R]

	// WithRandomDelay sets a random delay between the delayMin and delayMax (inclusive) to occur between retries.
	// Replaces any previously configured delay or backoff delay.
	WithRandomDelay(delayMin time.Duration, delayMax time.Duration) RetryPolicyBuilder[R]

	// WithJitter sets the jitter to randomly vary retry delays by. For each retry delay, a random portion of the jitter will
	// be added or subtracted to the delay. For example: a jitter of 100 milliseconds will randomly add between -100 and 100
	// milliseconds to each retry delay. Replaces any previously configured jitter factor.
	//
	// Jitter should be combined with fixed, random, or exponential backoff delays. If no delays are configured, this setting
	// is ignored.
	WithJitter(jitter time.Duration) RetryPolicyBuilder[R]

	// WithJitterFactor sets the jitterFactor to randomly vary retry delays by. For each retry delay, a random portion of the
	// delay multiplied by the jitterFactor will be added or subtracted to the delay. For example: a retry delay of 100
	// milliseconds and a jitterFactor of .25 will result in a random retry delay between 75 and 125 milliseconds. Replaces
	// any previously configured jitter duration.
	//
	// Jitter should be combined with fixed, random, or exponential backoff delays. If no delays are configured, this setting
	// is ignored.
	WithJitterFactor(jitterFactor float32) RetryPolicyBuilder[R]

	// OnAbort registers the listener to be called when an execution is aborted.
	OnAbort(listener func(failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R]

	// OnRetryScheduled registers the listener to be called when a retry is about to be scheduled. This method differs from
	// OnRetry since it is called when a retry is initially scheduled but before any configured delay, whereas OnRetry is
	// called after a delay, just before the retry attempt takes place.
	OnRetryScheduled(listener func(failsafe.ExecutionScheduledEvent[R])) RetryPolicyBuilder[R]

	// OnRetry registers the listener to be called when a retry is about to be attempted.
	OnRetry(listener func(failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R]

	// OnRetriesExceeded registers the listener to be called when an execution fails and the max retry attempts or max
	// duration are exceeded. The provided event will contain the last execution result and error.
	OnRetriesExceeded(listener func(failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R]

	// Build returns a new RetryPolicy using the builder's configuration.
	Build() RetryPolicy[R]
}

type retryPolicyConfig[R any] struct {
	*policy.BaseFailurePolicy[R]
	*policy.BaseDelayablePolicy[R]
	*policy.BaseAbortablePolicy[R]

	returnLastFailure bool
	delayMin          time.Duration
	delayMax          time.Duration
	delayFactor       float32
	maxDelay          time.Duration
	jitter            time.Duration
	jitterFactor      float32
	maxDuration       time.Duration
	maxRetries        int

	onAbort           func(failsafe.ExecutionEvent[R])
	onRetry           func(failsafe.ExecutionEvent[R])
	onRetryScheduled  func(failsafe.ExecutionScheduledEvent[R])
	onRetriesExceeded func(failsafe.ExecutionEvent[R])
}

var _ RetryPolicyBuilder[any] = &retryPolicyConfig[any]{}

type retryPolicy[R any] struct {
	config *retryPolicyConfig[R]
}

// WithDefaults creates a RetryPolicy for execution result type R that allows 3 execution attempts max with no delay. To
// configure additional options on a RetryPolicy, use Builder instead.
func WithDefaults[R any]() RetryPolicy[R] {
	return Builder[R]().Build()
}

// Builder creates a RetryPolicyBuilder for execution result type R, which by default will build a RetryPolicy that
// allows 3 execution attempts max with no delay, unless configured otherwise.
func Builder[R any]() RetryPolicyBuilder[R] {
	return &retryPolicyConfig[R]{
		BaseFailurePolicy:   &policy.BaseFailurePolicy[R]{},
		BaseDelayablePolicy: &policy.BaseDelayablePolicy[R]{},
		BaseAbortablePolicy: &policy.BaseAbortablePolicy[R]{},
		maxRetries:          defaultMaxRetries,
	}
}

func (c *retryPolicyConfig[R]) Build() RetryPolicy[R] {
	rpCopy := *c
	return &retryPolicy[R]{
		config: &rpCopy, // TODO copy base fields
	}
}

func (c *retryPolicyConfig[R]) AbortOnResult(result R) RetryPolicyBuilder[R] {
	c.BaseAbortablePolicy.AbortOnResult(result)
	return c
}

func (c *retryPolicyConfig[R]) AbortOnErrors(errs ...error) RetryPolicyBuilder[R] {
	c.BaseAbortablePolicy.AbortOnErrors(errs...)
	return c
}

func (c *retryPolicyConfig[R]) AbortIf(predicate func(R, error) bool) RetryPolicyBuilder[R] {
	c.BaseAbortablePolicy.AbortIf(predicate)
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

func (c *retryPolicyConfig[R]) ReturnLastFailure() RetryPolicyBuilder[R] {
	c.returnLastFailure = true
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

func (c *retryPolicyConfig[R]) WithDelayFunc(delayFunc failsafe.DelayFunc[R]) RetryPolicyBuilder[R] {
	c.BaseDelayablePolicy.WithDelayFunc(delayFunc)
	return c
}

func (c *retryPolicyConfig[R]) WithBackoff(delay time.Duration, maxDelay time.Duration) RetryPolicyBuilder[R] {
	return c.WithBackoffFactor(delay, maxDelay, 2)
}

func (c *retryPolicyConfig[R]) WithBackoffFactor(delay time.Duration, maxDelay time.Duration, delayFactor float32) RetryPolicyBuilder[R] {
	c.BaseDelayablePolicy.WithDelay(delay)
	c.maxDelay = maxDelay
	c.delayFactor = delayFactor

	// Clear random delay
	c.delayMin = 0
	c.delayMax = 0
	return c
}

func (c *retryPolicyConfig[R]) WithRandomDelay(delayMin time.Duration, delayMax time.Duration) RetryPolicyBuilder[R] {
	c.delayMin = delayMin
	c.delayMax = delayMax

	// Clear non-random delay
	c.Delay = 0
	c.maxDelay = 0
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

func (c *retryPolicyConfig[R]) OnSuccess(listener func(event failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.OnSuccess(listener)
	return c
}

func (c *retryPolicyConfig[R]) OnFailure(listener func(event failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R] {
	c.BaseFailurePolicy.OnFailure(listener)
	return c
}

func (c *retryPolicyConfig[R]) OnAbort(listener func(failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R] {
	c.onAbort = listener
	return c
}

func (c *retryPolicyConfig[R]) OnRetry(listener func(failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R] {
	c.onRetry = listener
	return c
}

func (c *retryPolicyConfig[R]) OnRetryScheduled(listener func(failsafe.ExecutionScheduledEvent[R])) RetryPolicyBuilder[R] {
	c.onRetryScheduled = listener
	return c
}

func (c *retryPolicyConfig[R]) OnRetriesExceeded(listener func(failsafe.ExecutionEvent[R])) RetryPolicyBuilder[R] {
	c.onRetriesExceeded = listener
	return c
}

func (c *retryPolicyConfig[R]) allowsRetries() bool {
	return c.maxRetries == -1 || c.maxRetries > 0
}

func (rp *retryPolicy[R]) ToExecutor(policyIndex int, _ R) any {
	rpe := &retryPolicyExecutor[R]{
		BaseExecutor: &policy.BaseExecutor[R]{
			BaseFailurePolicy: rp.config.BaseFailurePolicy,
			PolicyIndex:       policyIndex,
		},
		retryPolicy: rp,
	}
	rpe.Executor = rpe
	return rpe
}
