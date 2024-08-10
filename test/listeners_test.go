package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/cachepolicy"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// Asserts that listeners are called the expected number of times for a successful completion.
func TestListenersOnSuccess(t *testing.T) {
	// Given - Fail 4 times then succeed
	stub, _ := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 2, false, false, true)
	rpBuilder := retrypolicy.NewBuilder[bool]().HandleResult(false).WithMaxAttempts(10)
	cbBuilder := circuitbreaker.NewBuilder[bool]().HandleResult(false).WithDelay(0)
	fbBuilder := fallback.NewBuilderWithResult(false)
	_, fsCache := policytesting.NewCache[bool]()
	cpBuilder := cachepolicy.NewBuilder(fsCache).WithKey("foo")
	stats := &listenerStats{}
	registerRpListeners(stats, rpBuilder)
	registerCbListeners(stats, cbBuilder)
	registerFbListeners(stats, fbBuilder)
	registerCpListeners(stats, cpBuilder)
	executor := failsafe.NewExecutor[bool](fbBuilder.Build(), cpBuilder.Build(), rpBuilder.Build(), cbBuilder.Build())
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

	assert.Equal(t, 0, stats.fbDone)
	assert.Equal(t, 1, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 1, stats.cpCached)
	assert.Equal(t, 0, stats.cpHit)
	assert.Equal(t, 1, stats.cpMiss)

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 1, stats.success)
	assert.Equal(t, 0, stats.failure)
}

