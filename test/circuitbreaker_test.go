package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
	"failsafe/circuitbreaker"
	cbtesting "failsafe/internal/circuitbreaker_testutil"
	rptesting "failsafe/internal/retrypolicy_testutil"
	"failsafe/internal/testutil"
	"failsafe/retrypolicy"
)

func TestShouldRejectInitialExecutionWhenCircuitOpen(t *testing.T) {
	// Given
	cb := circuitbreaker.OfDefaults[any]()
	cb.Open()

	// When / Then
	testutil.TestRunFailure(t, failsafe.With(cb),
		func(execution failsafe.Execution[any]) error {
			return testutil.InvalidArgumentError{}
		},
		1, 0, circuitbreaker.ErrCircuitBreakerOpen)
	assert.True(t, cb.IsOpen())
}

// Should return ErrCircuitBreakerOpen when max half-open executions are occurring.
func TestShouldRejectExcessiveAttemptsWhenBreakerHalfOpen(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder().WithSuccessThreshold(3, 3).Build()
	cb.HalfOpen()
	waiter := testutil.NewWaiter()

	for i := 0; i < 3; i++ {
		go func() {
			failsafe.With(cb).Run(func() error {
				waiter.Resume()
				time.Sleep(1 * time.Minute)
				return nil
			})
		}()
	}

	// Assert that the breaker does not allow any more executions at the moment
	waiter.AwaitWithTimeout(3, 10*time.Second)
	for i := 0; i < 5; i++ {
		assert.ErrorIs(t, circuitbreaker.ErrCircuitBreakerOpen, failsafe.With(cb).Run(testutil.NoopFn))
	}
}

// Tests the handling of a circuit breaker with no failure conditions.
func TestCircuitBreakerWithoutConditions(t *testing.T) {
	// Given
	cb := circuitbreaker.BuilderForResult[bool]().WithDelay(0).Build()

	// When / Then
	testutil.TestRunFailure(t, failsafe.WithResult[bool](cb),
		func(execution failsafe.Execution[bool]) error {
			return testutil.InvalidArgumentError{}
		},
		1, 1, testutil.InvalidArgumentError{})
	assert.True(t, cb.IsOpen())

	// Given
	retryPolicy := retrypolicy.OfDefaults[bool]()
	counter := 0

	// When / Then
	testutil.TestGetSuccess[bool](t, failsafe.WithResult[bool](retryPolicy, cb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			counter++
			if counter < 3 {
				return false, testutil.InvalidArgumentError{}
			}
			return true, nil
		},
		3, 3, true)
	assert.True(t, cb.IsClosed())
}

func TestShouldReturnErrCircuitBreakerOpenAfterFailuresExceeded(t *testing.T) {
	// Given
	cb := circuitbreaker.BuilderForResult[bool]().
		WithFailureThreshold(circuitbreaker.NewCountBasedThreshold(2, 2)).
		HandleResult(false).
		WithDelay(10 * time.Second).
		Build()

	// When
	failsafe.WithResult[bool](cb).Get(testutil.GetFalseFn)
	failsafe.WithResult[bool](cb).Get(testutil.GetFalseFn)

	// Then
	testutil.TestGetFailure[bool](t, failsafe.WithResult[bool](cb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return true, nil
		},
		1, 0, circuitbreaker.ErrCircuitBreakerOpen)
	assert.True(t, cb.IsOpen())
}

// Tests a scenario where CircuitBreaker rejects some retried executions, which prevents the user's Supplier from being called.
func TestRejectedWithRetries(t *testing.T) {
	rpStats := &testutil.Stats{}
	cbStats := &testutil.Stats{}
	rp := rptesting.WithRetryStats(retrypolicy.Builder().WithMaxAttempts(7), rpStats).Build()
	cb := cbtesting.WithBreakerStats(circuitbreaker.Builder().
		WithFailureThreshold(circuitbreaker.NewCountBasedThreshold(3, 3)), cbStats).
		Build()

	testutil.TestRunFailure(t, failsafe.With(rp, cb),
		func(execution failsafe.Execution[any]) error {
			fmt.Println("Executing")
			return testutil.InvalidArgumentError{}
		},
		7, 3, circuitbreaker.ErrCircuitBreakerOpen)
	assert.Equal(t, 7, rpStats.FailedAttemptCount)
	assert.Equal(t, 6, rpStats.RetryCount)
	assert.Equal(t, uint(3), cb.GetExecutionCount())
	assert.Equal(t, uint(3), cb.GetFailureCount())
}

// Tests circuit breaker time based failure thresholding state transitions.
func TestSthouldSupportTimeBasedFailureThresholding(t *testing.T) {
	// Given
	cb := circuitbreaker.BuilderForResult[bool]().
		WithFailureThreshold(circuitbreaker.NewTimeBasedThreshold(2, 200*time.Millisecond).WithExecutionThreshold(3)).
		WithDelay(0).
		HandleResult(false).
		Build()
	executor := failsafe.WithResult[bool](cb)

	// When / Then
	executor.Get(testutil.GetFalseFn)
	executor.Get(testutil.GetTrueFn)
	// Force results to roll off
	time.Sleep(210 * time.Millisecond)
	executor.Get(testutil.GetFalseFn)
	executor.Get(testutil.GetTrueFn)
	// Force result to another bucket
	time.Sleep(50 * time.Millisecond)
	assert.True(t, cb.IsClosed())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsOpen())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsHalfOpen())
	// Half-open -> Open
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsOpen())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsHalfOpen())
	// Half-open -> Closed
	executor.Get(testutil.GetTrueFn)
	assert.True(t, cb.IsClosed())
}

// Tests circuit breaker time based failure rate thresholding state transitions.
func TestShouldSupportTimeBasedFailureRateThresholding(t *testing.T) {
	// Given
	cb := circuitbreaker.BuilderForResult[bool]().
		WithFailureThreshold(circuitbreaker.NewRateBasedThreshold(50, 3, 200*time.Millisecond)).
		WithDelay(0).
		HandleResult(false).
		Build()
	executor := failsafe.WithResult[bool](cb)

	// When / Then
	executor.Get(testutil.GetFalseFn)
	executor.Get(testutil.GetTrueFn)
	// Force results to roll off
	time.Sleep(210 * time.Millisecond)
	executor.Get(testutil.GetFalseFn)
	executor.Get(testutil.GetTrueFn)
	// Force result to another bucket
	time.Sleep(50 * time.Millisecond)
	executor.Get(testutil.GetTrueFn)
	assert.True(t, cb.IsClosed())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsOpen())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsHalfOpen())
	executor.Get(testutil.GetFalseFn)
	// Half-open -> Open
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsOpen())
	executor.Get(testutil.GetFalseFn)
	assert.True(t, cb.IsHalfOpen())
	executor.Get(testutil.GetTrueFn)
	// Half-open -> close
	executor.Get(testutil.GetTrueFn)
	assert.True(t, cb.IsClosed())
}
