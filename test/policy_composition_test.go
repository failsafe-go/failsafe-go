package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/cachepolicy"
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
	// Given
	rp := retrypolicy.Builder[bool]().WithMaxRetries(-1).Build()
	cb := circuitbreaker.Builder[bool]().
		WithFailureThreshold(3).
		WithDelay(10 * time.Minute).
		Build()
	stub, reset := testutil.ErrorNTimesThenReturn[bool](testutil.ErrConnecting, 2, true)
	setup := func() {
		reset()
		policytesting.ResetCircuitBreaker(cb)
	}

	// When / Then
	testutil.Test[bool](t).
		With(rp, cb).
		Setup(setup).
		Get(stub).
		AssertSuccess(3, 3, true, func() {
			assert.Equal(t, uint(1), cb.Metrics().Successes())
			assert.Equal(t, uint(2), cb.Metrics().Failures())
		})
}

// RetryPolicy -> CircuitBreaker
//
// Tests RetryPolicy with a CircuitBreaker that is open.
func TestRetryPolicyCircuitBreakerOpen(t *testing.T) {
	// Given
	rp := policytesting.WithRetryLogs(retrypolicy.Builder[any]()).Build()
	cb := policytesting.WithBreakerLogs(circuitbreaker.Builder[any]()).Build()
	setup := func() {
		policytesting.ResetCircuitBreaker(cb)
	}

	// When / Then
	testutil.Test[any](t).
		With(rp, cb).
		Setup(setup).
		Run(testutil.RunFn(errors.New("test"))).
		AssertFailure(3, 1, circuitbreaker.ErrOpen)
}

// CircuitBreaker -> RetryPolicy
func TestCircuitBreakerRetryPolicy(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[any]()
	cb := circuitbreaker.Builder[any]().WithFailureThreshold(3).Build()
	setup := func() {
		policytesting.ResetCircuitBreaker(cb)
	}

	// When / Then
	testutil.Test[any](t).
		With(cb, rp).
		Setup(setup).
		Run(testutil.RunFn(testutil.ErrInvalidState)).
		AssertFailure(3, 3, testutil.ErrInvalidState, func() {
			assert.Equal(t, uint(0), cb.Metrics().Successes())
			assert.Equal(t, uint(1), cb.Metrics().Failures())
			assert.True(t, cb.IsClosed())
		})
}

// Fallback -> RetryPolicy
func TestFallbackRetryPolicy(t *testing.T) {
	// Given
	fb := fallback.BuilderWithResult(true).HandleErrors(retrypolicy.ErrExceeded).Build()
	rp := retrypolicy.WithDefaults[bool]()

	// When / Then
	testutil.Test[bool](t).
		With(fb, rp).
		Get(testutil.GetFn(false, testutil.ErrInvalidArgument)).
		AssertSuccess(3, 3, true)

	// Given
	fb = fallback.WithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
		assert.False(t, exec.LastResult())
		assert.ErrorIs(t, exec.LastError(), testutil.ErrInvalidState)
		return true, nil
	})

	// When / Then
	testutil.Test[bool](t).
		With(fb, rp).
		Get(testutil.GetFn(false, testutil.ErrInvalidState)).
		AssertSuccess(3, 3, true)
}

// Fallback -> HedgePolicy
func TestFallbackHedgePolicy(t *testing.T) {
	// Given
	fb := fallback.WithResult(true)
	hp := hedgepolicy.WithDelay[bool](10 * time.Millisecond)

	// When / Then
	testutil.Test[bool](t).
		With(fb, hp).
		Get(func(execution failsafe.Execution[bool]) (bool, error) {
			time.Sleep(50 * time.Millisecond)
			return false, testutil.ErrInvalidArgument
		}).
		AssertSuccess(2, -1, true)
}

// RetryPolicy -> Fallback
func TestRetryPolicyFallback(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[string]()
	fb := fallback.WithResult("test")

	// When / Then
	testutil.Test[string](t).
		With(rp, fb).
		Get(testutil.GetFn("", testutil.ErrInvalidState)).
		AssertSuccess(1, 1, "test")
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
	setup := func() {
		policytesting.ResetCircuitBreaker(cb)
	}

	// When / Then
	testutil.Test[bool](t).
		With(fb, cb).
		Setup(setup).
		Get(testutil.GetFn(false, testutil.ErrInvalidState)).
		AssertSuccess(1, 1, true)
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
	testutil.Test[bool](t).
		With(fb, cb).
		Get(testutil.GetFn(true, nil)).
		AssertSuccess(1, 0, false)
}

