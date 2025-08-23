package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestAdaptiveLimiter(t *testing.T) {
	t.Run("should acquire permit after wait", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[string]().WithLimits(2, 2, 2).WithMaxWaitTime(time.Second).Build()
		before := func() {
			p1 := shouldTryAcquire(t, limiter)
			p2 := shouldTryAcquire(t, limiter) // limiter should be full
			go func() {
				time.Sleep(200 * time.Millisecond)
				p1.Drop()
				p2.Drop() // limiter should be empty
			}()
		}

		// When / Then
		testutil.Test[string](t).
			With(limiter).
			Before(before).
			Get(testutil.GetFn("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("when limit not exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		lb := adaptivelimiter.NewBuilder[any]().WithLimits(2, 2, 2)
		limiter := policytesting.WithAdaptiveLimiterStatsAndLogs(lb, stats, true).Build()

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Reset(stats).
			Get(testutil.GetFn[any]("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("when limit exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		lb := adaptivelimiter.NewBuilder[any]().WithLimits(2, 2, 2)
		limiter := policytesting.WithAdaptiveLimiterStatsAndLogs(lb, stats, true).Build()
		var p1, p2 adaptivelimiter.Permit
		before := func() {
			p1 = shouldTryAcquire(t, limiter)
			p2 = shouldTryAcquire(t, limiter) // limiter should be full
		}
		after := func() {
			p1.Drop()
			p2.Drop()
		}

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Before(before).
			After(after).
			Reset(stats).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded, func() {
				assert.Equal(t, 1, stats.LimitsExceeded())
			})
	})

	// Asserts that an exceeded maxWaitTime causes ErrExceeded.
	t.Run("when maxWaitTime exceeded", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().WithLimits(2, 2, 2).WithMaxWaitTime(20 * time.Millisecond).Build()
		shouldTryAcquire(t, limiter)
		shouldTryAcquire(t, limiter) // limiter should be full

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded)
	})

	// Asserts that a short maxWaitTime still allows a permit to be claimed.
	t.Run("with short maxWaitTime", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().WithLimits(2, 2, 2).Build()

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Run(testutil.RunFn(nil)).
			AssertSuccess(1, 1, nil)
	})
}

func shouldTryAcquire[R any](t *testing.T, limiter adaptivelimiter.AdaptiveLimiter[R]) adaptivelimiter.Permit {
	permit, ok := limiter.TryAcquirePermit()
	require.NotNil(t, permit)
	require.True(t, ok)
	return permit
}
