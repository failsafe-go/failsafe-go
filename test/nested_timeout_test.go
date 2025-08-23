package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func TestTimeout_Nested(t *testing.T) {
	t.Run("Timeout -> Timeout", func(t *testing.T) {
		innerTimeoutStats := &policytesting.Stats{}
		outerTimeoutStats := &policytesting.Stats{}
		innerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](100*time.Millisecond), innerTimeoutStats).Build()
		outerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](500*time.Millisecond), outerTimeoutStats).Build()

		testutil.Test[any](t).
			With(outerTimeout, innerTimeout).
			Reset(innerTimeoutStats, outerTimeoutStats).
			Run(func(exec failsafe.Execution[any]) error {
				testutil.WaitAndAssertCanceled(t, 150*time.Millisecond, exec)
				return nil
			}).
			AssertFailure(1, 1, timeout.ErrExceeded, func() {
				assert.Equal(t, 1, innerTimeoutStats.Executions())
			})
	})

	// Tests a scenario where an inner timeout is exceeded, triggering retries, then eventually the outer timeout is exceeded.
	t.Run("RetryPolicy -> Timeout", func(t *testing.T) {
		innerTimeoutStats := &policytesting.Stats{}
		retryStats := &policytesting.Stats{}
		outerTimeoutStats := &policytesting.Stats{}
		innerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](100*time.Millisecond), innerTimeoutStats).Build()
		retryPolicy := policytesting.WithRetryStatsAndLogs(retrypolicy.NewBuilder[any]().WithMaxRetries(10), retryStats).Build()
		outerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](500*time.Millisecond), outerTimeoutStats).Build()

		testutil.Test[any](t).
			With(outerTimeout, retryPolicy, innerTimeout).
			Reset(innerTimeoutStats, outerTimeoutStats).
			Run(func(exec failsafe.Execution[any]) error {
				testutil.WaitAndAssertCanceled(t, 150*time.Millisecond, exec)
				return nil
			}).
			AssertFailure(-1, -1, timeout.ErrExceeded, func() {
				assert.True(t, innerTimeoutStats.Executions() >= 3)
				assert.True(t, retryStats.Executions() >= 3)
			})
	})

	// Tests a scenario with a fallback, retry policy, and two timeouts, where the outer timeout triggers first.
	t.Run("Fallback -> RetryPolicy -> Timeout -> Timeout", func(t *testing.T) {
		innerTimeoutStats := &policytesting.Stats{}
		outerTimeoutStats := &policytesting.Stats{}
		innerTimeout := policytesting.WithTimeoutStatsAndLogs[bool](timeout.NewBuilder[bool](100*time.Millisecond), innerTimeoutStats).Build()
		outerTimeout := policytesting.WithTimeoutStatsAndLogs[bool](timeout.NewBuilder[bool](50*time.Millisecond), outerTimeoutStats).Build()
		rp := retrypolicy.NewWithDefaults[bool]()
		fb := fallback.NewWithResult(true)

		testutil.Test[bool](t).
			With(fb, rp, outerTimeout, innerTimeout).
			Reset(innerTimeoutStats, outerTimeoutStats).
			Get(func(exec failsafe.Execution[bool]) (bool, error) {
				testutil.WaitAndAssertCanceled(t, 150*time.Millisecond, exec)
				return false, nil
			}).
			AssertSuccess(3, 3, true, func() {
				assert.Equal(t, 0, innerTimeoutStats.Executions())
				assert.Equal(t, 3, outerTimeoutStats.Executions())
			})
	})

	// RetryPolicy -> Timeout -> Timeout
	//
	// Tests a scenario where three consecutive timeouts should cause the execution to be canceled for all policies.
	t.Run("cancel nested timeouts", func(t *testing.T) {
		retryStats := &policytesting.Stats{}
		innerTimeoutStats := &policytesting.Stats{}
		outerTimeoutStats := &policytesting.Stats{}
		rp := policytesting.WithRetryStatsAndLogs(retrypolicy.NewBuilder[any](), retryStats).Build()
		innerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](time.Second), innerTimeoutStats).Build()
		outerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](200*time.Millisecond), outerTimeoutStats).Build()

		testutil.Test[any](t).
			With(rp, outerTimeout, innerTimeout).
			Reset(retryStats, innerTimeoutStats, outerTimeoutStats).
			Run(func(exec failsafe.Execution[any]) error {
				testutil.WaitAndAssertCanceled(t, time.Second, exec)
				return nil
			}).
			AssertFailure(3, 3, timeout.ErrExceeded, func() {
				assert.Equal(t, 3, retryStats.Executions())
				assert.Equal(t, 0, innerTimeoutStats.Executions())
				assert.Equal(t, 3, outerTimeoutStats.Executions())
			})
	})
}
