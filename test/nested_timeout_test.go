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
	innerTimeout := policytesting.WithTimeoutStatsAndLogs[any](timeout.Builder[any](100*time.Millisecond), innerTimeoutStats).Build()
	retryPolicy := policytesting.WithRetryStatsAndLogs[any](retrypolicy.Builder[any]().WithMaxRetries(10), retryStats).Build()
	outerTimeout := policytesting.WithTimeoutStatsAndLogs[any](timeout.Builder[any](500*time.Millisecond), outerTimeoutStats).Build()

	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](outerTimeout, retryPolicy, innerTimeout),
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, 150*time.Millisecond, exec)
			return nil
		},
		-1, -1, timeout.ErrTimeoutExceeded)
	assert.True(t, innerTimeoutStats.ExecutionCount() >= 3)
	assert.True(t, retryStats.ExecutionCount() >= 3)
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
	setup := func() context.Context {
		innerTimeoutStats.Reset()
		outerTimeoutStats.Reset()
		return nil
	}

	testutil.TestGetSuccess(t, setup, failsafe.NewExecutor[bool](fb, rp, outerTimeout, innerTimeout),
		func(exec failsafe.Execution[bool]) (bool, error) {
			testutil.WaitAndAssertCanceled(t, 150*time.Millisecond, exec)
			return false, nil
		},
		3, 3, true)
	assert.Equal(t, 0, innerTimeoutStats.ExecutionCount())
	assert.Equal(t, 3, outerTimeoutStats.ExecutionCount())
}

// RetryPolicy -> Timeout -> Timeout
//
// Tests a scenario where three consecutive timeouts should cause the execution to be canceled for all policies.
func TestCancelNestedTimeouts(t *testing.T) {
	retryStats := &policytesting.Stats{}
	innerTimeoutStats := &policytesting.Stats{}
	outerTimeoutStats := &policytesting.Stats{}
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any](), retryStats).Build()
	innerTimeout := policytesting.WithTimeoutStatsAndLogs[any](timeout.Builder[any](time.Second), innerTimeoutStats).Build()
	outerTimeout := policytesting.WithTimeoutStatsAndLogs[any](timeout.Builder[any](200*time.Millisecond), outerTimeoutStats).Build()
	setup := func() context.Context {
		retryStats.Reset()
		innerTimeoutStats.Reset()
		outerTimeoutStats.Reset()
		return nil
	}

	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](rp, outerTimeout, innerTimeout),
		func(exec failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, exec)
			return nil
		},
		3, 3, timeout.ErrTimeoutExceeded)
	assert.Equal(t, 3, retryStats.ExecutionCount())
	assert.Equal(t, 0, innerTimeoutStats.ExecutionCount())
	assert.Equal(t, 3, outerTimeoutStats.ExecutionCount())
}
