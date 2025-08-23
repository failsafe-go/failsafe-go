package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
)

func TestRateLimiter(t *testing.T) {
	t.Run("should acquire permit after wait", func(t *testing.T) {
		// Given
		limiter := ratelimiter.NewSmoothBuilderWithMaxRate[string](50 * time.Millisecond).
			WithMaxWaitTime(2 * time.Second).
			Build()

		// When / Then
		limiter.TryAcquirePermit() // limiter should now be out of permits
		testutil.Test[string](t).
			With(limiter).
			Get(testutil.GetFn("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("should return ErrExceeded", func(t *testing.T) {
		// Given
		limiter := ratelimiter.NewSmoothBuilderWithMaxRate[any](1 * time.Hour).Build()

		// When / Then
		limiter.TryAcquirePermit() // limiter should now be out of permits
		testutil.Test[any](t).
			With(limiter).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, ratelimiter.ErrExceeded)
	})

	// Asserts that an exceeded maxWaitTime causes ErrExceeded.
	t.Run("with maxWaitTime exceeded", func(t *testing.T) {
		// Given
		limiter := ratelimiter.NewSmoothBuilderWithMaxRate[any](10 * time.Millisecond).Build()

		// When
		go func() {
			assert.NoError(t, limiter.AcquirePermitsWithMaxWait(nil, 50, time.Minute)) // limiter should now be well over its max permits
		}()
		time.Sleep(100 * time.Millisecond)

		// Then
		testutil.Test[any](t).
			With(limiter).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, ratelimiter.ErrExceeded)
	})

	t.Run("should cancel rate limiting", func(t *testing.T) {
		// Given
		limiter := ratelimiter.NewSmoothBuilderWithMaxRate[any](time.Second).Build()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		// When / Then
		assert.NoError(t, limiter.AcquirePermit(nil))
		assert.Error(t, limiter.AcquirePermit(ctx))
	})
}
