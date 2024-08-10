package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ stats = &smoothStats[any]{}
var _ stats = &burstyStats[any]{}

// Asserts that wait times and available permits are expected, over time, when calling acquirePermits.
func TestSmoothAcquirePermits(t *testing.T) {
	// Given 1 permit every 500ns
	s, stopwatch := newSmoothLimiterStats(500 * time.Millisecond)

	waitTime := acquireNTimes(s, 1, 7)
	assertSmoothRateLimiterStats(t, s, waitTime, 3000, 3500)

	stopwatch.CurrentTime = testutil.MillisToNanos(800)
	waitTime = acquire(s, 3)
	assertSmoothRateLimiterStats(t, s, waitTime, 3700, 5000)

	stopwatch.CurrentTime = testutil.MillisToNanos(2300)
	waitTime = acquire(s, 1)
	assertSmoothRateLimiterStats(t, s, waitTime, 2700, 5500)

	stopwatch.CurrentTime = testutil.MillisToNanos(3500)
	waitTime = acquire(s, 3)
	assertSmoothRateLimiterStats(t, s, waitTime, 3000, 7000)

	stopwatch.CurrentTime = testutil.MillisToNanos(9100)
	waitTime = acquire(s, 3)
	assertSmoothRateLimiterStats(t, s, waitTime, 900, 10500)

	stopwatch.CurrentTime = testutil.MillisToNanos(11000)
	waitTime = acquire(s, 1)
	assertSmoothRateLimiterStats(t, s, waitTime, 0, 11500)

	// Given 1 permit every 200ns
	s, stopwatch = newSmoothLimiterStats(200 * time.Millisecond)

	waitTime = acquire(s, 3)
	assertSmoothRateLimiterStats(t, s, waitTime, 400, 600)

	stopwatch.CurrentTime = testutil.MillisToNanos(550)
	waitTime = acquire(s, 2)
	assertSmoothRateLimiterStats(t, s, waitTime, 250, 1000)

	stopwatch.CurrentTime = testutil.MillisToNanos(2210)
	waitTime = acquire(s, 2)
	assertSmoothRateLimiterStats(t, s, waitTime, 190, 2600)
}

func TestSmoothAcquireInitialStats(t *testing.T) {
	// Given 1 permit every 100ns
	s, stopwatch := newSmoothLimiterStats(100 * time.Nanosecond)

	testutil.AssertDuration(t, 0, s.acquirePermits(1, -1))
	testutil.AssertDuration(t, 100, s.acquirePermits(1, -1))
	stopwatch.CurrentTime = 100
	testutil.AssertDuration(t, 100, s.acquirePermits(1, -1))
	testutil.AssertDuration(t, 200, s.acquirePermits(1, -1))

	// Given 1 permit every 100ns
	s, stopwatch = newSmoothLimiterStats(100 * time.Nanosecond)

	testutil.AssertDuration(t, 0, s.acquirePermits(1, -1))
	stopwatch.CurrentTime = 150
	testutil.AssertDuration(t, 0, s.acquirePermits(1, -1))
	stopwatch.CurrentTime = 250
	testutil.AssertDuration(t, 50, s.acquirePermits(2, -1))
}

