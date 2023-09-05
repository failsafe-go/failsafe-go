package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ RateLimiter[any] = &rateLimiter[any]{}

func TestAcquirePermit(t *testing.T) {
	limiter := SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()
	setTestStopwatch(limiter)

	elapsed := testutil.Timed(func() {
		limiter.AcquirePermit(nil) // waits 0
		limiter.AcquirePermit(nil) // waits 100
		limiter.AcquirePermit(nil) // waits 200
	})
	assert.True(t, elapsed.Milliseconds() >= 300 && elapsed.Milliseconds() <= 400)
}

func TestAcquireWithMaxWaitTime(t *testing.T) {
	limiter := SmoothBuilderForMaxRate[any](100 * time.Millisecond).Build()
	setTestStopwatch(limiter)

	limiter.AcquirePermitWithMaxWait(nil, 100*time.Millisecond)        // waits 0
	limiter.AcquirePermitWithMaxWait(nil, 1000*time.Millisecond)       // waits 100
	err := limiter.AcquirePermitWithMaxWait(nil, 100*time.Millisecond) // waits 200
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

func setTestStopwatch[R any](limiter RateLimiter[R]) *testutil.TestStopwatch {
	stopwatch := &testutil.TestStopwatch{}
	limiter.(*rateLimiter[R]).stats.(*smoothRateLimiterStats[R]).stopwatch = stopwatch
	return stopwatch
}
