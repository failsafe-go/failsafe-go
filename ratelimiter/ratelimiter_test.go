package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ RateLimiter[any] = &rateLimiter[any]{}

func TestAcquirePermit(t *testing.T) {
	limiter := NewSmoothBuilderWithMaxRate[any](100 * time.Millisecond).Build()
	setTestStopwatch(limiter)

	elapsed := testutil.Timed(func() {
		assert.Nil(t, limiter.AcquirePermit(nil)) // waits 0
		assert.Nil(t, limiter.AcquirePermit(nil)) // waits 100
		assert.Nil(t, limiter.AcquirePermit(nil)) // waits 200
	})
	assert.True(t, elapsed.Milliseconds() >= 300 && elapsed.Milliseconds() <= 400)
}

func TestAcquirePermitWithMaxWaitTime(t *testing.T) {
	limiter := NewSmoothBuilderWithMaxRate[any](100 * time.Millisecond).Build()
	setTestStopwatch(limiter)

	assert.Nil(t, limiter.AcquirePermitWithMaxWait(nil, 100*time.Millisecond))  // waits 0
	assert.Nil(t, limiter.AcquirePermitWithMaxWait(nil, 1000*time.Millisecond)) // waits 100
	err := limiter.AcquirePermitWithMaxWait(nil, 100*time.Millisecond)          // waits 200
	assert.ErrorIs(t, ErrExceeded, err)
}

func TestTryAcquirePermit(t *testing.T) {
	limiter := NewSmoothBuilderWithMaxRate[any](100 * time.Nanosecond).Build()
	stopwatch := setTestStopwatch(limiter)

	assert.True(t, limiter.TryAcquirePermit())
	assert.False(t, limiter.TryAcquirePermit())

	stopwatch.CurrentTime = 150
	assert.True(t, limiter.TryAcquirePermit())
	assert.False(t, limiter.TryAcquirePermit())

	stopwatch.CurrentTime = 210
	assert.True(t, limiter.TryAcquirePermit())
	assert.False(t, limiter.TryAcquirePermit())
}

func TestReservePermit(t *testing.T) {
	// Given
	limiter := NewSmoothBuilderWithMaxRate[any](100 * time.Millisecond).Build()

	// When / Then
	assert.Equal(t, time.Duration(0), limiter.ReservePermit())
	assert.True(t, limiter.ReservePermit() > 0)
	assert.True(t, limiter.ReservePermit() > 100)
}

func TestTryReservePermit(t *testing.T) {
	// Given
	limiter := NewSmoothBuilderWithMaxRate[any](100 * time.Millisecond).Build()

	// When / Then
	assert.Equal(t, time.Duration(0), limiter.TryReservePermit(1*time.Millisecond))
	assert.Equal(t, time.Duration(-1), limiter.TryReservePermit(10*time.Millisecond))
	assert.True(t, limiter.TryReservePermit(100*time.Millisecond).Milliseconds() > 0)
	assert.True(t, limiter.TryReservePermit(200*time.Millisecond).Milliseconds() > 100)
	assert.Equal(t, time.Duration(-1), limiter.TryReservePermit(100*time.Millisecond))
}

func setTestStopwatch[R any](limiter RateLimiter[R]) *testutil.TestStopwatch {
	stopwatch := &testutil.TestStopwatch{}
	limiter.(*rateLimiter[R]).stats.(*smoothStats[R]).stopwatch = stopwatch
	return stopwatch
}
