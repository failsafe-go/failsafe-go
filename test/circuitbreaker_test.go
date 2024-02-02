package test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestShouldRejectInitialExecutionWhenCircuitOpen(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[any]()
	cb.Open()

	// When / Then
	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](cb),
		func(execution failsafe.Execution[any]) error {
			return testutil.ErrInvalidArgument
		},
		1, 0, circuitbreaker.ErrOpen)
	assert.True(t, cb.IsOpen())
}

// Should return ErrOpen when max half-open executions are occurring.
func TestShouldRejectExcessiveAttemptsWhenBreakerHalfOpen(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder[any]().WithSuccessThreshold(3).Build()
	cb.HalfOpen()
	waiter := testutil.NewWaiter()

	for i := 0; i < 3; i++ {
		go func() {
			failsafe.Run(func() error {
				waiter.Resume()
				time.Sleep(1 * time.Minute)
				return nil
			}, cb)
		}()
	}

	// Assert that the breaker does not allow any more executions at the moment
	waiter.AwaitWithTimeout(3, 10*time.Second)
	for i := 0; i < 5; i++ {
		assert.ErrorIs(t, circuitbreaker.ErrOpen, failsafe.NewExecutor[any](cb).Run(testutil.NoopFn))
	}
}

// Tests the handling of a circuit breaker with no failure conditions.
func TestCircuitBreakerWithoutConditions(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder[bool]().WithDelay(0).Build()

	//	When / Then
	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[bool](cb),
		func(execution failsafe.Execution[bool]) error {
			return testutil.ErrInvalidArgument
		},
		1, 1, testutil.ErrInvalidArgument)
	assert.True(t, cb.IsOpen())

	// Given
	var counter int
	retryPolicy := retrypolicy.WithDefaults[bool]()
	setup := func() context.Context {
		counter = 0
		return nil
	}

	// When / Then
	testutil.TestGetSuccess[bool](t, setup, failsafe.NewExecutor[bool](retryPolicy, cb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			counter++
			if counter < 3 {
				return false, testutil.ErrInvalidArgument
			}
			return true, nil
		},
		3, 3, true)
	assert.True(t, cb.IsClosed())
}

func TestShouldReturnErrCircuitBreakerOpenAfterFailuresExceeded(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder[bool]().
		WithFailureThreshold(2).
		HandleResult(false).
		WithDelay(10 * time.Second).
		Build()

	// When
	failsafe.Get(testutil.GetFalseFn, cb)
	failsafe.Get(testutil.GetFalseFn, cb)

	// Then
	testutil.TestGetFailure[bool](t, nil, failsafe.NewExecutor[bool](cb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return true, nil
		},
		1, 0, circuitbreaker.ErrOpen)
	assert.True(t, cb.IsOpen())
}

// Tests a scenario where CircuitBreaker rejects some retried executions, which prevents the user's Supplier from being called.
func TestRejectedWithRetries(t *testing.T) {
	rpStats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.Builder[any]().WithMaxAttempts(7), rpStats).Build()
	cb := circuitbreaker.Builder[any]().WithFailureThreshold(3).Build()
	setup := func() context.Context {
		policytesting.ResetCircuitBreaker(cb)
		rpStats.Reset()
		return nil
	}

	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](rp, cb),
		func(execution failsafe.Execution[any]) error {
			fmt.Println("Executing")
			return testutil.ErrInvalidArgument
		},
		7, 3, circuitbreaker.ErrOpen)
	assert.Equal(t, 7, rpStats.Executions())
	assert.Equal(t, 6, rpStats.Retries())
	assert.Equal(t, uint(3), cb.Metrics().Executions())
	assert.Equal(t, uint(3), cb.Metrics().Failures())
}

// Tests circuit breaker time based failure thresholding state transitions.
func TestSthouldSupportTimeBasedFailureThresholding(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder[bool]().
		WithFailureThresholdPeriod(2, 200*time.Millisecond).
		WithDelay(0).
		HandleResult(false).
		Build()
	executor := failsafe.NewExecutor[bool](cb)

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
	cb := circuitbreaker.Builder[bool]().
		WithFailureRateThreshold(50, 3, 200*time.Millisecond).
		WithDelay(0).
		HandleResult(false).
		Build()
	executor := failsafe.NewExecutor[bool](cb)

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

func TestShouldReturnOpenErrorWithRemainingDelay(t *testing.T) {
	breaker := circuitbreaker.Builder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 1 * time.Second
	}).Build()
	breaker.Open()

	// When / Then
	err := failsafe.Run(func() error {
		return errors.New("test")
	}, breaker)
	var cbErr *circuitbreaker.OpenError
	assert.True(t, errors.As(err, &cbErr))
	assert.True(t, cbErr.RemainingDelay() > 0)
	assert.True(t, cbErr.RemainingDelay().Milliseconds() < 1001)
}
