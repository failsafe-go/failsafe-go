package test

import (
	"errors"
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
	timeoutStats := &policytesting.Stats{}
	rpStats := &policytesting.Stats{}
	timeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](50*time.Millisecond), timeoutStats).Build()
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any](), rpStats).Build()

	testutil.TestGetSuccess(t, failsafe.With[any](rp, timeout),
		func(exec failsafe.Execution[any]) (any, error) {
			if exec.Attempts <= 2 {
				// Block, trigginer the timeout
				time.Sleep(100 * time.Millisecond)
			}
			return false, nil
		}, 3, 3, false)
	assert.Equal(t, 2, timeoutStats.ExecutionCount)
	assert.Equal(t, 2, rpStats.RetryCount)
	assert.Equal(t, 1, rpStats.SuccessCount)
}

// Tests that when an outer retry is scheduled any inner timeouts are cancelled. This prevents the timeout from accidentally cancelling a
// scheduled retry that may be pending.
func TestRetryTimeoutWithPendingRetry(t *testing.T) {
	timeoutStats := &policytesting.Stats{}
	rpStats := &policytesting.Stats{}
	timeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](50*time.Millisecond), timeoutStats).Build()
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any]().WithDelay(100*time.Millisecond), rpStats).Build()

	testutil.TestGetFailure(t, failsafe.With[any](rp, timeout),
		func(exec failsafe.Execution[any]) (any, error) {
			return nil, testutil.InvalidArgumentError{}
		}, 3, 3, testutil.InvalidArgumentError{})
	assert.Equal(t, 0, timeoutStats.ExecutionCount)
	assert.Equal(t, 2, rpStats.RetryCount)
	assert.Equal(t, 3, rpStats.FailureCount)
}

// Tests that an outer timeout will cancel inner retries when the inner func is blocked. The flow should be:
//   - Execution that retries a few times, blocking each time
//   - Timeout
func TestTimeoutRetryWithBlockedFunc(t *testing.T) {
	timeoutStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](150*time.Millisecond), timeoutStats).Build()
	rp := retrypolicy.WithDefaults[any]()

	testutil.TestRunFailure(t, failsafe.With[any](to, rp),
		func(_ failsafe.Execution[any]) error {
			time.Sleep(60 * time.Millisecond)
			return testutil.InvalidArgumentError{}
		},
		3, 3, timeout.ErrTimeoutExceeded)
	assert.Equal(t, 1, timeoutStats.ExecutionCount)
}

// Tests that an outer timeout will cancel inner retries when an inner retry is pending. The flow should be:
//   - Execution
//   - Retry sleep/scheduled that blocks
//   - Timeout
func TestTimeoutRetryWithPendingRetry(t *testing.T) {
	timeoutStats := &policytesting.Stats{}
	rpStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](100*time.Millisecond), timeoutStats).Build()
	rp := policytesting.WithRetryStatsAndLogs[any](retrypolicy.Builder[any]().WithDelay(time.Second), rpStats).Build()

	testutil.TestRunFailure(t, failsafe.With[any](to).Compose(rp),
		func(_ failsafe.Execution[any]) error {
			return testutil.InvalidArgumentError{}
		},
		1, 1, timeout.ErrTimeoutExceeded)
	assert.Equal(t, 1, timeoutStats.ExecutionCount)
	assert.Equal(t, 1, rpStats.ExecutionCount)
}

// Tests an inner timeout that fires while the func is blocked.
func TestFallbackTimeoutWithBlockedFunc(t *testing.T) {
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](10*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs[any](fallback.BuilderWithError[any](testutil.InvalidArgumentError{}), fbStats).Build()

	testutil.TestRunFailure(t, failsafe.With[any](fb).Compose(to),
		func(_ failsafe.Execution[any]) error {
			time.Sleep(100 * time.Millisecond)
			return errors.New("test")
		},
		1, 1, testutil.InvalidArgumentError{})
	assert.Equal(t, 1, timeoutStats.ExecutionCount)
	assert.Equal(t, 1, fbStats.ExecutionCount)
}

// Tests that an inner timeout will not interrupt an outer fallback. The inner timeout is never triggered since the func completes immediately.
func TestFallbackTimeout(t *testing.T) {
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](10*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs[any](fallback.BuilderWithError[any](testutil.InvalidArgumentError{}), fbStats).Build()

	testutil.TestRunFailure(t, failsafe.With[any](fb).Compose(to),
		func(_ failsafe.Execution[any]) error {
			return errors.New("test")
		},
		1, 1, testutil.InvalidArgumentError{})
	assert.Equal(t, 0, timeoutStats.ExecutionCount)
	assert.Equal(t, 1, fbStats.ExecutionCount)
}

// Tests that an outer timeout will interrupt an inner func that is blocked, skipping the inner fallback.
func TestTimeoutFallbackWithBlockedFunc(t *testing.T) {
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](10*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs[any](fallback.BuilderWithError[any](testutil.InvalidArgumentError{}), fbStats).Build()

	testutil.TestRunFailure(t, failsafe.With[any](to).Compose(fb),
		func(_ failsafe.Execution[any]) error {
			time.Sleep(100 * time.Millisecond)
			return errors.New("test")
		},
		1, 1, timeout.ErrTimeoutExceeded)
	assert.Equal(t, 1, timeoutStats.ExecutionCount)
	assert.Equal(t, 0, fbStats.ExecutionCount)
}

// Tests that an outer timeout will interrupt an inner fallback that is blocked.
func TestTimeoutFallbackWithBlockedFallback(t *testing.T) {
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](100*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs[any](fallback.BuilderWithFn[any](func(_ failsafe.Execution[any]) (any, error) {
		time.Sleep(200 * time.Millisecond)
		return nil, testutil.InvalidStateError{}
	}), fbStats).Build()

	testutil.TestRunFailure(t, failsafe.With[any](to).Compose(fb),
		func(_ failsafe.Execution[any]) error {
			return errors.New("test")
		},
		1, 1, timeout.ErrTimeoutExceeded)
	assert.Equal(t, 1, timeoutStats.ExecutionCount)
	assert.Equal(t, 0, fbStats.ExecutionCount)
}