// RetryPolicy -> RateLimiter
func TestRetryPolicyRateLimiter(t *testing.T) {
	// Given
	rpStats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.Builder[any](), rpStats).WithMaxAttempts(7).Build()
	rl := ratelimiter.BurstyBuilder[any](3, 1*time.Second).Build()
	setup := func() {
		rpStats.Reset()
		policytesting.ResetRateLimiter(rl)
	}

	// When / Then
	testutil.Test[any](t).
		With(rp, rl).
		Setup(setup).
		Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
		AssertFailure(7, 3, ratelimiter.ErrExceeded, func() {
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
	setup := func() {
		policytesting.ResetCircuitBreaker(cb)
	}

	// When / Then
	testutil.Test[string](t).
		With(fb, rp, cb).
		Setup(setup).
		Get(testutil.GetFn[string]("", testutil.ErrInvalidState)).
		AssertSuccess(3, 3, "test", func() {
			assert.Equal(t, uint(0), cb.Metrics().Successes())
			assert.Equal(t, uint(3), cb.Metrics().Failures())
			assert.True(t, cb.IsClosed())
		})
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
	testutil.Test[any](t).
		With(rp, to).
		Reset(toStats).
		Get(func(e failsafe.Execution[any]) (any, error) {
			if e.Attempts() <= 2 {
				time.Sleep(100 * time.Millisecond)
				assert.True(t, e.IsCanceled())
			} else {
				assert.False(t, e.IsCanceled())
			}
			return "success", nil
		}).
		AssertSuccess(3, 3, "success", func() {
			assert.Equal(t, 2, toStats.Executions())
		})
}

// RetryPolicy -> HedgePolicy
func TestRetryPolicyHedgePolicy(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	rp := policytesting.WithRetryStatsAndLogs(retrypolicy.Builder[any](), stats).Build()

	t.Run("when hedge runs multiple times", func(t *testing.T) {
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](10*time.Millisecond), stats).Build()

		testutil.Test[any](t).
			With(rp, hp).
			Reset(stats).
			Get(func(e failsafe.Execution[any]) (any, error) {
				time.Sleep(20 * time.Millisecond)
				return nil, testutil.ErrInvalidState
			}).
			AssertFailure(6, -1, testutil.ErrInvalidState, func() {
				assert.Equal(t, 2, stats.Retries())
				assert.Equal(t, 3, stats.Hedges())
			})
	})

	t.Run("when hedge returns error", func(t *testing.T) {
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[any](time.Second), stats).Build()

		testutil.Test[any](t).
			With(rp, hp).
			Reset(stats).
			Get(func(e failsafe.Execution[any]) (any, error) {
				return nil, testutil.ErrInvalidState
			}).
			AssertFailure(3, 3, testutil.ErrInvalidState, func() {
				assert.Equal(t, 2, stats.Retries())
				assert.Equal(t, 0, stats.Hedges())
			})
	})
}

// CircuitBreaker -> Timeout
func TestCircuitBreakerTimeout(t *testing.T) {
	// Given
	to := timeout.With[string](50 * time.Millisecond)
	cb := circuitbreaker.WithDefaults[string]()
	assert.True(t, cb.IsClosed())
	setup := func() {
		policytesting.ResetCircuitBreaker(cb)
	}

	// When / Then
	testutil.Test[string](t).
		With(cb, to).
		Setup(setup).
		Run(func(execution failsafe.Execution[string]) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		}).
		AssertFailure(1, 1, timeout.ErrExceeded, func() {
			assert.True(t, cb.IsOpen())
		})
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
	testutil.Test[bool](t).
		With(fb, to).
		Get(func(execution failsafe.Execution[bool]) (bool, error) {
			time.Sleep(100 * time.Millisecond)
			return false, nil
		}).
		AssertSuccess(1, 1, true)
}

// RetryPolicy -> Bulkhead
func TestRetryPolicyBulkhead(t *testing.T) {
	rp := retrypolicy.Builder[any]().WithMaxAttempts(7).Build()
	bh := bulkhead.With[any](2)
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	testutil.Test[any](t).
		With(rp, bh).
		Run(testutil.RunFn(errors.New("test"))).
		AssertFailure(7, 0, bulkhead.ErrFull)
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

	// When / Then
	testutil.Test[any](t).
		With(hp, to).
		Reset(stats, toStats).
		Run(func(e failsafe.Execution[any]) error {
			testutil.WaitAndAssertCanceled(t, time.Second, e)
			return errors.New("not cancellable")
		}).
		AssertFailure(3, -1, timeout.ErrExceeded, func() {
			assert.Equal(t, 2, stats.Hedges())
			assert.Equal(t, 3, toStats.Executions())
		})
}

// CachePolicy -> RetryPolicy
func TestCachePolicyRetryPolicy(t *testing.T) {
	// Given
	cpStats := &policytesting.Stats{}
	cache, failsafeCache := policytesting.NewCache[any]()
	cp := policytesting.WithCacheStats[any](cachepolicy.Builder(failsafeCache), cpStats).WithKey("foo").Build()
	rpStats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.Builder[any](), rpStats).Build()
	execution, reset := testutil.ErrorNTimesThenReturn[any](testutil.ErrInvalidState, 2, "bar")
	setup := func() {
		cpStats.Reset()
		rpStats.Reset()
		reset()
		clear(cache)
	}

	// When / Then
	testutil.Test[any](t).
		With(cp, rp).
		Setup(setup).
		Get(execution).
		AssertSuccess(3, 3, "bar", func() {
			assert.Equal(t, 3, rpStats.Executions())
			assert.Equal(t, 2, rpStats.Retries())
			assert.Equal(t, 1, cpStats.Caches())
			assert.Equal(t, 0, cpStats.CacheHits())
			assert.Equal(t, 1, cpStats.CacheMisses())
		})
	testutil.Test[any](t).
		With(cp, rp).
		Reset(cpStats).
		Get(execution).
		AssertSuccess(1, 0, "bar", func() {
			assert.Equal(t, 0, cpStats.Caches())
			assert.Equal(t, 1, cpStats.CacheHits())
			assert.Equal(t, 0, cpStats.CacheMisses())
		})
}
