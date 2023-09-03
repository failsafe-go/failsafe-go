package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe/internal/testutil"
)

var _ RateLimiter[any] = &rateLimiter[any]{}

func TestAcquirePermit(t *testing.T) {
	limiter := SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()
	setTestStopwatch(limiter)

	elapsed := testutil.Timed(func() {
		limiter.AcquirePermit() // waits 0
		limiter.AcquirePermit() // waits 100
		limiter.AcquirePermit() // waits 200
	})
	assert.True(t, elapsed.Milliseconds() >= 300 && elapsed.Milliseconds() <= 400)
}

func TestAcquireWithMaxWaitTime(t *testing.T) {
	limiter := SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()
	setTestStopwatch(limiter)

	limiter.AcquirePermitWithMaxWait(100 * time.Millisecond)        // waits 0
	limiter.AcquirePermitWithMaxWait(1000 * time.Millisecond)       // waits 100
	err := limiter.AcquirePermitWithMaxWait(100 * time.Millisecond) // waits 200
	assert.ErrorIs(t, ErrRateLimitExceeded, err)
}

func TestTryAcquirePermit(t *testing.T) {
	limiter := SmoothBuilderForMaxRate[any](100 * time.Nanosecond).Build()
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

func TestTryAcquirePermitWithMaxWaitTime(t *testing.T) {
	limiter := SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()
	stopwatch := setTestStopwatch(limiter)

	assert.True(t, limiter.TryAcquirePermitWithMaxWait(50*time.Millisecond))
	assert.False(t, limiter.TryAcquirePermitWithMaxWait(50*time.Millisecond))
	elapsed := testutil.Timed(func() {
		assert.True(t, limiter.TryAcquirePermitWithMaxWait(100*time.Millisecond))
	})
	assert.True(t, elapsed.Milliseconds() >= 100 && elapsed.Milliseconds() < 200)

	stopwatch.CurrentTime = 200 * time.Millisecond.Nanoseconds()
	assert.True(t, limiter.TryAcquirePermitWithMaxWait(50*time.Millisecond))
	assert.False(t, limiter.TryAcquirePermitWithMaxWait(50*time.Millisecond))
	elapsed = testutil.Timed(func() {
		assert.True(t, limiter.TryAcquirePermitWithMaxWait(100*time.Millisecond))
	})
	assert.True(t, elapsed.Milliseconds() >= 100 && elapsed.Milliseconds() < 200)
}

func TestTryAcquirePermitsWithMaxWaitTime(t *testing.T) {
	limiter := SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()
	stopwatch := setTestStopwatch(limiter)

	assert.False(t, limiter.TryAcquirePermitsWithMaxWait(2, 50*time.Millisecond))
	elapsed := testutil.Timed(func() {
		assert.True(t, limiter.TryAcquirePermitsWithMaxWait(2, 100*time.Millisecond))
	})
	assert.True(t, elapsed.Milliseconds() >= 100 && elapsed.Milliseconds() < 200)

	stopwatch.CurrentTime = 450 * time.Millisecond.Nanoseconds()
	assert.False(t, limiter.TryAcquirePermitsWithMaxWait(2, 10*time.Millisecond))
	elapsed = testutil.Timed(func() {
		assert.True(t, limiter.TryAcquirePermitsWithMaxWait(2, 300*time.Millisecond))
	})
	assert.True(t, elapsed.Milliseconds() >= 50 && elapsed.Milliseconds() < 100)
}

func setTestStopwatch[R any](limiter RateLimiter[R]) *testutil.TestStopwatch {
	stopwatch := &testutil.TestStopwatch{}
	limiter.(*rateLimiter[R]).stats.(*smoothRateLimiterStats[R]).stopwatch = stopwatch
	return stopwatch
}
