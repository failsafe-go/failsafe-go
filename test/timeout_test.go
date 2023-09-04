package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
	rptesting "failsafe/internal/retrypolicy_testutil"

	"failsafe/internal/testutil"
	timouttesting "failsafe/internal/timeout_testutil"

	"failsafe/retrypolicy"
	"failsafe/timeout"
)

// Tests a simple execution that does not timeout.
func TestShouldNotTimeout(t *testing.T) {
	// Given
	timeout := timeout.With[any](time.Second)

	// When / Then
	testutil.TestGetSuccess(t, failsafe.With[any](timeout),
		func(execution failsafe.Execution[any]) (any, error) {
			return "success", nil
		},
		1, 1, "success")
}

// Tests that an inner timeout does not prevent outer retries from being performed when the inner func is blocked.
func TestRetryTimeoutWithBlockedFunc(t *testing.T) {
	timeoutStats := &testutil.Stats{}
	rpStats := &testutil.Stats{}
	timeout := timouttesting.WithTimeoutStatsAndLogs(timeout.Builder[any](50*time.Millisecond), timeoutStats).Build()
	rp := rptesting.WithRetryStatsAndLogs(retrypolicy.Builder[any](), rpStats).Build()

	testutil.TestGetSuccess(t, failsafe.With[any](rp, timeout),
		func(exec failsafe.Execution[any]) (any, error) {
			if exec.Attempts <= 2 {
				// Block, trigginer the timeout
				time.Sleep(100 * time.Millisecond)
			}
			return false, nil
		}, 3, 3, false)
	assert.Equal(t, 3, timeoutStats.ExecutionCount)
	assert.Equal(t, 2, timeoutStats.FailureCount)
	assert.Equal(t, 1, timeoutStats.SuccessCount)
	assert.Equal(t, 2, rpStats.RetryCount)
	assert.Equal(t, 1, rpStats.SuccessCount)
}

// Tests that when an outer retry is scheduled any inner timeouts are cancelled. This prevents the timeout from accidentally cancelling a
// scheduled retry that may be pending.
func TestRetryTimeoutWithPendingRetry(t *testing.T) {
	timeoutStats := &testutil.Stats{}
	rpStats := &testutil.Stats{}
	timeout := timouttesting.WithTimeoutStatsAndLogs(timeout.Builder[any](50*time.Millisecond), timeoutStats).Build()
	rp := rptesting.WithRetryStatsAndLogs(retrypolicy.Builder[any]().WithDelay(100*time.Millisecond), rpStats).Build()

	testutil.TestGetFailure(t, failsafe.With[any](rp, timeout),
		func(exec failsafe.Execution[any]) (any, error) {
			return nil, testutil.InvalidArgumentError{}
		}, 3, 3, testutil.InvalidArgumentError{})
	assert.Equal(t, 3, timeoutStats.ExecutionCount)
	assert.Equal(t, 0, timeoutStats.FailureCount)
	assert.Equal(t, 3, timeoutStats.SuccessCount)
	assert.Equal(t, 2, rpStats.RetryCount)
	assert.Equal(t, 1, rpStats.FailureCount)
}
