package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

// Asserts that an execution is marked as canceled when a timeout is exceeded.
func TestCancelWithTimeoutDuringExecution(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// When
	err := failsafe.NewExecutor[any](rp).WithContext(ctx).RunWithExecution(func(exec failsafe.Execution[any]) error {
		testutil.WaitAndAssertCanceled(t, time.Second, exec)
		return nil
	})

	// Then
	assert.Error(t, context.Canceled, err)
}

// Asserts that an execution is marked as canceled when a provided Context is canceled.
func TestCancelWithContextDuringExecution(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// When
	err := failsafe.NewExecutor[any](rp).WithContext(ctx).RunWithExecution(func(exec failsafe.Execution[any]) error {
		testutil.WaitAndAssertCanceled(t, time.Second, exec)
		return nil
	})

	// Then
	assert.Error(t, context.Canceled, err)
}

// Asserts that an execution is marked as canceled when a provided Context deadline is exceeded.
func TestCancelWithContextDeadlingDuringExecution(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()
	to := timeout.With[any](100 * time.Millisecond)

	// When
	err := failsafe.RunWithExecution(func(exec failsafe.Execution[any]) error {
		testutil.WaitAndAssertCanceled(t, time.Second, exec)
		return nil
	}, to, rp)

	// Then
	assert.Error(t, timeout.ErrTimeoutExceeded, err)
}

// Asserts that when a RetryPolicy is blocked on a delay, canceling the context results in a Canceled error being returned.
func TestCancelWithContextDuringPendingRetry(t *testing.T) {
	// Given
	rp := policytesting.WithRetryLogs[any](retrypolicy.Builder[any]().WithDelay(time.Second)).Build()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// When / Then
	testutil.TestGetFailure(t, failsafe.NewExecutor[any](rp).WithContext(ctx),
		testutil.GetWithExecutionFn[any](nil, testutil.ErrInvalidState),
		1, 1, context.Canceled)
}

func TestCancelWithContextDuringFallbackFn(t *testing.T) {
	// Given
	fb := fallback.WithFn[any](func(exec failsafe.Execution[any]) (any, error) {
		testutil.WaitAndAssertCanceled(t, time.Second, exec)
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// When
	err := failsafe.NewExecutor[any](fb).WithContext(ctx).Run(testutil.RunFn(testutil.ErrInvalidState))

	// Then
	assert.Error(t, context.Canceled, err)
}

// Asserts that when a RateLimiter is blocked on a delay, canceling the context results in a Canceled error being returned.
func TestCancelWithContextDuringRateLimiterDelay(t *testing.T) {
	// Given
	rl := ratelimiter.SmoothBuilderWithMaxRate[any](time.Minute).WithMaxWaitTime(time.Minute).Build()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	rl.TryAcquirePermit() // All permits used

	// When
	err := failsafe.NewExecutor[any](rl).WithContext(ctx).RunWithExecution(func(exec failsafe.Execution[any]) error {
		return nil
	})

	// Then
	assert.Error(t, context.Canceled, err)
}

// Asserts that when a RateLimiter is blocked on a delay, canceling with a timeout results in the rate limiter being unblocked.
func TestCancelWithTimeoutDuringRateLimiterDelay(t *testing.T) {
	// Given
	rl := ratelimiter.SmoothBuilderWithMaxRate[any](time.Minute).WithMaxWaitTime(time.Minute).Build()
	to := timeout.With[any](100 * time.Millisecond)
	rl.TryAcquirePermit() // All permits used

	// When
	err := failsafe.RunWithExecution(func(exec failsafe.Execution[any]) error {
		return nil
	}, to, rl)

	// Then
	assert.Error(t, timeout.ErrTimeoutExceeded, err)
}
