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

func TestShouldNotHedgeWhenDelayNotExceeded(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](time.Second), stats).Build()

	// When / Then
	testutil.Test[any](t).
		With(hp).
		Reset(stats).
		Run(testutil.RunFn(nil)).
		AssertSuccess(1, 1, nil, func() {
			assert.Equal(t, 0, stats.Hedges())
		})
}

// Tests a simple execution that hedges after a delay. Should return a result from the initial execution, but not until hedges are started.
func TestShouldHedgeWhenDelayExceeded(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[bool](10*time.Millisecond).WithMaxHedges(2), stats).Build()

	// When / Then
	testutil.Test[bool](t).
		With(hp).
		Reset(stats).
		Get(func(exec failsafe.Execution[bool]) (bool, error) {
			if exec.Attempts() == 1 {
				time.Sleep(100 * time.Millisecond)
				return true, nil
			}
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return false, testutil.ErrInvalidState
		}).
		AssertSuccess(3, -1, true, func() {
			assert.Equal(t, 2, stats.Hedges())
		})
}

// Asserts that the expected number of hedges are executed.
func TestAllHedgesUsed(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[int](20*time.Millisecond).WithMaxHedges(2), stats).Build()

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
}

// Asserts that backup executions, which are hedges with 0 delay, are supported.
func TestBackupExecutions(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[int](0).
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
}

// Asserts that a specific cancellable hedge result is returned.
func TestCancelOnResult(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](10*time.Millisecond).
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
			Get(func(exec failsafe.Execution[any]) (any, error) {
				attempts := exec.Attempts()
				if attempts <= 3 {
					// First 3 results return before being canceled
					time.Sleep(100 * time.Millisecond)
				} else {
					// Last 2 results are cancelled before being returned
					testutil.WaitAndAssertCanceled(t, 100*time.Millisecond, exec)
				}
				return attempts, nil
			}).
			AssertSuccess(5, -1, 3, func() {
				assert.Equal(t, 4, stats.Hedges())
			})
	})
}
