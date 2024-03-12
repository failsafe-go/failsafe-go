package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

// RetryPolicy -> CircuitBreaker
func TestRetryPolicyCircuitBreaker(t *testing.T) {
	rp := retrypolicy.Builder[bool]().WithMaxRetries(-1).Build()
	cb := circuitbreaker.Builder[bool]().
		WithFailureThreshold(3).
		WithDelay(10 * time.Minute).
		Build()
	stub, reset := testutil.ErrorNTimesThenReturn[bool](testutil.ErrConnecting, 2, true)
	setup := func() context.Context {
		reset()
		policytesting.ResetCircuitBreaker(cb)
		return nil
	}

	testutil.TestGetSuccess(t, setup, failsafe.NewExecutor[bool](rp, cb), stub,
		3, 3, true)
	assert.Equal(t, uint(1), cb.Metrics().Successes())
	assert.Equal(t, uint(2), cb.Metrics().Failures())
}

// RetryPolicy -> CircuitBreaker
//
// Tests RetryPolicy with a CircuitBreaker that is open.
func TestRetryPolicyCircuitBreakerOpen(t *testing.T) {
	rp := policytesting.WithRetryLogs(retrypolicy.Builder[any]()).Build()
	cb := policytesting.WithBreakerLogs(circuitbreaker.Builder[any]()).Build()
	setup := func() context.Context {
		policytesting.ResetCircuitBreaker(cb)
		return nil
	}

	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](rp, cb),
		func(execution failsafe.Execution[any]) error {
			return errors.New("test")
		}, 3, 1, circuitbreaker.ErrOpen)
}

// CircuitBreaker -> RetryPolicy
func TestCircuitBreakerRetryPolicy(t *testing.T) {
	rp := retrypolicy.WithDefaults[any]()
	cb := circuitbreaker.Builder[any]().WithFailureThreshold(3).Build()
	setup := func() context.Context {
		policytesting.ResetCircuitBreaker(cb)
		return nil
	}

	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](cb, rp),
		func(execution failsafe.Execution[any]) error {
			return testutil.ErrInvalidState
		}, 3, 3, testutil.ErrInvalidState)
	assert.Equal(t, uint(0), cb.Metrics().Successes())
	assert.Equal(t, uint(1), cb.Metrics().Failures())
	assert.True(t, cb.IsClosed())
}

// Fallback -> RetryPolicy
func TestFallbackRetryPolicy(t *testing.T) {
	// Given
	fb := fallback.BuilderWithResult(true).HandleErrors(retrypolicy.ErrExceeded).Build()
	rp := retrypolicy.WithDefaults[bool]()

	// When / Then
	testutil.TestGetSuccess[bool](t, nil, failsafe.NewExecutor[bool](fb, rp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrInvalidArgument
		},
		3, 3, true)

	// Given
	fb = fallback.WithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
		assert.False(t, exec.LastResult())
		assert.ErrorIs(t, exec.LastError(), testutil.ErrInvalidState)
		return true, nil
	})

	// When / Then
	testutil.TestGetSuccess[bool](t, nil, failsafe.NewExecutor[bool](fb, rp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrInvalidState
		},
		3, 3, true)
}

// Fallback -> HedgePolicy
func TestFallbackHedgePolicy(t *testing.T) {
	// Given
	fb := fallback.WithResult(true)
	hp := hedgepolicy.WithDelay[bool](10 * time.Millisecond)

	// When / Then
	testutil.TestGetSuccess[bool](t, nil, failsafe.NewExecutor[bool](fb, hp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			time.Sleep(50 * time.Millisecond)
			return false, testutil.ErrInvalidArgument
		},
		2, -1, true)
}

// RetryPolicy -> Fallback
func TestRetryPolicyFallback(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[string]()
	fb := fallback.WithResult("test")

	// When / Then
	testutil.TestGetSuccess[string](t, nil, failsafe.NewExecutor[string](rp, fb),
		func(execution failsafe.Execution[string]) (string, error) {
			return "", testutil.ErrInvalidState
		},
		1, 1, "test")
}

// Fallback -> CircuitBreaker
//
// Tests fallback with a circuit breaker that is closed.
func TestFallbackCircuitBreaker(t *testing.T) {
	// Given
	fb := fallback.WithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
		assert.False(t, exec.LastResult())
		assert.ErrorIs(t, testutil.ErrInvalidState, exec.LastError())
		return true, nil
	})
	cb := circuitbreaker.Builder[bool]().WithSuccessThreshold(3).Build()
	setup := func() context.Context {
		policytesting.ResetCircuitBreaker(cb)
		return nil
	}

	// When / Then
	testutil.TestGetSuccess(t, setup, failsafe.NewExecutor[bool](fb, cb),
		testutil.GetWithExecutionFn[bool](false, testutil.ErrInvalidState),
		1, 1, true)
}

// Fallback -> CircuitBreaker
//
// Tests fallback with a circuit breaker that is open.
func TestFallbackCircuitBreakerOpen(t *testing.T) {
	// Given
	fb := fallback.WithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
		assert.False(t, exec.LastResult())
		assert.ErrorIs(t, circuitbreaker.ErrOpen, exec.LastError())
		return false, nil
	})
	cb := circuitbreaker.Builder[bool]().WithSuccessThreshold(3).Build()

	// When / Then
	cb.Open()
	testutil.TestGetSuccess(t, nil, failsafe.NewExecutor[bool](fb, cb),
		testutil.GetWithExecutionFn[bool](true, nil),
		1, 0, false)
}

