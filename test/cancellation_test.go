package test

import (
	"context"
	"net/http"
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

// Asserts that an execution is marked as canceled when a timeout is exceeded.
// Also asserts that the context provided to an execution is canceled.
func TestCancelWithTimeoutDuringExecution(t *testing.T) {
	// Given
	to := timeout.With[any](100 * time.Millisecond)
	executor := failsafe.NewExecutor[any](to).WithContext(context.Background())

	// When / Then
	testutil.TestRunFailure(t, nil, executor,
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		},
		1, 1, timeout.ErrExceeded)
}

// Asserts that an execution is marked as canceled when a provided Context is canceled.
func TestCancelWithContextDuringExecution(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)

	// When / Then
	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](rp),
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		},
		1, 1, context.Canceled)
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
	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](rp),
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		},
		1, 1, context.DeadlineExceeded)
}

// Asserts that an execution is marked as canceled when it's canceled via an execution result.
// Also asserts that the context provided to an execution is canceled.
func TestCancelWithExecutionResult(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()

	// When
	executor := failsafe.NewExecutor[any](rp).WithContext(context.Background())
	result := executor.RunWithExecutionAsync(func(e failsafe.Execution[any]) error {
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

// Asserts that when a RetryPolicy is blocked on a delay, canceling the context results in a Canceled error being returned.
func TestCancelWithContextDuringPendingRetry(t *testing.T) {
	// Given
	rp := policytesting.WithRetryLogs[any](retrypolicy.Builder[any]().WithDelay(time.Second)).Build()
	setup := testutil.SetupWithContextSleep(50 * time.Millisecond)

	// When / Then
	testutil.TestGetFailure(t, setup, failsafe.NewExecutor[any](rp),
		testutil.GetWithExecutionFn[any](nil, testutil.ErrInvalidState),
		1, 1, context.Canceled)
}

// Asserts that a cancellation with a fallback returns the expected error.
func TestCancelWithContextWithFallback(t *testing.T) {
	// Given
	fb := fallback.WithError[any](nil)
	setup := testutil.SetupWithContextSleep(50 * time.Millisecond)

	// When / Then
	testutil.TestGetFailure(t, setup, failsafe.NewExecutor[any](fb),
		func(execution failsafe.Execution[any]) (any, error) {
			time.Sleep(200 * time.Millisecond)
			return nil, testutil.ErrInvalidArgument
		},
		1, 1, context.Canceled)
}

// Asserts that waiting on a cancelation works from within a fallback function.
func TestCancelWithContextDuringFallbackFn(t *testing.T) {
	// Given
	fb := fallback.WithFunc(func(exec failsafe.Execution[any]) (any, error) {
		testutil.WaitAndAssertCanceled(t, time.Second, exec)
		return nil, nil
	})
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)

	// When / Then
	testutil.TestGetFailure(t, setup, failsafe.NewExecutor[any](fb),
		testutil.GetWithExecutionFn[any](nil, testutil.ErrInvalidState),
		1, 1, context.Canceled)
}

// Asserts that when a RateLimiter is blocked on a delay, canceling the context results in a Canceled error being returned.
func TestCancelWithContextDuringRateLimiterDelay(t *testing.T) {
	// Given
	rl := ratelimiter.SmoothBuilderWithMaxRate[any](time.Second).WithMaxWaitTime(time.Minute).Build()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)
	rl.TryAcquirePermit() // All permits used

	// When / Then
	testutil.TestGetFailure(t, setup, failsafe.NewExecutor[any](rl),
		testutil.GetWithExecutionFn[any](nil, nil),
		1, 0, context.Canceled)
}

// Asserts that when a RateLimiter is blocked on a delay, canceling with a timeout results in the rate limiter being unblocked.
func TestCancelWithTimeoutDuringRateLimiterDelay(t *testing.T) {
	// Given
	rl := ratelimiter.SmoothBuilder[any](1, 30*time.Second).WithMaxWaitTime(time.Minute).Build()
	to := timeout.With[any](100 * time.Millisecond)
	rl.TryAcquirePermit() // All permits used

	// When / Then
	testutil.TestGetFailure(t, nil, failsafe.NewExecutor[any](to, rl),
		testutil.GetWithExecutionFn[any](nil, nil),
		1, 0, timeout.ErrExceeded)
}

func TestCancelWithContextDuringBulkheadDelay(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(200 * time.Millisecond).Build()
	setup := testutil.SetupWithContextSleep(100 * time.Millisecond)
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.TestGetFailure(t, setup, failsafe.NewExecutor[any](bh),
		testutil.GetWithExecutionFn[any](nil, nil),
		1, 0, context.Canceled)
}

func TestCancelWithTimeoutDuringBulkheadDelay(t *testing.T) {
	// Given
	to := timeout.With[any](10 * time.Millisecond)
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(100 * time.Millisecond).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.TestGetFailure(t, nil, failsafe.NewExecutor[any](to, bh),
		testutil.GetWithExecutionFn[any](nil, nil),
		1, 0, timeout.ErrExceeded)
}

// Tests canceling an execution that is blocked, before hedges have started.
func TestCancelWithContextBeforeHedge(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](time.Second).WithMaxHedges(2), stats).Build()
	setupInner := testutil.SetupWithContextSleep(100 * time.Millisecond)
	setup := func() context.Context {
		stats.Reset()
		return setupInner()
	}

	// When / Then
	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](hp),
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		},
		1, 1, context.Canceled, func() {
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
	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](hp),
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			waiter.Resume()
			return nil
		},
		3, -1, context.Canceled, func() {
			waiter.AwaitWithTimeout(3, time.Second)
		})
}

func TestCancelWithTimeoutDuringHedge(t *testing.T) {
	// Given
	to := timeout.With[any](100 * time.Millisecond)
	hp := hedgepolicy.BuilderWithDelay[any](10 * time.Millisecond).WithMaxHedges(2).Build()
	waiter := testutil.NewWaiter()

	// When / Then
	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](to, hp),
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			waiter.Resume()
			return nil
		},
		3, -1, timeout.ErrExceeded, func() {
			waiter.AwaitWithTimeout(3, 3*time.Second)
		})
}

// Tests that a failsafe roundtripper's requests are canceled when an external context is canceled.
func TestCancelWithContextDuringHttpRequest(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	rp := retrypolicy.WithDefaults[*http.Response]()
	ctx := testutil.SetupWithContextSleep(50 * time.Millisecond)()
	executor := failsafe.NewExecutor[*http.Response](rp).WithContext(ctx)

	// When / Then
	testutil.TestRequestFailureError(t, server.URL, executor,
		1, 1, context.Canceled)
}

// Tests that a failsafe roundtripper's requests are canceled when an external context is canceled.
func TestCancelWithTimeoutDuringHttpRequest(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	to := timeout.With[*http.Response](100 * time.Millisecond)
	executor := failsafe.NewExecutor[*http.Response](to)

	// When / Then
	testutil.TestRequestFailureError(t, server.URL, executor,
		1, 1, timeout.ErrExceeded)
}

// Tests a scenario where a canceled channel is closed before it's accessed, which should use the internally shared
// closedChan.
func TestCloseCanceledChannelBeforeAccessingIt(t *testing.T) {
	// Given
	to := timeout.With[any](10 * time.Millisecond)

	// When / Then
	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](to),
		func(e failsafe.Execution[any]) error {
			time.Sleep(100 * time.Millisecond)
			<-e.Canceled()
			return nil
		},
		1, 1, timeout.ErrExceeded)
}
