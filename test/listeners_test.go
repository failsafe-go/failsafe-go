package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// Asserts that listeners are called the expected number of times for a successful completion.
func TestListenersOnSuccess(t *testing.T) {
	// Given - Fail 4 times then succeed
	stub := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 2, false, false, true)
	rpBuilder := retrypolicy.Builder[bool]().HandleResult(false).WithMaxAttempts(10)
	cbBuilder := circuitbreaker.Builder[bool]().HandleResult(false).WithDelay(0)
	fbBuilder := fallback.BuilderWithResult(false)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	registerFbListeners(stats, fbBuilder)
	executor := failsafe.NewExecutor[bool](fbBuilder.Build(), rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.GetWithExecution(stub)

	// Then
	assert.Equal(t, 0, stats.abort)
	assert.Equal(t, 4, stats.retry)
	assert.Equal(t, 4, stats.retryScheduled)
	assert.Equal(t, 0, stats.retriesExceeded)
	assert.Equal(t, 1, stats.rpSuccess)
	assert.Equal(t, 4, stats.rpFailure)

	assert.Equal(t, 9, stats.stateChanged)
	assert.Equal(t, 4, stats.open)
	assert.Equal(t, 4, stats.halfOpen)
	assert.Equal(t, 1, stats.close)
	assert.Equal(t, 1, stats.cbSuccess)
	assert.Equal(t, 4, stats.cbFailure)

	assert.Equal(t, 0, stats.fbComplete)
	assert.Equal(t, 1, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 1, stats.complete)
	assert.Equal(t, 1, stats.success)
	assert.Equal(t, 0, stats.failure)
}

