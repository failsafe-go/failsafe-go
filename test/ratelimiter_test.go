package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
	"failsafe/internal/testutil"
	"failsafe/ratelimiter"
)

func TestReservePermit(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()

	// When / Then
	assert.Equal(t, time.Duration(0), limiter.ReservePermit())
	assert.True(t, limiter.ReservePermit() > 0)
	assert.True(t, limiter.ReservePermit() > 100)
}

func TestTryReservePermit(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()

	// When / Then
	assert.Equal(t, time.Duration(0), limiter.TryReservePermit(1*time.Millisecond))
	assert.Equal(t, time.Duration(-1), limiter.TryReservePermit(10*time.Millisecond))
	assert.True(t, limiter.TryReservePermit(100*time.Millisecond).Milliseconds() > 0)
	assert.True(t, limiter.TryReservePermit(200*time.Millisecond).Milliseconds() > 100)
	assert.Equal(t, time.Duration(-1), limiter.TryReservePermit(100*time.Millisecond))
}

func TestPermitAcquiredAfterWait(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderForMaxRate[string](50 * time.Millisecond).
		WithMaxWaitTime(2 * time.Second).
		Build()

	// When / Then
	limiter.TryAcquirePermit() // limiter should now be out of permits
	testutil.TestGetSuccess(t, failsafe.With[string](limiter),
		func(exec failsafe.Execution[string]) (string, error) {
			return "test", nil
		},
		1, 1, "test")
}

func TestShouldReturnRateLimitExceededError(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()

	// When / Then
	limiter.TryAcquirePermit() // limiter should now be out of permits
	testutil.TestRunFailure(t, failsafe.With[any](limiter),
		func(execution failsafe.Execution[any]) error {
			return nil
		},
		1, 0, ratelimiter.ErrRateLimitExceeded)
}

// Asserts that an exceeded maxWaitTime causes ErrRateLimitExceeded.
func TestMaxWaitTimeExceeded(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderForMaxRate[any](10 * time.Millisecond).Build()

	// When / Then
	limiter.AcquirePermitsWithMaxWait(nil, 50, time.Minute) // limiter should now be well over its max permits
	testutil.TestRunFailure(t, failsafe.With[any](limiter),
		func(execution failsafe.Execution[any]) error {
			return nil
		},
		1, 0, ratelimiter.ErrRateLimitExceeded)
}

func TestCancelRateLimiting(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderForMaxRate[any](time.Second).Build()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// When / Then
	assert.NoError(t, limiter.AcquirePermit(nil))
	assert.Error(t, limiter.AcquirePermit(ctx))
}
