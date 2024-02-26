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
	testutil.TestRunSuccess(t, policytesting.SetupFn(stats), failsafe.NewExecutor[any](hp),
		func(exec failsafe.Execution[any]) error {
			return nil
		},
		1, 1, func() {
			assert.Equal(t, 0, stats.Hedges())
		})
}

// Tests a simple execution that hedges after a delay. Should return a result from the initial execution, but not until hedges are started.
func TestShouldHedgeWhenDelayExceeded(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[bool](10*time.Millisecond).WithMaxHedges(2), stats).Build()

	// When / Then
	testutil.TestGetSuccess(t, policytesting.SetupFn(stats), failsafe.NewExecutor[bool](hp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			if exec.Attempts() == 1 {
				time.Sleep(100 * time.Millisecond)
				return true, nil
			}
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return false, testutil.ErrInvalidState
		},
		3, -1, true, func() {
			assert.Equal(t, 2, stats.Hedges())
		})
}

func TestAllHedgesUsed(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[int](10*time.Millisecond).WithMaxHedges(2), stats).Build()

	// When / Then
	testutil.TestGetSuccess(t, policytesting.SetupFn(stats), failsafe.NewExecutor[int](hp),
		func(exec failsafe.Execution[int]) (int, error) {
			attempt := exec.Attempts()
			assert.Equal(t, attempt > 1, exec.IsHedge())
			time.Sleep(100 * time.Millisecond)
			return attempt, nil
		},
		3, 1, 1, func() {
			assert.Equal(t, 2, stats.Hedges())
		})
}

func TestCancelOnResult(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[bool](10*time.Millisecond).
		WithMaxHedges(4).
		CancelOnResult(true), stats).
		Build()

	// When / Then
	t.Run("first returned result triggers cancellation", func(t *testing.T) {
		testutil.TestGetSuccess(t, policytesting.SetupFn(stats), failsafe.NewExecutor[bool](hp),
			func(exec failsafe.Execution[bool]) (bool, error) {
				attempt := exec.Attempts()
				if attempt == 3 {
					return true, nil
				}
				testutil.WaitAndAssertCanceled(t, time.Second, exec)
				return false, nil
			},
			3, -1, true, func() {
				assert.Equal(t, 2, stats.Hedges())
			})
	})

	// When / Then
	t.Run("third returned result triggers cancellation", func(t *testing.T) {
		testutil.TestGetSuccess(t, policytesting.SetupFn(stats), failsafe.NewExecutor[bool](hp),
			func(exec failsafe.Execution[bool]) (bool, error) {
				attempt := exec.Attempts()

				// First two results do not cause hedges to be cancelled
				if attempt < 3 {
					time.Sleep(100 * time.Millisecond)
					return false, nil
				}

				// Third result causes hedges to be cancelled
				if attempt != 3 {
					time.Sleep(100 * time.Millisecond)
					return true, nil
				}

				// Remaining hedges are cancelled
				testutil.WaitAndAssertCanceled(t, time.Second, exec)
				return false, nil
			},
			5, -1, true, func() {
				assert.Equal(t, 4, stats.Hedges())
			})
	})
}
