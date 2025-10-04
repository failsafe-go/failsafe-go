package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func TestExecution_Cancellation(t *testing.T) {
	// Asserts that an execution is marked as canceled when a provided Context is canceled.
	t.Run("with Context", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewWithDefaults[any]()
		ctxFn := testutil.ContextWithCancel(100 * time.Millisecond)

		// When / Then
		testutil.Test[any](t).
			With(rp).
			Context(ctxFn).
			Run(func(exec failsafe.Execution[any]) error {
				testutil.WaitAndAssertCanceled(t, time.Second, exec)
				return nil
			}).
			AssertFailure(1, 1, context.Canceled)
	})

	// Asserts that an execution is marked as canceled when a provided Context deadline is exceeded.
	t.Run("with Deadline", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewWithDefaults[any]()
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
	})

	// Asserts that an execution is marked as canceled when it's canceled via an execution result.
	// Also asserts that the context provided to an execution is canceled.
	t.Run("with ExecutionResult", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewWithDefaults[any]()

		// When
		result := failsafe.With(rp).RunAsyncWithExecution(func(e failsafe.Execution[any]) error {
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
	})
}

func TestTimeout_Cancellation(t *testing.T) {
	// Asserts that an execution is marked as canceled when a timeout is exceeded.
	// Also asserts that the context provided to an execution is canceled.
	t.Run("during execution", func(t *testing.T) {
		// Given
		to := timeout.New[any](100 * time.Millisecond)
		executor := failsafe.With(to).WithContext(context.Background())

		// When / Then
		testutil.Test[any](t).
			WithExecutor(executor).
			Run(func(exec failsafe.Execution[any]) error {
				testutil.WaitAndAssertCanceled(t, time.Second, exec)
				return nil
			}).
			AssertFailure(1, 1, timeout.ErrExceeded)
	})
}

func TestAdaptiveLimiter_Cancellation(t *testing.T) {
	t.Run("while blocked waiting for permit", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			WithMaxWaitTime(time.Second).
			Build()
		shouldAcquire(t, limiter) // limiter should be full
		ctxFn := testutil.ContextWithCancel(100 * time.Millisecond)

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Context(ctxFn).
			Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
			AssertFailure(1, 0, context.Canceled)
	})
}