// Asserts that listeners are called the expected number of times for an unhandled failure.
func TestListenersForUnhandledFailure(t *testing.T) {
	// Given - Fail 2 times then don't match policy
	stub := testutil.ErrorNTimesThenError[bool](testutil.ErrInvalidState, 2, testutil.ErrInvalidArgument)
	rpBuilder := retrypolicy.NewBuilder[bool]().HandleErrors(testutil.ErrInvalidState).WithMaxAttempts(10)
	cbBuilder := circuitbreaker.NewBuilder[bool]().WithDelay(0)
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

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

// Asserts that listeners are called the expected number of times when retries are exceeded.
func TestListenersForRetriesExceeded(t *testing.T) {
	// Given - Fail 4 times and exceed retries
	stub, _ := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	rpBuilder := retrypolicy.NewBuilder[bool]().WithMaxRetries(3)
	cbBuilder := circuitbreaker.NewBuilder[bool]().WithDelay(0)
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

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForAbort(t *testing.T) {
	// Given - Fail twice then abort
	stub := testutil.ErrorNTimesThenError[bool](testutil.ErrInvalidState, 3, testutil.ErrInvalidArgument)
	rpBuilder := retrypolicy.NewBuilder[bool]().AbortOnErrors(testutil.ErrInvalidArgument).WithMaxRetries(3)
	cbBuilder := circuitbreaker.NewBuilder[bool]().WithDelay(0)
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

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForFailingRetryPolicy(t *testing.T) {
	// Given - Fail 10 times
	stub, _ := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	// NewExecutor failing RetryPolicy
	rpBuilder := retrypolicy.NewBuilder[bool]()
	// And successful CircuitBreaker and Fallback
	cbBuilder := circuitbreaker.NewBuilder[bool]().HandleErrors(testutil.ErrInvalidArgument).WithDelay(0)
	fbBuilder := fallback.NewBuilderWithResult[bool](true).HandleErrors(testutil.ErrInvalidArgument)
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

	assert.Equal(t, 0, stats.fbDone)
	assert.Equal(t, 1, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForFailingCircuitBreaker(t *testing.T) {
	// Given - Fail 10 times
	stub, _ := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	// NewExecutor successful RetryPolicy
	rpBuilder := retrypolicy.NewBuilder[bool]().HandleErrors(testutil.ErrInvalidArgument)
	// And failing CircuitBreaker
	cbBuilder := circuitbreaker.NewBuilder[bool]().WithDelay(0)
	// And successful Fallback
	fbBuilder := fallback.NewBuilderWithResult[bool](true).HandleErrors(testutil.ErrInvalidArgument)
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

	assert.Equal(t, 0, stats.fbDone)
	assert.Equal(t, 1, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestListenersForFailingFallback(t *testing.T) {
	// Given - Fail 10 times
	stub, _ := testutil.ErrorNTimesThenReturn[bool](testutil.ErrInvalidState, 10)
	// Given successful RetryPolicy and CircuitBreaker
	rpBuilder := retrypolicy.NewBuilder[bool]().HandleErrors(testutil.ErrInvalidArgument)
	cbBuilder := circuitbreaker.NewBuilder[bool]().HandleErrors(testutil.ErrInvalidArgument).WithDelay(0)
	// And failing Fallback
	fbBuilder := fallback.NewBuilderWithError[bool](testutil.ErrConnecting)
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

	assert.Equal(t, 1, stats.fbDone)
	assert.Equal(t, 0, stats.fbSuccess)
	assert.Equal(t, 1, stats.fbFailure)

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 1, stats.failure)
}

func TestGetElapsedTime(t *testing.T) {
	rp := retrypolicy.NewBuilder[any]().
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
	rp := retrypolicy.NewBuilder[any]().HandleResult(nil).WithMaxRetries(1).
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
	rlBuilder := ratelimiter.NewSmoothBuilderWithMaxRate[any](100 * time.Millisecond)
	stats := &listenerStats{}
	registerRlListeners(stats, rlBuilder)
	executor := failsafe.NewExecutor[any](rlBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	executor.RunWithExecution(testutil.RunFn(nil)) // Success
	executor.RunWithExecution(testutil.RunFn(nil)) // Failure
	time.Sleep(110 * time.Millisecond)
	executor.RunWithExecution(testutil.RunFn(nil)) // Success
	executor.RunWithExecution(testutil.RunFn(nil)) // Failure
	executor.RunWithExecution(testutil.RunFn(nil)) // Failure

	// Then
	assert.Equal(t, 3, stats.rlExceeded)

	assert.Equal(t, 5, stats.done)
	assert.Equal(t, 2, stats.success)
	assert.Equal(t, 3, stats.failure)
}

func TestListenersForBulkhead(t *testing.T) {
	// Given
	bhBuilder := bulkhead.NewBuilder[any](2)
	stats := &listenerStats{}
	registerBhListeners(stats, bhBuilder)
	bh := bhBuilder.Build()
	executor := failsafe.NewExecutor[any](bh)
	registerExecutorListeners(stats, executor)
	assert.NoError(t, bh.AcquirePermit(context.Background()))
	assert.NoError(t, bh.AcquirePermit(context.Background()))

	// When
	assert.Error(t, bulkhead.ErrFull, executor.RunWithExecution(testutil.RunFn(nil)))
	assert.Error(t, bulkhead.ErrFull, executor.RunWithExecution(testutil.RunFn(nil)))
	bh.ReleasePermit()
	bh.ReleasePermit()

	// Then
	assert.Equal(t, 2, stats.bhFull)

	assert.Equal(t, 2, stats.done)
	assert.Equal(t, 0, stats.success)
	assert.Equal(t, 2, stats.failure)
}

func TestListenersForCache(t *testing.T) {
	_, fsCache := policytesting.NewCache[any]()
	cpBuilder := cachepolicy.NewBuilder(fsCache).WithKey("foo")
	stats := &listenerStats{}
	registerCpListeners(stats, cpBuilder)
	executor := failsafe.NewExecutor[any](cpBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	result, _ := executor.GetWithExecution(testutil.GetFn[any]("success1", nil))
	assert.Equal(t, "success1", result)
	result, _ = executor.GetWithExecution(testutil.GetFn[any]("success2", nil))
	assert.Equal(t, "success1", result)

	// Then
	assert.Equal(t, 1, stats.cpCached)
	assert.Equal(t, 1, stats.cpMiss)
	assert.Equal(t, 1, stats.cpHit)

	assert.Equal(t, 2, stats.done)
	assert.Equal(t, 2, stats.success)
	assert.Equal(t, 0, stats.failure)
}

func TestListenersForHedgePolicy(t *testing.T) {
	hpBuilder := hedgepolicy.NewBuilderWithDelay[bool](10 * time.Millisecond).WithMaxHedges(2)
	stats := &listenerStats{}
	registerHpListeners(stats, hpBuilder)
	executor := failsafe.NewExecutor[bool](hpBuilder.Build())
	registerExecutorListeners(stats, executor)

	// When
	result, err := executor.GetWithExecution(func(exec failsafe.Execution[bool]) (bool, error) {
		time.Sleep(100 * time.Millisecond)
		return true, nil
	})

	// Then
	assert.True(t, result)
	assert.NoError(t, err)
	assert.Equal(t, 2, stats.hpHedge)

	assert.Equal(t, 1, stats.done)
	assert.Equal(t, 1, stats.success)
	assert.Equal(t, 0, stats.failure)
}

// Asserts which listeners are called when a panic occurs.
func TestListenersOnPanic(t *testing.T) {
	// Given - Fail 2 times then panic
	panicValue := "test panic"
	stub := testutil.ErrorNTimesThenPanic[bool](testutil.ErrInvalidState, 2, panicValue)
	rpBuilder := retrypolicy.NewBuilder[bool]().WithMaxAttempts(10)
	cbBuilder := circuitbreaker.NewBuilder[bool]().WithDelay(0)
	fbBuilder := fallback.NewBuilderWithResult(true)
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

	assert.Equal(t, 0, stats.fbDone) // Done listener will not be called since the fallback is currently skipped on a panic
	assert.Equal(t, 0, stats.fbSuccess)
	assert.Equal(t, 0, stats.fbFailure)

	assert.Equal(t, 0, stats.done)    // Done listener is not called on a panic
	assert.Equal(t, 0, stats.success) // Success listener is not called on a panic
	assert.Equal(t, 0, stats.failure) // Failure listener is not called on a panic
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
	fbDone    int
	fbSuccess int
	fbFailure int

	// RateLimiter
	rlExceeded int

	// Buulkhead
	bhFull int

	// Hedge
	hpHedge int

	// Cache
	cpCached int
	cpHit    int
	cpMiss   int

	// Executor
	done    int
	success int
	failure int
}

func registerRpListeners[R any](stats *listenerStats, rpBuilder retrypolicy.Builder[R]) {
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

func registerCbListeners[R any](stats *listenerStats, cbBuilder circuitbreaker.Builder[R]) {
	cbBuilder.OnStateChanged(func(event circuitbreaker.StateChangedEvent) {
		fmt.Println("CircuitBreaker state change from", event.OldState, "to", event.NewState)
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

func registerFbListeners[R any](stats *listenerStats, fbBuilder fallback.Builder[R]) {
	fbBuilder.OnFallbackExecuted(func(f failsafe.ExecutionDoneEvent[R]) {
		stats.fbDone++
	}).OnFailure(func(e failsafe.ExecutionEvent[R]) {
		stats.fbFailure++
	}).OnSuccess(func(e failsafe.ExecutionEvent[R]) {
		stats.fbSuccess++
	})
}

func registerRlListeners[R any](stats *listenerStats, rlBuilder ratelimiter.Builder[R]) {
	rlBuilder.OnRateLimitExceeded(func(event failsafe.ExecutionEvent[R]) {
		stats.rlExceeded++
	})
}

func registerBhListeners[R any](stats *listenerStats, bhBuilder bulkhead.Builder[R]) {
	bhBuilder.OnFull(func(event failsafe.ExecutionEvent[R]) {
		stats.bhFull++
	})
}

func registerHpListeners[R any](stats *listenerStats, hpBuilder hedgepolicy.Builder[R]) {
	hpBuilder.OnHedge(func(f failsafe.ExecutionEvent[R]) {
		stats.hpHedge++
	})
}

func registerCpListeners[R any](stats *listenerStats, cpBuilder cachepolicy.Builder[R]) {
	cpBuilder.OnResultCached(func(event failsafe.ExecutionEvent[R]) {
		stats.cpCached++
	}).OnCacheHit(func(event failsafe.ExecutionDoneEvent[R]) {
		stats.cpHit++
	}).OnCacheMiss(func(event failsafe.ExecutionEvent[R]) {
		stats.cpMiss++
	})
}

func registerExecutorListeners[R any](stats *listenerStats, executor failsafe.Executor[R]) {
	executor.OnDone(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.done++
	}).OnFailure(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.failure++
	}).OnSuccess(func(e failsafe.ExecutionDoneEvent[R]) {
		stats.success++
	})
}
