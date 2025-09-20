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