func TestRetryPolicy_Cancellation(t *testing.T) {
	// Asserts that when a RetryPolicy is blocked on a delay, canceling the context results in a Canceled error being returned.
	t.Run("with context during pending retry", func(t *testing.T) {
		// Given
		rp := policytesting.WithRetryLogs[any](retrypolicy.NewBuilder[any]().WithDelay(time.Second)).Build()
		ctxFn := testutil.ContextWithCancel(50 * time.Millisecond)

		// When / Then
		testutil.Test[any](t).
			With(rp).
			Context(ctxFn).
			Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
			AssertFailure(1, 1, context.Canceled)
	})

	// Asserts that waiting on a cancelation from an ExecutionResult works from during a retry delay.
	t.Run("with ExecutionResult during successive retry", func(t *testing.T) {
		// Given
		rp := policytesting.WithRetryLogs[any](retrypolicy.NewBuilder[any]().WithDelay(time.Second)).Build()

		// When
		result := failsafe.With(rp).RunAsync(func() error {
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
	})
}

func TestFallback_Cancellation(t *testing.T) {
	// Asserts that a cancellation with a fallback returns the expected error.
	t.Run("with context during execution", func(t *testing.T) {
		// Given
		fb := fallback.NewWithError[any](nil)
		ctxFn := testutil.ContextWithCancel(50 * time.Millisecond)

		// When / Then
		testutil.Test[any](t).
			With(fb).
			Context(ctxFn).
			Get(func(execution failsafe.Execution[any]) (any, error) {
				time.Sleep(200 * time.Millisecond)
				return nil, testutil.ErrInvalidArgument
			}).
			AssertFailure(1, 1, context.Canceled)
	})

	// Asserts that waiting on a cancelation from a context works from within a fallback function.
	t.Run("with context during fallback func", func(t *testing.T) {
		// Given
		fb := fallback.NewWithFunc(func(exec failsafe.Execution[any]) (any, error) {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil, nil
		})
		ctxFn := testutil.ContextWithCancel(100 * time.Millisecond)

		// When / Then
		testutil.Test[any](t).
			With(fb).
			Context(ctxFn).
			Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
			AssertFailure(1, 1, context.Canceled)
	})

	// Asserts that waiting on a cancelation from an ExecutionResult works from within a fallback function.
	t.Run("with ExecutionResult during fallback func", func(t *testing.T) {
		// Given
		fb := fallback.NewWithFunc(func(exec failsafe.Execution[any]) (any, error) {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil, nil
		})

		// When
		result := failsafe.With(fb).RunAsync(func() error {
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
	})
}

func TestRateLimit_Cancellation(t *testing.T) {
	// Asserts that when a RateLimiter is blocked on a delay, canceling the context results in a Canceled error being returned.
	t.Run("with context during RateLimiter delay", func(t *testing.T) {
		// Given
		rl := ratelimiter.NewSmoothBuilderWithMaxRate[any](time.Second).WithMaxWaitTime(time.Minute).Build()
		ctxFn := testutil.ContextWithCancel(100 * time.Millisecond)
		rl.TryAcquirePermit() // All permits used

		// When / Then
		testutil.Test[any](t).
			With(rl).
			Context(ctxFn).
			Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
			AssertFailure(1, 0, context.Canceled)
	})

	// Asserts that when a RateLimiter is blocked on a delay, canceling with a timeout results in the rate limiter being unblocked.
	t.Run("with Timeout during RateLimiter delay", func(t *testing.T) {
		// Given
		rl := ratelimiter.NewSmoothBuilder[any](1, 30*time.Second).WithMaxWaitTime(time.Minute).Build()
		to := timeout.New[any](100 * time.Millisecond)
		rl.TryAcquirePermit() // All permits used

		// When / Then
		testutil.Test[any](t).
			With(to, rl).
			Get(testutil.GetFn[any](nil, nil)).
			AssertFailure(1, 0, timeout.ErrExceeded)
	})

	// Asserts that an execution is marked as canceled when it's canceled via an execution result.
	// Also asserts that the context provided to an execution is canceled.
	t.Run("with ExecutionResult during RateLimiter delay", func(t *testing.T) {
		// Given
		rl := ratelimiter.NewSmoothBuilder[any](1, 30*time.Second).WithMaxWaitTime(time.Minute).Build()
		rl.TryAcquirePermit() // All permits used

		// When
		result := failsafe.With(rl).RunAsync(func() error {
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
	})
}

func TestBulkhead_Cancellation(t *testing.T) {
	t.Run("with context during Bulkhead delay", func(t *testing.T) {
		// Given
		bh := bulkhead.NewBuilder[any](2).WithMaxWaitTime(200 * time.Millisecond).Build()
		ctxFn := testutil.ContextWithCancel(100 * time.Millisecond)
		bh.TryAcquirePermit()
		bh.TryAcquirePermit() // bulkhead should be full

		// When / Then
		testutil.Test[any](t).
			With(bh).
			Context(ctxFn).
			Get(testutil.GetFn[any](nil, nil)).
			AssertFailure(1, 0, context.Canceled)
	})

	t.Run("with Timeout during Bulkhead delay", func(t *testing.T) {
		// Given
		to := timeout.New[any](10 * time.Millisecond)
		bh := bulkhead.NewBuilder[any](2).WithMaxWaitTime(100 * time.Millisecond).Build()
		bh.TryAcquirePermit()
		bh.TryAcquirePermit() // bulkhead should be full

		// When / Then
		testutil.Test[any](t).
			With(to, bh).
			Get(testutil.GetFn[any](nil, nil)).
			AssertFailure(1, 0, timeout.ErrExceeded)
	})

	// Asserts that waiting on a cancelation from an ExecutionResult works from within a bulkhead function.
	t.Run("with ExecutionResult during Bulkhead delay", func(t *testing.T) {
		// Given
		bh := bulkhead.NewBuilder[any](2).WithMaxWaitTime(time.Second).Build()
		bh.TryAcquirePermit()
		bh.TryAcquirePermit() // bulkhead should be full

		// When
		result := failsafe.With(bh).RunAsync(func() error {
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
	})
}

func TestHedgePolicy_Cancellation(t *testing.T) {
	// Tests canceling an execution that is blocked, before hedges have started.
	t.Run("with context before hedge", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[any](time.Second).WithMaxHedges(2), stats).Build()
		ctxFn := testutil.ContextWithCancel(100 * time.Millisecond)

		// When / Then
		testutil.Test[any](t).
			With(hp).
			Reset(stats).
			Context(ctxFn).
			Run(func(exec failsafe.Execution[any]) error {
				testutil.WaitAndAssertCanceled(t, time.Second, exec)
				return nil
			}).
			AssertFailure(1, 1, context.Canceled, func() {
				assert.Equal(t, 0, stats.Hedges())
			})
	})

	// Tests canceling an execution after hedges have been started.
	t.Run("with context during hedge", func(t *testing.T) {
		// Given
		hp := hedgepolicy.NewBuilderWithDelay[any](10 * time.Millisecond).WithMaxHedges(2).Build()
		ctxFn := testutil.ContextWithCancel(100 * time.Millisecond)
		waiter := testutil.NewWaiter()

		// When / Then
		testutil.Test[any](t).
			With(hp).
			Context(ctxFn).
			Run(func(exec failsafe.Execution[any]) error {
				testutil.WaitAndAssertCanceled(t, time.Second, exec)
				waiter.Resume()
				return nil
			}).
			AssertFailure(3, -1, context.Canceled, func() {
				waiter.AwaitWithTimeout(3, time.Second)
			})
	})

	t.Run("with Timeout during hedge", func(t *testing.T) {
		// Given
		to := timeout.New[any](100 * time.Millisecond)
		hp := hedgepolicy.NewBuilderWithDelay[any](10 * time.Millisecond).WithMaxHedges(2).Build()
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
	})
}

// Tests a scenario where a canceled channel is closed before it's accessed, which should use the internally shared
// closedChan.
func TestCloseCanceledChannelBeforeAccessingIt(t *testing.T) {
	// Given
	to := timeout.New[any](10 * time.Millisecond)

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