// RetryPolicy -> RateLimiter
func TestRetryPolicyRateLimiter(t *testing.T) {
	// Given
	rpStats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.Builder[any](), rpStats).WithMaxAttempts(7).Build()
	rl := ratelimiter.BurstyBuilder[any](3, 1*time.Second).Build()
	setup := func() context.Context {
		rpStats.Reset()
		policytesting.ResetRateLimiter(rl)
		return nil
	}

	// When / Then
	testutil.TestGetFailure(t, setup, failsafe.NewExecutor[any](rp, rl),
		testutil.GetWithExecutionFn[any](nil, testutil.ErrInvalidState),
		7, 3, ratelimiter.ErrExceeded, func() {
			assert.Equal(t, 7, rpStats.Executions())
			assert.Equal(t, 6, rpStats.Retries())
		})
}

// Fallback -> RetryPolicy -> CircuitBreaker
func TestFallbackRetryPolicyCircuitBreaker(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[string]()
	cb := circuitbreaker.Builder[string]().WithFailureThreshold(5).Build()
	fb := fallback.WithResult("test")
	setup := func() context.Context {
		policytesting.ResetCircuitBreaker(cb)
		return nil
	}

	// When / Then
	testutil.TestGetSuccess(t, setup, failsafe.NewExecutor[string](fb, rp, cb),
		testutil.GetWithExecutionFn[string]("", testutil.ErrInvalidState),
		3, 3, "test")
	assert.Equal(t, uint(0), cb.Metrics().Successes())
	assert.Equal(t, uint(3), cb.Metrics().Failures())
	assert.True(t, cb.IsClosed())
}

// RetryPolicy -> Timeout
//
// Tests 2 timeouts, then a success, and asserts the execution is cancelled after each timeout.
func TestRetryPolicyTimeout(t *testing.T) {
	// Given
	rp := retrypolicy.Builder[any]().OnFailure(func(e failsafe.ExecutionEvent[any]) {
		assert.ErrorIs(t, e.LastError(), timeout.ErrExceeded)
	}).Build()
	toStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](50*time.Millisecond), toStats).Build()

	// When / Then
	testutil.TestGetSuccess(t, policytesting.SetupFn(toStats), failsafe.NewExecutor[any](rp, to),
		func(e failsafe.Execution[any]) (any, error) {
			if e.Attempts() <= 2 {
				time.Sleep(100 * time.Millisecond)
				assert.True(t, e.IsCanceled())
			} else {
				assert.False(t, e.IsCanceled())
			}
			return "success", nil
		}, 3, 3, "success", func() {
			assert.Equal(t, 2, toStats.Executions())
		})
}

// RetryPolicy -> HedgePolicy
func TestRetryPolicyHedgePolicy(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any](), stats).Build()
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](10*time.Millisecond), stats).Build()

	// When / Then
	testutil.TestGetFailure(t, policytesting.SetupFn(stats), failsafe.NewExecutor[any](rp, hp),
		func(e failsafe.Execution[any]) (any, error) {
			time.Sleep(20 * time.Millisecond)
			return nil, testutil.ErrInvalidState
		}, 6, -1, testutil.ErrInvalidState, func() {
			assert.Equal(t, 2, stats.Retries())
			assert.Equal(t, 3, stats.Hedges())
		})
}

// CircuitBreaker -> Timeout
func TestCircuitBreakerTimeout(t *testing.T) {
	// Given
	to := timeout.With[string](50 * time.Millisecond)
	cb := circuitbreaker.WithDefaults[string]()
	assert.True(t, cb.IsClosed())
	setup := func() context.Context {
		policytesting.ResetCircuitBreaker(cb)
		return nil
	}

	// When / Then
	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[string](cb, to),
		func(execution failsafe.Execution[string]) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		}, 1, 1, timeout.ErrExceeded)
	assert.True(t, cb.IsOpen())
}

// Fallback -> Timeout
func TestFallbackTimeout(t *testing.T) {
	// Given
	to := timeout.With[bool](10 * time.Millisecond)
	fb := fallback.WithFunc(func(e failsafe.Execution[bool]) (bool, error) {
		assert.ErrorIs(t, e.LastError(), timeout.ErrExceeded)
		return true, nil
	})

	// When / Then
	testutil.TestGetSuccess(t, nil, failsafe.NewExecutor[bool](fb, to),
		func(execution failsafe.Execution[bool]) (bool, error) {
			time.Sleep(100 * time.Millisecond)
			return false, nil
		},
		1, 1, true)
}

// RetryPolicy -> Bulkhead
func TestRetryPolicyBulkhead(t *testing.T) {
	rp := retrypolicy.Builder[any]().WithMaxAttempts(7).Build()
	bh := bulkhead.With[any](2)
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](rp, bh),
		func(execution failsafe.Execution[any]) error {
			return errors.New("test")
		}, 7, 0, bulkhead.ErrFull)
}

// HedgePolicy -> Timeout
//
// Hedge should be triggered twice since the timeouts are longer than the hedge delay.
// Timeout should be triggered 3 times since the results from the hedges are not cancellable.
func TestHedgePolicyTimeout(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](10*time.Millisecond).
		CancelIf(func(a any, err error) bool {
			return err == nil
		}).
		WithMaxHedges(2), stats).
		Build()
	toStats := &policytesting.Stats{}
	to := policytesting.WithTimeoutStatsAndLogs(timeout.Builder[any](100*time.Millisecond), toStats).Build()
	setup := func() context.Context {
		stats.Reset()
		toStats.Reset()
		return nil
	}

	// When / Then
	testutil.TestRunFailure(t, setup, failsafe.NewExecutor[any](hp, to),
		func(e failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, e)
			return errors.New("not cancellable")
		}, 3, -1, timeout.ErrExceeded, func() {
			assert.Equal(t, 2, stats.Hedges())
			assert.Equal(t, 3, toStats.Executions())
		})
}
