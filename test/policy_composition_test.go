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

func TestRetryPolicy_Composition(t *testing.T) {
	t.Run("with CircuitBreaker - success after failures", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewBuilder[bool]().WithMaxRetries(-1).Build()
		cb := circuitbreaker.NewBuilder[bool]().
			WithFailureThreshold(3).
			WithDelay(10 * time.Minute).
			Build()
		stub, reset := testutil.ErrorNTimesThenReturn[bool](testutil.ErrConnecting, 2, true)
		before := func() {
			reset()
			policytesting.Reset(cb)
		}

		// When / Then
		testutil.Test[bool](t).
			With(rp, cb).
			Before(before).
			Get(stub).
			AssertSuccess(3, 3, true, func() {
				assert.Equal(t, uint(1), cb.Metrics().Successes())
				assert.Equal(t, uint(2), cb.Metrics().Failures())
			})
	})

	// Tests RetryPolicy with a CircuitBreaker that is open.
	t.Run("with CircuitBreaker - open", func(t *testing.T) {
		// Given
		rp := policytesting.WithRetryLogs(retrypolicy.NewBuilder[any]()).Build()
		cb := policytesting.WithBreakerLogs(circuitbreaker.NewBuilder[any]()).Build()
		before := func() {
			policytesting.Reset(cb)
		}

		// When / Then
		testutil.Test[any](t).
			With(rp, cb).
			Before(before).
			Run(testutil.RunFn(errors.New("test"))).
			AssertFailure(3, 1, circuitbreaker.ErrOpen)
	})

	t.Run("with RateLimiter", func(t *testing.T) {
		// Given
		rpStats := &policytesting.Stats{}
		rp := policytesting.WithRetryStats(retrypolicy.NewBuilder[any](), rpStats).WithMaxAttempts(7).Build()
		rl := ratelimiter.NewBurstyBuilder[any](3, 1*time.Second).Build()
		before := func() {
			rpStats.Reset()
			policytesting.Reset(rl)
		}

		// When / Then
		testutil.Test[any](t).
			With(rp, rl).
			Before(before).
			Get(testutil.GetFn[any](nil, testutil.ErrInvalidState)).
			AssertFailure(7, 3, ratelimiter.ErrExceeded, func() {
				assert.Equal(t, 7, rpStats.Executions())
				assert.Equal(t, 6, rpStats.Retries())
			})
	})

	t.Run("with Fallback", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewWithDefaults[string]()
		fb := fallback.NewWithResult("test")

		// When / Then
		testutil.Test[string](t).
			With(rp, fb).
			Get(testutil.GetFn("", testutil.ErrInvalidState)).
			AssertSuccess(1, 1, "test")
	})

	// Tests 2 timeouts, then a success, and asserts the execution is cancelled after each timeout.
	t.Run("with Timeout", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewBuilder[any]().OnFailure(func(e failsafe.ExecutionEvent[any]) {
			assert.ErrorIs(t, e.LastError(), timeout.ErrExceeded)
		}).Build()
		toStats := &policytesting.Stats{}
		to := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](50*time.Millisecond), toStats).Build()

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
	})

	t.Run("with Bulkhead", func(t *testing.T) {
		rp := retrypolicy.NewBuilder[any]().WithMaxAttempts(7).Build()
		bh := bulkhead.New[any](2)
		bh.TryAcquirePermit()
		bh.TryAcquirePermit() // bulkhead should be full

		testutil.Test[any](t).
			With(rp, bh).
			Run(testutil.RunFn(errors.New("test"))).
			AssertFailure(7, 0, bulkhead.ErrFull)
	})

	t.Run("with HedgePolicy", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		rp := policytesting.WithRetryStatsAndLogs(retrypolicy.NewBuilder[any](), stats).Build()

		t.Run("when hedge runs multiple times", func(t *testing.T) {
			hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[any](10*time.Millisecond), stats).Build()

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
			hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[any](time.Second), stats).Build()

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
	})
}

func TestCircuitBreaker_Composition(t *testing.T) {
	t.Run("with RetryPolicy", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewWithDefaults[any]()
		cb := circuitbreaker.NewBuilder[any]().WithFailureThreshold(3).Build()
		before := func() {
			policytesting.Reset(cb)
		}

		// When / Then
		testutil.Test[any](t).
			With(cb, rp).
			Before(before).
			Run(testutil.RunFn(testutil.ErrInvalidState)).
			AssertFailure(3, 3, testutil.ErrInvalidState, func() {
				assert.Equal(t, uint(0), cb.Metrics().Successes())
				assert.Equal(t, uint(1), cb.Metrics().Failures())
				assert.True(t, cb.IsClosed())
			})
	})

	t.Run("with Timeout", func(t *testing.T) {
		// Given
		to := timeout.New[string](50 * time.Millisecond)
		cb := circuitbreaker.NewWithDefaults[string]()
		assert.True(t, cb.IsClosed())
		before := func() {
			policytesting.Reset(cb)
		}

		// When / Then
		testutil.Test[string](t).
			With(cb, to).
			Before(before).
			Run(func(execution failsafe.Execution[string]) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			}).
			AssertFailure(1, 1, timeout.ErrExceeded, func() {
				assert.True(t, cb.IsOpen())
			})
	})
}

