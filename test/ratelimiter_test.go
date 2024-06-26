package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
)

func TestRateLimiterPermitAcquiredAfterWait(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderWithMaxRate[string](50 * time.Millisecond).
		WithMaxWaitTime(2 * time.Second).
		Build()

	// When / Then
	limiter.TryAcquirePermit() // limiter should now be out of permits
	testutil.Test[string](t).
		With(limiter).
		Get(testutil.GetFn("test", nil)).
		AssertSuccess(1, 1, "test")
}

func TestShouldReturnRateLimitExceededError(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderWithMaxRate[any](1 * time.Hour).Build()

	// When / Then
	limiter.TryAcquirePermit() // limiter should now be out of permits
	testutil.Test[any](t).
		With(limiter).
		Run(testutil.RunFn(nil)).
		AssertFailure(1, 0, ratelimiter.ErrExceeded)
}

// Asserts that an exceeded maxWaitTime causes ErrExceeded.
func TestRateLimiterMaxWaitTimeExceeded(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderWithMaxRate[any](10 * time.Millisecond).Build()

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
}

func TestCancelRateLimiting(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderWithMaxRate[any](time.Second).Build()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// When / Then
	assert.NoError(t, limiter.AcquirePermit(nil))
	assert.Error(t, limiter.AcquirePermit(ctx))
}
