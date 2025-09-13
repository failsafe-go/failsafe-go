package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestCircuitBreaker(t *testing.T) {
	t.Run("should reject initial execution when circuit open", func(t *testing.T) {
		// Given
		cb := circuitbreaker.NewWithDefaults[any]()
		cb.Open()

		// When / Then
		testutil.Test[any](t).
			With(cb).
			Run(testutil.RunFn(testutil.ErrInvalidArgument)).
			AssertFailure(1, 0, circuitbreaker.ErrOpen, func() {
				assert.True(t, cb.IsOpen())
			})
	})

	// Should return ErrOpen when max half-open executions are occurring.
	t.Run("should reject excessive attempts when breaker half-open", func(t *testing.T) {
		// Given
		cb := circuitbreaker.NewBuilder[any]().WithSuccessThreshold(3).Build()
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
	})

	// Tests the handling of a circuit breaker with no failure conditions.
	t.Run("without conditions", func(t *testing.T) {
		// Given
		cb1 := circuitbreaker.NewBuilder[any]().WithDelay(0).Build()

		//	When / Then
		testutil.Test[any](t).
			With(cb1).
			Run(testutil.RunFn(testutil.ErrInvalidArgument)).
			AssertFailure(1, 1, testutil.ErrInvalidArgument, func() {
				assert.True(t, cb1.IsOpen())
			})

		// Given
		cb2 := circuitbreaker.NewBuilder[bool]().WithDelay(0).Build()
		var counter int
		retryPolicy := retrypolicy.NewWithDefaults[bool]()
		before := func() {
			counter = 0
		}

		// When / Then
		testutil.Test[bool](t).
			With(retryPolicy).
			Before(before).
			Get(func(execution failsafe.Execution[bool]) (bool, error) {
				counter++
				if counter < 3 {
					return false, testutil.ErrInvalidArgument
				}
				return true, nil
			}).
			AssertSuccess(3, 3, true, func() {
				assert.True(t, cb2.IsClosed())
			})
	})

	t.Run("should return ErrOpen after failures exceeded", func(t *testing.T) {
		// Given
		cb := circuitbreaker.NewBuilder[bool]().
			WithFailureThreshold(2).
			HandleResult(false).
			WithDelay(10 * time.Second).
			Build()

		// When
		failsafe.Get(testutil.GetFalseFn, cb)
		failsafe.Get(testutil.GetFalseFn, cb)

		// Then
		testutil.Test[bool](t).
			With(cb).
			Get(testutil.GetFn(true, nil)).
			AssertFailure(1, 0, circuitbreaker.ErrOpen, func() {
				assert.True(t, cb.IsOpen())
			})
	})

	// Tests a scenario where CircuitBreaker rejects some retried executions, which prevents the user's Supplier from being called.
	t.Run("with rejected retries", func(t *testing.T) {
		// Given
		rpStats := &policytesting.Stats{}
		rp := policytesting.WithRetryStats(retrypolicy.NewBuilder[any]().WithMaxAttempts(7), rpStats).Build()
		cb := circuitbreaker.NewBuilder[any]().WithFailureThreshold(3).Build()
		before := func() {
			rpStats.Reset()
			policytesting.Reset(cb)
		}

		// When / Then
		testutil.Test[any](t).
			With(rp, cb).
			Before(before).
			Run(testutil.RunFn(testutil.ErrInvalidArgument)).
			AssertFailure(7, 3, circuitbreaker.ErrOpen, func() {
				assert.Equal(t, 7, rpStats.Executions())
				assert.Equal(t, 6, rpStats.Retries())
				assert.Equal(t, uint(3), cb.Metrics().Executions())
				assert.Equal(t, uint(3), cb.Metrics().Failures())
			})
	})

	// Tests circuit breaker time based failure thresholding state transitions.
	t.Run("should support time based failure threshold", func(t *testing.T) {
		// Given
		cb := circuitbreaker.NewBuilder[bool]().
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
	})

	// Tests circuit breaker time based failure rate thresholding state transitions.
	t.Run("should support time based failure rate thresholding", func(t *testing.T) {
		// Given
		cb := circuitbreaker.NewBuilder[bool]().
			WithFailureRateThreshold(.5, 3, 200*time.Millisecond).
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
	})

	t.Run("support call OnOpen listener", func(t *testing.T) {
		t.Run("without context", func(t *testing.T) {
			// Given
			var called bool
			cb := circuitbreaker.NewBuilder[bool]().
				WithFailureThresholdRatio(1, 2).
				OnOpen(func(e circuitbreaker.StateChangedEvent) {
					called = true
					assert.Equal(t, uint(1), e.Metrics().Failures())
					assert.Equal(t, uint(1), e.Metrics().Successes())
					assert.Equal(t, context.Background(), e.Context())
				}).
				Build()

			// When / Then
			cb.RecordSuccess()
			cb.RecordFailure()
			assert.True(t, called)
		})

		t.Run("with context", func(t *testing.T) {
			// Given
			var called bool
			ctx, _ := context.WithCancel(context.Background())
			cb := circuitbreaker.NewBuilder[any]().
				WithFailureThreshold(1).
				OnOpen(func(e circuitbreaker.StateChangedEvent) {
					called = true
					assert.Equal(t, ctx, e.Context())
				}).
				Build()

			// When / Then
			_, _ = failsafe.NewExecutor[any](cb).
				WithContext(ctx).
				GetWithExecution(testutil.GetFn[any](nil, testutil.ErrInvalidArgument))
			assert.True(t, called)
		})
	})

	t.Run("should call OnClose listener", func(t *testing.T) {
		// Given
		var called bool
		cb := circuitbreaker.NewBuilder[bool]().
			WithSuccessThresholdRatio(3, 5).
			OnClose(func(e circuitbreaker.StateChangedEvent) {
				called = true
				assert.Equal(t, uint(2), e.Metrics().Failures())
				assert.Equal(t, uint(3), e.Metrics().Successes())
			}).
			Build()

		// When
		cb.HalfOpen()

		cb.RecordFailure()
		cb.RecordFailure()
		cb.RecordSuccess()
		cb.RecordSuccess()
		cb.RecordSuccess()

		// Then
		assert.True(t, called)
	})
}
