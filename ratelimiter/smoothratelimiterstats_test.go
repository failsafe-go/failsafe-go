package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe/internal/testutil"
)

// Asserts that wait times and available permits are expected, over time, when calling acquirePermits.
func TestSmoothAcquirePermits(t *testing.T) {
	// Given 1 permit every 500ns
	stats, stopwatch := newSmoothLimiterStats(500 * time.Millisecond)

	waitTime := acquireNTimes(stats, 1, 7)
	assertSmoothRateLimiterStats(t, stats, waitTime, 3000, 3500)

	stopwatch.CurrentTime = testutil.MillisToNanos(800)
	waitTime = acquire(stats, 3)
	assertSmoothRateLimiterStats(t, stats, waitTime, 3700, 5000)

	stopwatch.CurrentTime = testutil.MillisToNanos(2300)
	waitTime = acquire(stats, 1)
	assertSmoothRateLimiterStats(t, stats, waitTime, 2700, 5500)

	stopwatch.CurrentTime = testutil.MillisToNanos(3500)
	waitTime = acquire(stats, 3)
	assertSmoothRateLimiterStats(t, stats, waitTime, 3000, 7000)

	stopwatch.CurrentTime = testutil.MillisToNanos(9100)
	waitTime = acquire(stats, 3)
	assertSmoothRateLimiterStats(t, stats, waitTime, 900, 10500)

	stopwatch.CurrentTime = testutil.MillisToNanos(11000)
	waitTime = acquire(stats, 1)
	assertSmoothRateLimiterStats(t, stats, waitTime, 0, 11500)

	// Given 1 permit every 200ns
	stats, stopwatch = newSmoothLimiterStats(200 * time.Millisecond)

	waitTime = acquire(stats, 3)
	assertSmoothRateLimiterStats(t, stats, waitTime, 400, 600)

	stopwatch.CurrentTime = testutil.MillisToNanos(550)
	waitTime = acquire(stats, 2)
	assertSmoothRateLimiterStats(t, stats, waitTime, 250, 1000)

	stopwatch.CurrentTime = testutil.MillisToNanos(2210)
	waitTime = acquire(stats, 2)
	assertSmoothRateLimiterStats(t, stats, waitTime, 190, 2600)
}

func TestSmoothAcquireInitialStats(t *testing.T) {
	// Given 1 permit every 100ns
	stats, stopwatch := newSmoothLimiterStats(100 * time.Nanosecond)

	testutil.AssertDuration(t, 0, stats.acquirePermits(1, -1))
	testutil.AssertDuration(t, 100, stats.acquirePermits(1, -1))
	stopwatch.CurrentTime = 100
	testutil.AssertDuration(t, 100, stats.acquirePermits(1, -1))
	testutil.AssertDuration(t, 200, stats.acquirePermits(1, -1))

	// Given 1 permit every 100ns
	stats, stopwatch = newSmoothLimiterStats(100 * time.Nanosecond)

	testutil.AssertDuration(t, 0, stats.acquirePermits(1, -1))
	stopwatch.CurrentTime = 150
	testutil.AssertDuration(t, 0, stats.acquirePermits(1, -1))
	stopwatch.CurrentTime = 250
	testutil.AssertDuration(t, 50, stats.acquirePermits(2, -1))
}

func newSmoothLimiterStats(maxRate time.Duration) (*smoothRateLimiterStats[any], *testutil.TestStopwatch) {
	stats := SmoothBuilderForMaxRate[any](maxRate).Build().(*rateLimiter[any]).stats.(*smoothRateLimiterStats[any])
	stopwatch := &testutil.TestStopwatch{}
	stats.stopwatch = stopwatch
	return stats, stopwatch
}

func acquire(stats rateLimiterStats, permits int) (waitTime int) {
	return acquireNTimes(stats, permits, 1)
}

func acquireNTimes(stats rateLimiterStats, permits int, numberOfCalls int) (waitTime int) {
	waitTime = 0
	for i := 0; i < numberOfCalls; i++ {
		waitTime = int(stats.acquirePermits(permits, -1).Milliseconds())
	}
	return waitTime
}

func assertSmoothRateLimiterStats(t *testing.T, stats *smoothRateLimiterStats[any], waitTime int, expectedWaitTime int, expectedNextFreePermitTime int) {
	assert.Equal(t, expectedWaitTime, waitTime)
	assert.Equal(t, expectedNextFreePermitTime, int(stats.nextFreePermitTime.Milliseconds()))

	// Asserts that the nextFreePermitNanos makes sense relative to the elapsedTime and waitTime
	computedNextFreePermitTime := int(stats.stopwatch.ElapsedTime().Milliseconds()) + waitTime + int(stats.config.interval.Milliseconds())
	assert.Equal(t, computedNextFreePermitTime, int(stats.nextFreePermitTime.Milliseconds()))
}
