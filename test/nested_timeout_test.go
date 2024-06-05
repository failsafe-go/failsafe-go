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

// Timeout -> RetryPolicy -> Timeout
//
// Tests a scenario where an inner timeout is exceeded, triggering retries, then eventually the outer timeout is exceeded.
func TestTimeoutRetryPolicyTimeout(t *testing.T) {
	innerTimeoutStats := &policytesting.Stats{}
	retryStats := &policytesting.Stats{}
	outerTimeoutStats := &policytesting.Stats{}
	innerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](100*time.Millisecond), innerTimeoutStats).Build()
	retryPolicy := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any]().WithMaxRetries(10), retryStats).Build()
	outerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](500*time.Millisecond), outerTimeoutStats).Build()

	testutil.Test[any](t).
		With(outerTimeout, retryPolicy, innerTimeout).
		Run(func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, 150*time.Millisecond, exec)
			return nil
		}).
		AssertFailure(-1, -1, timeout.ErrExceeded, func() {
			assert.True(t, innerTimeoutStats.Executions() >= 3)
			assert.True(t, retryStats.Executions() >= 3)
		})
}

// Fallback -> RetryPolicy -> Timeout -> Timeout
//
// Tests a scenario with a fallback, retry policy, and two timeouts, where the outer timeout triggers first.
func TestFallbackRetryPolicyTimeoutTimeout(t *testing.T) {
	innerTimeoutStats := &policytesting.Stats{}
	outerTimeoutStats := &policytesting.Stats{}
	innerTimeout := policytesting.WithTimeoutStatsAndLogs[bool](timeout.Builder[bool](100*time.Millisecond), innerTimeoutStats).Build()
	outerTimeout := policytesting.WithTimeoutStatsAndLogs[bool](timeout.Builder[bool](50*time.Millisecond), outerTimeoutStats).Build()
	rp := retrypolicy.WithDefaults[bool]()
	fb := fallback.WithResult(true)

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
}

// RetryPolicy -> Timeout -> Timeout
//
// Tests a scenario where three consecutive timeouts should cause the execution to be canceled for all policies.
func TestCancelNestedTimeouts(t *testing.T) {
	retryStats := &policytesting.Stats{}
	innerTimeoutStats := &policytesting.Stats{}
	outerTimeoutStats := &policytesting.Stats{}
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any](), retryStats).Build()
	innerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](time.Second), innerTimeoutStats).Build()
	outerTimeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](200*time.Millisecond), outerTimeoutStats).Build()

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
}
