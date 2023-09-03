package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe/internal/testutil"
)

// Asserts that wait times and available permits are expected, over time, when calling acquirePermits.
func TestBurstyAcquirePermits(t *testing.T) {
	// Given 2 max permits per second
	stats, stopwatch := newBurstyLimiterStats(2, time.Second)

	assert.Equal(t, 3000, acquireNTimes(stats, 1, 7))
	assert.Equal(t, -5, stats.availablePermits)
	assert.Equal(t, 0, stats.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(800)
	assert.Equal(t, 3200, acquire(stats, 3))
	assert.Equal(t, -8, stats.availablePermits)
	assert.Equal(t, 0, stats.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(2300)
	assert.Equal(t, 2700, acquire(stats, 1))
	assert.Equal(t, -5, stats.availablePermits)
	assert.Equal(t, 2, stats.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(3500)
	assert.Equal(t, 2500, acquireNTimes(stats, 1, 3))
	assert.Equal(t, -6, stats.availablePermits)
	assert.Equal(t, 3, stats.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(7000)
	assert.Equal(t, 0, acquire(stats, 1))
	assert.Equal(t, 1, stats.availablePermits)
	assert.Equal(t, 7, stats.currentPeriod)

	// Given 5 max permits per second
	stats, stopwatch = newBurstyLimiterStats(5, 1*time.Second)

	stopwatch.CurrentTime = testutil.MillisToNanos(300)
	assert.Equal(t, 0, acquire(stats, 3))
	assert.Equal(t, 2, stats.availablePermits)
	assert.Equal(t, 0, stats.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(1550)
	assert.Equal(t, 450, acquire(stats, 10))
	assert.Equal(t, -5, stats.availablePermits)
	assert.Equal(t, 1, stats.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(2210)
	assert.Equal(t, 790, acquire(stats, 2)) // Must wait till next period
	assert.Equal(t, -2, stats.availablePermits)
	assert.Equal(t, 2, stats.currentPeriod)
}

func newBurstyLimiterStats(maxPermits int, period time.Duration) (*burstyRateLimiterStats[any], *testutil.TestStopwatch) {
	stats := BurstyBuilder[any](maxPermits, period).Build().(*rateLimiter[any]).stats.(*burstyRateLimiterStats[any])
	stopwatch := &testutil.TestStopwatch{}
	stats.stopwatch = stopwatch
	return stats, stopwatch
}
