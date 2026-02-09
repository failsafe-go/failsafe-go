package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestHedgePolicy(t *testing.T) {
	t.Run("should not hedge when delay not exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[any](time.Second), stats).Build()

		// When / Then
		testutil.Test[any](t).
			With(hp).
			Reset(stats).
			Run(testutil.RunFn(nil)).
			AssertSuccess(1, 1, nil, func() {
				assert.Equal(t, 0, stats.Hedges())
			})
	})

	// Tests a simple execution that hedges after a delay. Should return a result from the initial execution, but not until hedges are started.
	t.Run("should hedge when delay exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[bool](10*time.Millisecond).WithMaxHedges(2), stats).Build()

		// When / Then
		testutil.Test[bool](t).
			With(hp).
			Reset(stats).
			Get(testutil.SlowNTimesThenReturn(t, 1, 100*time.Millisecond, true, false)).
			AssertSuccess(3, -1, true, func() {
				assert.Equal(t, 2, stats.Hedges())
			})
	})

	// Asserts that the expected number of hedges are executed.
	t.Run("all hedges used", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[int](20*time.Millisecond).WithMaxHedges(2), stats).Build()

		// When / Then
		testutil.Test[int](t).
			With(hp).
			Reset(stats).
			Get(func(exec failsafe.Execution[int]) (int, error) {
				attempt := exec.Attempts()
				assert.Equal(t, attempt > 1, exec.IsHedge())
				time.Sleep(100 * time.Millisecond)
				return attempt, nil
			}).
			AssertSuccess(3, 1, 1, func() {
				assert.Equal(t, 2, stats.Hedges())
			})
	})

	// Asserts that backup executions, which are hedges with 0 delay, are supported.
	t.Run("backup executions", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[int](0).
			WithMaxHedges(2).
			CancelOnResult(3), stats).Build()

		// When / Then
		testutil.Test[int](t).
			With(hp).
			Reset(stats).
			Get(func(exec failsafe.Execution[int]) (int, error) {
				return exec.Attempts(), nil
			}).
			AssertSuccess(3, -1, 3, func() {
				assert.Equal(t, 2, stats.Hedges())
			})
	})

	// Asserts that no hedging occurs during the warmup period for quantile-based delay.
	t.Run("should not hedge with quantile during warmup", func(t *testing.T) {
		// Given - a quantile-based hedge policy
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelayQuantile[int](0.95, 30, 20), stats).Build()

		// When - run 20 executions (the warmup period), each should not trigger hedging
		for i := 0; i < 20; i++ {
			_, err := failsafe.With[int](hp).GetWithExecution(func(exec failsafe.Execution[int]) (int, error) {
				time.Sleep(5 * time.Millisecond)
				return 1, nil
			})
			assert.Nil(t, err)
		}

		// Then - no hedges should have occurred
		assert.Equal(t, 0, stats.Hedges())
	})

	// Asserts that quantile-based delay triggers hedging for slow executions after warmup.
	t.Run("should hedge with quantile", func(t *testing.T) {
		// Given - a quantile-based hedge policy
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelayQuantile[int](0.95, 30, 20), stats).Build()

		// Warmup - feed 25 fast executions to establish a baseline
		for i := 0; i < 25; i++ {
			_, err := failsafe.With[int](hp).GetWithExecution(func(exec failsafe.Execution[int]) (int, error) {
				time.Sleep(5 * time.Millisecond)
				return 1, nil
			})
			assert.Nil(t, err)
		}
		hedgesBeforeSlow := stats.Hedges()

		// When - execute a slow request that exceeds the quantile delay
		_, err := failsafe.With[int](hp).GetWithExecution(func(exec failsafe.Execution[int]) (int, error) {
			if exec.IsHedge() {
				return 2, nil
			}
			// Slow execution - should trigger hedging
			time.Sleep(200 * time.Millisecond)
			return 1, nil
		})
		assert.Nil(t, err)
		assert.Equal(t, hedgesBeforeSlow+1, stats.Hedges(), "should have hedged the slow execution")
	})

	// Asserts that a specific cancellable hedge result is returned.
	t.Run("should cancel on result", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[any](10*time.Millisecond).
			WithMaxHedges(4).
			CancelOnResult(true).
			CancelOnResult(3), stats).
			Build()

		// When / Then
		t.Run("first returned result triggers cancellation", func(t *testing.T) {
			testutil.Test[any](t).
				With(hp).
				Reset(stats).
				Get(func(exec failsafe.Execution[any]) (any, error) {
					attempt := exec.Attempts()
					if attempt == 3 {
						return true, nil
					}
					testutil.WaitAndAssertCanceled(t, time.Second, exec)
					return false, nil
				}).
				AssertSuccess(3, -1, true, func() {
					assert.Equal(t, 2, stats.Hedges())
				})
		})

		// When / Then
		t.Run("third returned result triggers cancellation", func(t *testing.T) {
			testutil.Test[any](t).
				With(hp).
				Reset(stats).
				Get(testutil.SlowNTimesThenReturn[any](t, 3, 100*time.Millisecond, 3, 0)).
				AssertSuccess(5, -1, 3, func() {
					assert.Equal(t, 4, stats.Hedges())
				})
		})
	})
}