// Asserts that wait times and available permits are expected, over time, when calling acquirePermits.
func TestBurstyAcquirePermits(t *testing.T) {
	// Given 2 max permits per second
	s, stopwatch := newBurstyLimiterStats(2, time.Second)

	assert.Equal(t, 3000, acquireNTimes(s, 1, 7))
	assert.Equal(t, -5, s.availablePermits)
	assert.Equal(t, 0, s.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(800)
	assert.Equal(t, 3200, acquire(s, 3))
	assert.Equal(t, -8, s.availablePermits)
	assert.Equal(t, 0, s.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(2300)
	assert.Equal(t, 2700, acquire(s, 1))
	assert.Equal(t, -5, s.availablePermits)
	assert.Equal(t, 2, s.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(3500)
	assert.Equal(t, 2500, acquireNTimes(s, 1, 3))
	assert.Equal(t, -6, s.availablePermits)
	assert.Equal(t, 3, s.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(7000)
	assert.Equal(t, 0, acquire(s, 1))
	assert.Equal(t, 1, s.availablePermits)
	assert.Equal(t, 7, s.currentPeriod)

	// Given 5 max permits per second
	s, stopwatch = newBurstyLimiterStats(5, 1*time.Second)

	stopwatch.CurrentTime = testutil.MillisToNanos(300)
	assert.Equal(t, 0, acquire(s, 3))
	assert.Equal(t, 2, s.availablePermits)
	assert.Equal(t, 0, s.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(1550)
	assert.Equal(t, 450, acquire(s, 10))
	assert.Equal(t, -5, s.availablePermits)
	assert.Equal(t, 1, s.currentPeriod)

	stopwatch.CurrentTime = testutil.MillisToNanos(2210)
	assert.Equal(t, 790, acquire(s, 2)) // Must wait till next period
	assert.Equal(t, -2, s.availablePermits)
	assert.Equal(t, 2, s.currentPeriod)
}

func TestShouldAcquirePermitsEqually(t *testing.T) {
	test := func(statsFn func() (stats, *testutil.TestStopwatch)) {
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
	test(func() (stats, *testutil.TestStopwatch) {
		return newSmoothLimiterStats(500 * time.Second)
	})

	// Test for bursty stats
	test(func() (stats, *testutil.TestStopwatch) {
		return newBurstyLimiterStats(2, time.Second)
	})
}

// Asserts that acquire on a new stats object with a single permit has zero wait time.
func TestShouldHaveZeroWaitTime(t *testing.T) {
	test := func(statsFn func() stats) {
		assert.Equal(t, 0, acquire(statsFn(), 1))
	}

	// Test for smooth stats
	test(func() stats {
		s, _ := newSmoothLimiterStats(500 * time.Second)
		return s
	})

	// Test for bursty stats
	test(func() stats {
		s, _ := newBurstyLimiterStats(2, time.Second)
		return s
	})
}

func newSmoothLimiterStats(maxRate time.Duration) (*smoothStats[any], *testutil.TestStopwatch) {
	s := NewSmoothBuilderWithMaxRate[any](maxRate).Build().(*rateLimiter[any]).stats.(*smoothStats[any])
	stopwatch := &testutil.TestStopwatch{}
	s.stopwatch = stopwatch
	return s, stopwatch
}

func newBurstyLimiterStats(maxPermits uint, period time.Duration) (*burstyStats[any], *testutil.TestStopwatch) {
	s := NewBurstyBuilder[any](maxPermits, period).Build().(*rateLimiter[any]).stats.(*burstyStats[any])
	stopwatch := &testutil.TestStopwatch{}
	s.stopwatch = stopwatch
	return s, stopwatch
}

func acquire(stats stats, permits int) (waitTime int) {
	return acquireNTimes(stats, permits, 1)
}

func acquireNTimes(stats stats, permits int, numberOfCalls int) (waitTime int) {
	waitTime = 0
	for i := 0; i < numberOfCalls; i++ {
		waitTime = int(stats.acquirePermits(permits, -1).Milliseconds())
	}
	return waitTime
}

func assertSmoothRateLimiterStats(t *testing.T, stats *smoothStats[any], waitTime int, expectedWaitTime int, expectedNextFreePermitTime int) {
	assert.Equal(t, expectedWaitTime, waitTime)
	assert.Equal(t, expectedNextFreePermitTime, int(stats.nextFreePermitTime.Milliseconds()))

	// Asserts that the nextFreePermitNanos makes sense relative to the elapsedTime and waitTime
	computedNextFreePermitTime := int(stats.stopwatch.ElapsedTime().Milliseconds()) + waitTime + int(stats.interval.Milliseconds())
	assert.Equal(t, computedNextFreePermitTime, int(stats.nextFreePermitTime.Milliseconds()))
}
