package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

// Asserts that an execution is marked as canceled when a provided Context is canceled.
func TestCancelWithContextDuringExecution(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)

	// When / Then
	testutil.Test[any](t).
		With(rp).
		Context(setup).
		Run(func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		}).
		AssertFailure(1, 1, context.Canceled)
}

// Asserts that an execution is marked as canceled when a provided Context deadline is exceeded.
func TestCancelWithContextDeadlineDuringExecution(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()
	setup := func() context.Context {
		ctx, _ := context.WithTimeout(context.Background(), 100*time.Millisecond)
		return ctx
	}

	// When / Then
	testutil.Test[any](t).
		With(rp).
		Context(setup).
		Run(func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		}).
		AssertFailure(1, 1, context.DeadlineExceeded)
}

// Asserts that an execution is marked as canceled when it's canceled via an execution result.
// Also asserts that the context provided to an execution is canceled.
func TestCancelWithExecutionResultDuringExecution(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()

	// When
	result := failsafe.NewExecutor[any](rp).RunWithExecutionAsync(func(e failsafe.Execution[any]) error {
		testutil.WaitAndAssertCanceled(t, time.Second, e)
		return nil
	})
	assert.False(t, result.IsDone())
	time.Sleep(100 * time.Millisecond)
	result.Cancel()

	// Then
	res, err := result.Get()
	assert.True(t, result.IsDone())
	assert.Nil(t, res)
	assert.ErrorIs(t, err, failsafe.ErrExecutionCanceled)
}

// Asserts that an execution is marked as canceled when a timeout is exceeded.
// Also asserts that the context provided to an execution is canceled.
func TestCancelWithTimeoutDuringExecution(t *testing.T) {
	// Given
	to := timeout.With[any](100 * time.Millisecond)
	executor := failsafe.NewExecutor[any](to).WithContext(context.Background())

	// When / Then
	testutil.Test[any](t).
		WithExecutor(executor).
		Run(func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		}).
		AssertFailure(1, 1, timeout.ErrExceeded)
}

// Asserts that when a RetryPolicy is blocked on a delay, canceling the context results in a Canceled error being returned.
func TestCancelWithContextDuringPendingRetry(t *testing.T) {
	// Given
	rp := policytesting.WithRetryLogs[any](retrypolicy.Builder[any]().WithDelay(time.Second)).Build()
	setup := testutil.SetupWithContextSleep(50 * time.Millisecond)

	// When / Then
	testutil.Test[any](t).
		With(rp).
		Context(setup).
		Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
		AssertFailure(1, 1, context.Canceled)
}

// Asserts that waiting on a cancelation from an ExecutionResult works from during a retry delay.
func TestCancelWithExecutionResultDuringPendingRetry(t *testing.T) {
	// Given
	rp := policytesting.WithRetryLogs[any](retrypolicy.Builder[any]().WithDelay(time.Second)).Build()

	// When
	result := failsafe.NewExecutor[any](rp).RunAsync(func() error {
		return testutil.ErrInvalidState
	})
	assert.False(t, result.IsDone())
	time.Sleep(100 * time.Millisecond)
	result.Cancel()

	// Then
	res, err := result.Get()
	assert.True(t, result.IsDone())
	assert.Nil(t, res)
	assert.ErrorIs(t, err, failsafe.ErrExecutionCanceled)
}

// Asserts that a cancellation with a fallback returns the expected error.
func TestCancelWithContextWithFallback(t *testing.T) {
	// Given
	fb := fallback.WithError[any](nil)
	setup := testutil.SetupWithContextSleep(50 * time.Millisecond)

	// When / Then
	testutil.Test[any](t).
		With(fb).
		Context(setup).
		Get(func(execution failsafe.Execution[any]) (any, error) {
			time.Sleep(200 * time.Millisecond)
			return nil, testutil.ErrInvalidArgument
		}).
		AssertFailure(1, 1, context.Canceled)
}

// Asserts that waiting on a cancelation from a context works from within a fallback function.
func TestCancelWithContextDuringFallbackFn(t *testing.T) {
	// Given
	fb := fallback.WithFunc(func(exec failsafe.Execution[any]) (any, error) {
		testutil.WaitAndAssertCanceled(t, time.Second, exec)
		return nil, nil
	})
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)

	// When / Then
	testutil.Test[any](t).
		With(fb).
		Context(setup).
		Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
		AssertFailure(1, 1, context.Canceled)
}

// Asserts that waiting on a cancelation from an ExecutionResult works from within a fallback function.
func TestCancelWithExecutionResultDuringFallbackFn(t *testing.T) {
	// Given
	fb := fallback.WithFunc(func(exec failsafe.Execution[any]) (any, error) {
		testutil.WaitAndAssertCanceled(t, time.Second, exec)
		return nil, nil
	})

	// When
	result := failsafe.NewExecutor[any](fb).RunAsync(func() error {
		return testutil.ErrInvalidState
	})
	assert.False(t, result.IsDone())
	time.Sleep(100 * time.Millisecond)
	result.Cancel()

	// Then
	res, err := result.Get()
	assert.True(t, result.IsDone())
	assert.Nil(t, res)
	assert.ErrorIs(t, err, failsafe.ErrExecutionCanceled)
}