// Asserts that listeners are called the expected number of times for an unhandled failure.
func TestListenersForUnhandledFailure(t *testing.T) {
	// Given - Fail 2 times then don't match policy
	stub := testutil.ErrorNTimesThenError[bool](testutil.ErrInvalidState, 2, testutil.ErrInvalidArgument)
	rpBuilder := retrypolicy.Builder[bool]().HandleErrors(testutil.ErrInvalidState).WithMaxAttempts(10)
	cbBuilder := circuitbreaker.Builder[bool]().WithDelay(0)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	executor := failsafe.NewExecutor[bool](rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.GetWithExecution(stub)

	// Then
	assert.Equal(t, 0, stats.abort)
	assert.Equal(t, 2, stats.retry)
	assert.Equal(t, 2, stats.retryScheduled)
	assert.Equal(t, 0, stats.retriesExceeded)
	assert.Equal(t, 1, stats.rpSuccess)
	assert.Equal(t, 2, stats.rpFailure)

	assert.Equal(t, 5, stats.stateChanged)
	assert.Equal(t, 3, stats.open)
	assert.Equal(t, 2, stats.halfOpen)
	assert.Equal(t, 0, stats.close)
	assert.Equal(t, 0, stats.cbSuccess)
	assert.Equal(t, 3, stats.cbFailure)

	assert.Equal(t, 1, stats.complete)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

// Asserts that listeners are called the expected number of times when retries are exceeded.
func TestListenersForRetriesExceeded(t *testing.T) {
	// Given - Fail 4 times and exceed retries
	stub := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	rpBuilder := retrypolicy.Builder[bool]().WithMaxRetries(3)
	cbBuilder := circuitbreaker.Builder[bool]().WithDelay(0)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	executor := failsafe.NewExecutor[bool](rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.GetWithExecution(stub)

	// Then
	assert.Equal(t, 0, stats.abort)
	assert.Equal(t, 3, stats.retry)
	assert.Equal(t, 3, stats.retryScheduled)
	assert.Equal(t, 1, stats.retriesExceeded)
	assert.Equal(t, 4, stats.rpFailure)
	assert.Equal(t, 0, stats.rpSuccess)

	assert.Equal(t, 7, stats.stateChanged)
	assert.Equal(t, 4, stats.open)
	assert.Equal(t, 3, stats.halfOpen)
	assert.Equal(t, 0, stats.close)
	assert.Equal(t, 0, stats.cbSuccess)
	assert.Equal(t, 4, stats.cbFailure)

	assert.Equal(t, 1, stats.complete)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForAbort(t *testing.T) {
	// Given - Fail twice then abort
	stub := testutil.ErrorNTimesThenError[bool](testutil.ErrInvalidState, 3, testutil.ErrInvalidArgument)
	rpBuilder := retrypolicy.Builder[bool]().AbortOnErrors(testutil.ErrInvalidArgument).WithMaxRetries(3)
	cbBuilder := circuitbreaker.Builder[bool]().WithDelay(0)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	executor := failsafe.NewExecutor[bool](rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.GetWithExecution(stub)

	// Then
	assert.Equal(t, 1, stats.abort)
	assert.Equal(t, 3, stats.retry)
	assert.Equal(t, 3, stats.retryScheduled)
	assert.Equal(t, 0, stats.retriesExceeded)
	assert.Equal(t, 0, stats.rpSuccess)
	assert.Equal(t, 4, stats.rpFailure)

	assert.Equal(t, 7, stats.stateChanged)
	assert.Equal(t, 4, stats.open)
	assert.Equal(t, 3, stats.halfOpen)
	assert.Equal(t, 0, stats.close)
	assert.Equal(t, 0, stats.cbSuccess)
	assert.Equal(t, 4, stats.cbFailure)

	assert.Equal(t, 1, stats.complete)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForFailingRetryPolicy(t *testing.T) {
	// Given - Fail 10 times
	stub := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	// NewExecutor failing RetryPolicy
	rpBuilder := retrypolicy.Builder[bool]()
	// And successful CircuitBreaker and Fallback
	cbBuilder := circuitbreaker.Builder[bool]().HandleErrors(testutil.ErrInvalidArgument).WithDelay(0)
	fbBuilder := fallback.BuilderWithResult[bool](true).HandleErrors(testutil.ErrInvalidArgument)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	registerFbListeners(stats, fbBuilder)
	executor := failsafe.NewExecutor[bool](fbBuilder.Build(), rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.GetWithExecution(stub)

	// Then
	assert.Equal(t, 0, stats.rpSuccess)
	assert.Equal(t, 3, stats.rpFailure)

	assert.Equal(t, 3, stats.cbSuccess)
	assert.Equal(t, 0, stats.cbFailure)

	assert.Equal(t, 0, stats.fbComplete)
	assert.Equal(t, 1, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 1, stats.complete)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForFailingCircuitBreaker(t *testing.T) {
	// Given - Fail 10 times
	stub := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	// NewExecutor successful RetryPolicy
	rpBuilder := retrypolicy.Builder[bool]().HandleErrors(testutil.ErrInvalidArgument)
	// And failing CircuitBreaker
	cbBuilder := circuitbreaker.Builder[bool]().WithDelay(0)
	// And successful Fallback
	fbBuilder := fallback.BuilderWithResult[bool](true).HandleErrors(testutil.ErrInvalidArgument)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	registerFbListeners(stats, fbBuilder)
	executor := failsafe.NewExecutor[bool](fbBuilder.Build(), rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.GetWithExecution(stub)

	// Then
	assert.Equal(t, 1, stats.rpSuccess)
	assert.Equal(t, 0, stats.rpFailure)

	assert.Equal(t, 0, stats.cbSuccess)
	assert.Equal(t, 1, stats.cbFailure)

	assert.Equal(t, 0, stats.fbComplete)
	assert.Equal(t, 1, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 1, stats.complete)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForFailingFallback(t *testing.T) {
	// Given - Fail 10 times
	stub := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	// Given successful RetryPolicy and CircuitBreaker
	rpBuilder := retrypolicy.Builder[bool]().HandleErrors(testutil.ErrInvalidArgument)
	cbBuilder := circuitbreaker.Builder[bool]().HandleErrors(testutil.ErrInvalidArgument).WithDelay(0)
	// And failing Fallback
	fbBuilder := fallback.BuilderWithError[bool](testutil.ErrConnecting)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	registerFbListeners(stats, fbBuilder)
	executor := failsafe.NewExecutor[bool](fbBuilder.Build(), rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.GetWithExecution(stub)

	// Then
	assert.Equal(t, 1, stats.rpSuccess)
	assert.Equal(t, 0, stats.rpFailure)

	assert.Equal(t, 1, stats.cbSuccess)
	assert.Equal(t, 0, stats.cbFailure)

	assert.Equal(t, 1, stats.fbComplete)
	assert.Equal(t, 0, stats.fbSuccess)
	assert.Equal(t, 1, stats.fbFailure)

	assert.Equal(t, 1, stats.complete)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestGetElapsedTime(t *testing.T) {
	rp := retrypolicy.Builder[any]().
		HandleResult(false).
		OnRetryScheduled(func(e failsafe.ExecutionScheduledEvent[any]) {
			assert.True(t, e.ElapsedAttemptTime().Milliseconds() >= 90)
		}).
		Build()
	failsafe.Get(func() (any, error) {
		time.Sleep(100 * time.Millisecond)
		return false, nil
	}, rp)
}

func TestRetryPolicyOnScheduledRetry(t *testing.T) {
	executions := 0
	rp := retrypolicy.Builder[any]().HandleResult(nil).WithMaxRetries(1).
		OnFailure(func(e failsafe.ExecutionEvent[any]) {
			if executions == 1 {
				assert.True(t, e.IsFirstAttempt())
				assert.False(t, e.IsRetry())
			} else {
				assert.False(t, e.IsFirstAttempt())
				assert.True(t, e.IsRetry())
			}
		}).
		OnRetry(func(e failsafe.ExecutionEvent[any]) {
			assert.False(t, e.IsFirstAttempt())
			assert.True(t, e.IsRetry())
		}).
		OnRetryScheduled(func(e failsafe.ExecutionScheduledEvent[any]) {
			if executions == 1 {
				assert.True(t, e.IsFirstAttempt())
				assert.False(t, e.IsRetry())
			} else {
				assert.False(t, e.IsFirstAttempt())
				assert.True(t, e.IsRetry())
			}
		}).
		Build()

	failsafe.NewExecutor[any](rp).Get(func() (any, error) {
		executions++
		return nil, nil
	})
}

func TestListenersForRateLimiter(t *testing.T) {
	// Given - Fail 4 times then succeed
	rlBuilder := ratelimiter.SmoothBuilderWithMaxRate[bool](100 * time.Millisecond)
	stats := &listenerStats{}
	registerRlListeners(stats, rlBuilder)
	executor := failsafe.NewExecutor[bool](rlBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.Run(testutil.RunFn(nil)) // Success
	executor.Run(testutil.RunFn(nil)) // Failure
	time.Sleep(110 * time.Millisecond)
	executor.Run(testutil.RunFn(nil)) // Success
	executor.Run(testutil.RunFn(nil)) // Failure
	executor.Run(testutil.RunFn(nil)) // Failure

	// Then
	assert.Equal(t, 3, stats.rlExceeded)

	assert.Equal(t, 5, stats.complete)
	assert.Equal(t, 2, stats.success)
	assert.Equal(t, 3, stats.failure)
}

// Asserts which listeners are called when a panic occurs.
func TestListenersOnPanic(t *testing.T) {
	// Given - Fail 2 times then panic
	panicValue := "test panic"
	stub := testutil.ErrorNTimesThenPanic[bool](testutil.ErrInvalidState, 2, panicValue)
	rpBuilder := retrypolicy.Builder[bool]().WithMaxAttempts(10)
	cbBuilder := circuitbreaker.Builder[bool]().WithDelay(0)
	fbBuilder := fallback.BuilderWithResult(true)
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	registerFbListeners(stats, fbBuilder)
	executor := failsafe.NewExecutor[bool](fbBuilder.Build(), rpBuilder.Build(), cbBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	assert.PanicsWithValue(t, panicValue, func() {
		executor.GetWithExecution(stub)
	})

	// Then
	assert.Equal(t, 0, stats.abort)
	assert.Equal(t, 2, stats.retry)
	assert.Equal(t, 2, stats.retryScheduled)
	assert.Equal(t, 0, stats.retriesExceeded)
	assert.Equal(t, 0, stats.rpSuccess) // Success listener is not called on a panic
	assert.Equal(t, 2, stats.rpFailure) // Failure is currently skipped on a panic

	assert.Equal(t, 4, stats.stateChanged)
	assert.Equal(t, 2, stats.open)
	assert.Equal(t, 2, stats.halfOpen)
	assert.Equal(t, 0, stats.close)
	assert.Equal(t, 0, stats.cbSuccess)
	assert.Equal(t, 2, stats.cbFailure)

	assert.Equal(t, 0, stats.fbComplete) // Complete listener will not be called since the fallback is currently skipped on a panic
	assert.Equal(t, 0, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 0, stats.complete) // Complete listener is not called on a panic
	assert.Equal(t, 0, stats.success)  // Success listener is not called on a panic
	assert.Equal(t, 0, stats.failure)  // Failure listener is not called on a panic
}

type listenerStats struct {
	// RetryPolicy
	abort           int
	retry           int
	retryScheduled  int
	retriesExceeded int
	rpSuccess       int
	rpFailure       int

	// CircuitBreaker
	stateChanged int
	open         int
	close        int
	halfOpen     int
	cbSuccess    int
	cbFailure    int

	// Fallback
	fbComplete int
	fbSuccess  int
	fbFailure  int

	// RateLimiter
	rlExceeded int

	// Executor
	complete int
	success  int
	failure  int
}

func registerRpListeners[R any](stats *listenerStats, rpBuilder retrypolicy.RetryPolicyBuilder[R]) {
	rpBuilder.OnAbort(func(f failsafe.ExecutionEvent[R]) {
		stats.abort++
	}).OnRetriesExceeded(func(f failsafe.ExecutionEvent[R]) {
		stats.retriesExceeded++
	}).OnRetry(func(f failsafe.ExecutionEvent[R]) {
		fmt.Println("RetryPolicy retry")
		stats.retry++
	}).OnRetryScheduled(func(f failsafe.ExecutionScheduledEvent[R]) {
		stats.retryScheduled++
	}).OnSuccess(func(event failsafe.ExecutionEvent[R]) {
		stats.rpSuccess++
	}).OnFailure(func(event failsafe.ExecutionEvent[R]) {
		stats.rpFailure++
	})
}

func registerCbListeners[R any](stats *listenerStats, cbBuilder circuitbreaker.CircuitBreakerBuilder[R]) {
	cbBuilder.OnStateChanged(func(event circuitbreaker.StateChangedEvent) {
		fmt.Println("CircuitBreaker state change from", event.PreviousState, "to", event.CurrentState)
		stats.stateChanged++
	})
	cbBuilder.OnOpen(func(event circuitbreaker.StateChangedEvent) {
		fmt.Println("CircuitBreaker open")
		stats.open++
	}).OnClose(func(event circuitbreaker.StateChangedEvent) {
		fmt.Println("CircuitBreaker closed")
		stats.close++
	}).OnHalfOpen(func(event circuitbreaker.StateChangedEvent) {
		fmt.Println("CircuitBreaker half-open")
		stats.halfOpen++
	}).OnSuccess(func(event failsafe.ExecutionEvent[R]) {
		stats.cbSuccess++
	}).OnFailure(func(event failsafe.ExecutionEvent[R]) {
		stats.cbFailure++
	})
}

func registerFbListeners[R any](stats *listenerStats, fbBuilder fallback.FallbackBuilder[R]) {
	fbBuilder.OnFallbackExecuted(func(f failsafe.ExecutionCompletedEvent[R]) {
		stats.fbComplete++
	}).OnFailure(func(e failsafe.ExecutionEvent[R]) {
		stats.fbFailure++
	}).OnSuccess(func(e failsafe.ExecutionEvent[R]) {
		stats.fbSuccess++
	})
}

func registerRlListeners[R any](stats *listenerStats, rlBuilder ratelimiter.RateLimiterBuilder[R]) {
	rlBuilder.OnRateLimitExceeded(func(event failsafe.ExecutionCompletedEvent[R]) {
		stats.rlExceeded++
	})
}

func registerExecutorListeners[R any](stats *listenerStats, executor failsafe.Executor[R]) {
	executor.OnComplete(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.complete++
	}).OnFailure(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.failure++
	}).OnSuccess(func(e failsafe.ExecutionCompletedEvent[R]) {
		stats.success++
	})
}