func TestFallback_Composition(t *testing.T) {
	t.Run("with RetryPolicy", func(t *testing.T) {
		// Given
		fb := fallback.NewBuilderWithResult(true).HandleErrors(retrypolicy.ErrExceeded).Build()
		rp := retrypolicy.NewWithDefaults[bool]()

		// When / Then
		testutil.Test[bool](t).
			With(fb, rp).
			Get(testutil.GetFn(false, testutil.ErrInvalidArgument)).
			AssertSuccess(3, 3, true)

		// Given
		fb = fallback.NewWithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
			assert.False(t, exec.LastResult())
			assert.ErrorIs(t, exec.LastError(), testutil.ErrInvalidState)
			return true, nil
		})

		// When / Then
		testutil.Test[bool](t).
			With(fb, rp).
			Get(testutil.GetFn(false, testutil.ErrInvalidState)).
			AssertSuccess(3, 3, true)
	})

	t.Run("with HedgePolicy", func(t *testing.T) {
		// Given
		fb := fallback.NewWithResult(true)
		hp := hedgepolicy.NewWithDelay[bool](10 * time.Millisecond)

		// When / Then
		testutil.Test[bool](t).
			With(fb, hp).
			Get(func(execution failsafe.Execution[bool]) (bool, error) {
				time.Sleep(50 * time.Millisecond)
				return false, testutil.ErrInvalidArgument
			}).
			AssertSuccess(2, -1, true)
	})

	// Tests fallback with a circuit breaker that is closed.
	t.Run("with CircuitBreaker - closed", func(t *testing.T) {
		// Given
		fb := fallback.NewWithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
			assert.False(t, exec.LastResult())
			assert.ErrorIs(t, testutil.ErrInvalidState, exec.LastError())
			return true, nil
		})
		cb := circuitbreaker.NewBuilder[bool]().WithSuccessThreshold(3).Build()
		before := func() {
			policytesting.Reset(cb)
		}

		// When / Then
		testutil.Test[bool](t).
			With(fb, cb).
			Before(before).
			Get(testutil.GetFn(false, testutil.ErrInvalidState)).
			AssertSuccess(1, 1, true)
	})

	// Tests fallback with a circuit breaker that is open.
	t.Run("with CircuitBreaker - open", func(t *testing.T) {
		// Given
		fb := fallback.NewWithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
			assert.False(t, exec.LastResult())
			assert.ErrorIs(t, circuitbreaker.ErrOpen, exec.LastError())
			return false, nil
		})
		cb := circuitbreaker.NewBuilder[bool]().WithSuccessThreshold(3).Build()

		// When / Then
		cb.Open()
		testutil.Test[bool](t).
			With(fb, cb).
			Get(testutil.GetFn(true, nil)).
			AssertSuccess(1, 0, false)
	})

	t.Run("with Timeout", func(t *testing.T) {
		// Given
		to := timeout.New[bool](10 * time.Millisecond)
		fb := fallback.NewWithFunc(func(e failsafe.Execution[bool]) (bool, error) {
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
	})
}

func TestHedgePolicy_Composition(t *testing.T) {
	// Hedge should be triggered twice since the timeouts are longer than the hedge delay.
	// Timeout should be triggered 3 times since the results from the hedges are not cancellable.
	t.Run("with Timeout", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[any](10*time.Millisecond).
			CancelIf(func(a any, err error) bool {
				return err == nil
			}).
			WithMaxHedges(2), stats).
			Build()
		toStats := &policytesting.Stats{}
		to := policytesting.WithTimeoutStatsAndLogs(timeout.NewBuilder[any](100*time.Millisecond), toStats).Build()

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
	})
}

func TestCachePolicy_Composition(t *testing.T) {
	t.Run("with RetryPolicy", func(t *testing.T) {
		// Given
		cpStats := &policytesting.Stats{}
		cache, failsafeCache := policytesting.NewCache[any]()
		cp := policytesting.WithCacheStats[any](cachepolicy.NewBuilder(failsafeCache), cpStats).WithKey("foo").Build()
		rpStats := &policytesting.Stats{}
		rp := policytesting.WithRetryStats(retrypolicy.NewBuilder[any](), rpStats).Build()
		execution, reset := testutil.ErrorNTimesThenReturn[any](testutil.ErrInvalidState, 2, "bar")
		before := func() {
			cpStats.Reset()
			rpStats.Reset()
			reset()
			clear(cache)
		}

		// When / Then
		testutil.Test[any](t).
			With(cp, rp).
			Before(before).
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
	})
}

// Fallback -> RetryPolicy -> CircuitBreaker
func TestMultiPolicy_Composition(t *testing.T) {
	t.Run("with Fallback -> RetryPolicy -> CircuitBreaker", func(t *testing.T) {
		// Given
		rp := retrypolicy.NewWithDefaults[string]()
		cb := circuitbreaker.NewBuilder[string]().WithFailureThreshold(5).Build()
		fb := fallback.NewWithResult("test")
		before := func() {
			policytesting.Reset(cb)
		}

		// When / Then
		testutil.Test[string](t).
			With(fb, rp, cb).
			Before(before).
			Get(testutil.GetFn[string]("", testutil.ErrInvalidState)).
			AssertSuccess(3, 3, "test", func() {
				assert.Equal(t, uint(0), cb.Metrics().Successes())
				assert.Equal(t, uint(3), cb.Metrics().Failures())
				assert.True(t, cb.IsClosed())
			})
	})
}
