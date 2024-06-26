package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
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
	testutil.Test[any](t).
		With(timeout).
		Get(testutil.GetFn[any]("success", nil)).
		AssertSuccess(1, 1, "success")
}

// Tests that an inner timeout does not prevent outer retries from being performed when the inner func is blocked.
func TestRetryTimeoutWithBlockedFunc(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	rpStats := &policytesting.Stats{}
	timeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](50*time.Millisecond), timeoutStats).Build()
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any](), rpStats).Build()

	// When / Then
	testutil.Test[any](t).
		With(rp, timeout).
		Reset(timeoutStats, rpStats).
		Get(func(exec failsafe.Execution[any]) (any, error) {
			if exec.Attempts() <= 2 {
				// Block, triggering the timeout
				time.Sleep(100 * time.Millisecond)
			}
			return false, nil
		}).
		AssertSuccess(3, 3, false, func() {
			assert.Equal(t, 2, timeoutStats.Executions())
			assert.Equal(t, 2, rpStats.Retries())
			assert.Equal(t, 1, rpStats.Successes())
		})
}

// Tests that when an outer retry is scheduled any inner timeouts are cancelled. This prevents the timeout from accidentally cancelling a
// scheduled retry that may be pending.
func TestRetryTimeoutWithPendingRetry(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	rpStats := &policytesting.Stats{}
	timeout := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](50*time.Millisecond), timeoutStats).Build()
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any]().WithDelay(100*time.Millisecond), rpStats).Build()

	// When / Then
	testutil.Test[any](t).
		With(rp, timeout).
		Reset(timeoutStats, rpStats).
		Get(testutil.GetFn[any](nil, testutil.ErrInvalidArgument)).
		AssertFailure(3, 3, testutil.ErrInvalidArgument, func() {
			assert.Equal(t, 0, timeoutStats.Executions())
			assert.Equal(t, 2, rpStats.Retries())
			assert.Equal(t, 3, rpStats.Failures())
		})
}

// Tests that an outer timeout will cancel inner retries when the inner func is blocked. The flow should be:
//   - Execution that retries a few times, blocking each time
//   - Timeout
func TestTimeoutRetryWithBlockedFunc(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](150*time.Millisecond), timeoutStats).Build()
	rp := retrypolicy.WithDefaults[any]()

	// When / Then
	testutil.Test[any](t).
		With(to, rp).
		Reset(timeoutStats).
		Run(func(_ failsafe.Execution[any]) error {
			time.Sleep(60 * time.Millisecond)
			return testutil.ErrInvalidArgument
		}).
		AssertFailure(3, 3, timeout.ErrExceeded, func() {
			assert.Equal(t, 1, timeoutStats.Executions())
		})
}

// Tests that an outer timeout will cancel inner retries when an inner retry is pending. The flow should be:
//   - Execution
//   - Retry sleep/scheduled that blocks
//   - Timeout
func TestTimeoutRetryWithPendingRetry(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	rpStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](100*time.Millisecond), timeoutStats).Build()
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any]().WithDelay(time.Second), rpStats).Build()

	// When / Then
	testutil.Test[any](t).
		With(to, rp).
		Reset(timeoutStats, rpStats).
		Run(testutil.RunFn(testutil.ErrInvalidArgument)).
		AssertFailure(1, 1, timeout.ErrExceeded, func() {
			assert.Equal(t, 1, timeoutStats.Executions())
			assert.Equal(t, 1, rpStats.Executions())
		})
}

// Tests that an outer timeout will cancel inner hedge when the inner func is blocked. The flow should be:
//   - Execution a hedge
//   - Timeout
func TestTimeoutHedgeWithBlockedFunc(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](100*time.Millisecond), stats).Build()
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](10*time.Millisecond), stats).WithMaxHedges(2).Build()

	// When / Then
	testutil.Test[any](t).
		With(to, hp).
		Reset(stats).
		Run(func(_ failsafe.Execution[any]) error {
			time.Sleep(time.Second)
			return testutil.ErrInvalidArgument
		}).
		AssertFailure(3, -1, timeout.ErrExceeded, func() {
			assert.Equal(t, 1, stats.Executions())
			assert.Equal(t, 2, stats.Hedges())
		})
}

// Tests an inner timeout that fires while the func is blocked.
func TestFallbackTimeoutWithBlockedFunc(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](10*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs(fallback.BuilderWithError[any](testutil.ErrInvalidArgument), fbStats).Build()

	// When / Then
	testutil.Test[any](t).
		With(fb, to).
		Reset(timeoutStats, fbStats).
		Run(func(_ failsafe.Execution[any]) error {
			time.Sleep(100 * time.Millisecond)
			return errors.New("test")
		}).
		AssertFailure(1, 1, testutil.ErrInvalidArgument, func() {
			assert.Equal(t, 1, timeoutStats.Executions())
			assert.Equal(t, 1, fbStats.Executions())
		})
}

// Tests that an inner timeout will not interrupt an outer fallback. The inner timeout is never triggered since the func
// completes immediately.
func TestFallbackWithInnerTimeout(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](10*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs(fallback.BuilderWithError[any](testutil.ErrInvalidArgument), fbStats).Build()

	// When / Then
	testutil.Test[any](t).
		With(fb, to).
		Reset(timeoutStats, fbStats).
		Run(testutil.RunFn(errors.New("test"))).
		AssertFailure(1, 1, testutil.ErrInvalidArgument, func() {
			assert.Equal(t, 0, timeoutStats.Executions())
			assert.Equal(t, 1, fbStats.Executions())
		})
}

// Tests that an outer timeout will interrupt an inner func that is blocked, skipping the inner fallback.
func TestTimeoutFallbackWithBlockedFunc(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](10*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs(fallback.BuilderWithError[any](testutil.ErrInvalidArgument), fbStats).Build()

	// When / Then
	testutil.Test[any](t).
		With(to, fb).
		Reset(timeoutStats, fbStats).
		Run(func(_ failsafe.Execution[any]) error {
			time.Sleep(100 * time.Millisecond)
			return errors.New("test")
		}).
		AssertFailure(1, 1, timeout.ErrExceeded, func() {
			assert.Equal(t, 1, timeoutStats.Executions())
			assert.Equal(t, 0, fbStats.Executions())
		})
}

// Tests that an outer timeout will interrupt an inner fallback that is blocked.
func TestTimeoutFallbackWithBlockedFallback(t *testing.T) {
	// Given
	timeoutStats := &policytesting.Stats{}
	fbStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](100*time.Millisecond), timeoutStats).Build()
	fb := policytesting.WithFallbackStatsAndLogs(fallback.BuilderWithFunc[any](func(_ failsafe.Execution[any]) (any, error) {
		time.Sleep(200 * time.Millisecond)
		return nil, testutil.ErrInvalidState
	}), fbStats).Build()

	// When / Then
	testutil.Test[any](t).
		With(to, fb).
		Reset(timeoutStats, fbStats).
		Run(testutil.RunFn(errors.New("test"))).
		AssertFailure(1, 1, timeout.ErrExceeded, func() {
			assert.Equal(t, 1, timeoutStats.Executions())
			assert.Equal(t, 0, fbStats.Executions())
		})
}