// Asserts that when a RateLimiter is blocked on a delay, canceling the context results in a Canceled error being returned.
func TestCancelWithContextDuringRateLimiterDelay(t *testing.T) {
	// Given
	rl := ratelimiter.SmoothBuilderWithMaxRate[any](time.Second).WithMaxWaitTime(time.Minute).Build()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)
	rl.TryAcquirePermit() // All permits used

	// When / Then
	testutil.Test[any](t).
		With(rl).
		Context(setup).
		Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
		AssertFailure(1, 0, context.Canceled)
}

// Asserts that when a RateLimiter is blocked on a delay, canceling with a timeout results in the rate limiter being unblocked.
func TestCancelWithTimeoutDuringRateLimiterDelay(t *testing.T) {
	// Given
	rl := ratelimiter.SmoothBuilder[any](1, 30*time.Second).WithMaxWaitTime(time.Minute).Build()
	to := timeout.With[any](100 * time.Millisecond)
	rl.TryAcquirePermit() // All permits used

	// When / Then
	testutil.Test[any](t).
		With(to, rl).
		Get(testutil.GetFn[any](nil, nil)).
		AssertFailure(1, 0, timeout.ErrExceeded)
}

// Asserts that an execution is marked as canceled when it's canceled via an execution result.
// Also asserts that the context provided to an execution is canceled.
func TestCancelWithExecutionResultDuringRateLimiterDelay(t *testing.T) {
	// Given
	rl := ratelimiter.SmoothBuilder[any](1, 30*time.Second).WithMaxWaitTime(time.Minute).Build()
	rl.TryAcquirePermit() // All permits used

	// When
	result := failsafe.NewExecutor[any](rl).RunAsync(func() error {
		return nil
	})
	assert.False(t, result.IsDone())
	time.Sleep(100 * time.Millisecond)
	result.Cancel()

	// Then
	res, err := result.Get()
	assert.True(t, result.IsDone())
	assert.Nil(t, res)
	assert.ErrorIs(t, err, failsafe.ErrExecutionCanceled)
}

func TestCancelWithContextDuringBulkheadDelay(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(200 * time.Millisecond).Build()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.Test[any](t).
		With(bh).
		Context(setup).
		Get(testutil.GetFn[any](nil, nil)).
		AssertFailure(1, 0, context.Canceled)
}

func TestCancelWithTimeoutDuringBulkheadDelay(t *testing.T) {
	// Given
	to := timeout.With[any](10 * time.Millisecond)
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(100 * time.Millisecond).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.Test[any](t).
		With(to, bh).
		Get(testutil.GetFn[any](nil, nil)).
		AssertFailure(1, 0, timeout.ErrExceeded)
}

// Asserts that waiting on a cancelation from an ExecutionResult works from within a bulkhead function.
func TestCancelWithExecutionResultDuringBulkheadDelay(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(time.Second).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When
	result := failsafe.NewExecutor[any](bh).RunAsync(func() error {
		return testutil.ErrInvalidState
	})
	assert.False(t, result.IsDone())
	time.Sleep(100 * time.Millisecond)
	result.Cancel()

	// Then
	res, err := result.Get()
	assert.True(t, result.IsDone())
	assert.Nil(t, res)
	assert.ErrorIs(t, err, failsafe.ErrExecutionCanceled)
}

// Tests canceling an execution that is blocked, before hedges have started.
func TestCancelWithContextBeforeHedge(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](time.Second).WithMaxHedges(2), stats).Build()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)

	// When / Then
	testutil.Test[any](t).
		With(hp).
		Reset(stats).
		Context(setup).
		Run(func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		}).
		AssertFailure(1, 1, context.Canceled, func() {
			assert.Equal(t, 0, stats.Hedges())
		})
}

// Tests canceling an execution after hedges have been started.
func TestCancelWithContextDuringHedge(t *testing.T) {
	// Given
	hp := hedgepolicy.BuilderWithDelay[any](10 * time.Millisecond).WithMaxHedges(2).Build()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)
	waiter := testutil.NewWaiter()

	// When / Then
	testutil.Test[any](t).
		With(hp).
		Context(setup).
		Run(func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			waiter.Resume()
			return nil
		}).
		AssertFailure(3, -1, context.Canceled, func() {
			waiter.AwaitWithTimeout(3, time.Second)
		})
}

func TestCancelWithTimeoutDuringHedge(t *testing.T) {
	// Given
	to := timeout.With[any](100 * time.Millisecond)
	hp := hedgepolicy.BuilderWithDelay[any](10 * time.Millisecond).WithMaxHedges(2).Build()
	waiter := testutil.NewWaiter()

	// When / Then
	testutil.Test[any](t).
		With(to, hp).
		Run(func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			waiter.Resume()
			return nil
		}).
		AssertFailure(3, -1, timeout.ErrExceeded, func() {
			waiter.AwaitWithTimeout(3, 3*time.Second)
		})
}

// Tests a scenario where a canceled channel is closed before it's accessed, which should use the internally shared
// closedChan.
func TestCloseCanceledChannelBeforeAccessingIt(t *testing.T) {
	// Given
	to := timeout.With[any](10 * time.Millisecond)

	// When / Then
	testutil.Test[any](t).
		With(to).
		Run(func(e failsafe.Execution[any]) error {
			time.Sleep(100 * time.Millisecond)
			<-e.Canceled()
			return nil
		}).
		AssertFailure(1, 1, timeout.ErrExceeded)
}
