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

func TestShouldAcquirePermitsEqually(t *testing.T) {
	test := func(statsFn func() (rateLimiterStats, *testutil.TestStopwatch)) {
		// Given
		stats1, stopwatch1 := statsFn()
		stats2, stopwatch2 := statsFn()

		// When making initial calls
		singleCallWait := acquire(stats1, 7)
		multipleCallWait := acquireNTimes(stats2, 1, 7)

		// Then
		assert.Equal(t, singleCallWait, multipleCallWait)

		// Given
		stopwatch1.CurrentTime = 2700
		stopwatch2.CurrentTime = 2700

		// When making initial calls
		singleCallWait = acquire(stats1, 5)
		multipleCallWait = acquireNTimes(stats2, 1, 5)

		// Then
		assert.Equal(t, singleCallWait, multipleCallWait)
	}

	// Test for smooth stats
	test(func() (rateLimiterStats, *testutil.TestStopwatch) {
		return newSmoothLimiterStats(500 * time.Second)
	})

	// Test for bursty stats
	test(func() (rateLimiterStats, *testutil.TestStopwatch) {
		return newBurstyLimiterStats(2, time.Second)
	})
}

// Asserts that acquire on a new stats object with a single permit has zero wait time.
func TestShouldHaveZeroWaitTime(t *testing.T) {
	test := func(statsFn func() rateLimiterStats) {
		assert.Equal(t, 0, acquire(statsFn(), 1))
	}

	// Test for smooth stats
	test(func() rateLimiterStats {
		stats, _ := newSmoothLimiterStats(500 * time.Second)
		return stats
	})

	// Test for bursty stats
	test(func() rateLimiterStats {
		stats, _ := newBurstyLimiterStats(2, time.Second)
		return stats
	})
}

func newSmoothLimiterStats(maxRate time.Duration) (*smoothRateLimiterStats[any], *testutil.TestStopwatch) {
	stats := SmoothBuilderForMaxRate[any](maxRate).Build().(*rateLimiter[any]).stats.(*smoothRateLimiterStats[any])
	stopwatch := &testutil.TestStopwatch{}
	stats.stopwatch = stopwatch
	return stats, stopwatch
}

func newBurstyLimiterStats(maxPermits int, period time.Duration) (*burstyRateLimiterStats[any], *testutil.TestStopwatch) {
	stats := BurstyBuilder[any](maxPermits, period).Build().(*rateLimiter[any]).stats.(*burstyRateLimiterStats[any])
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
