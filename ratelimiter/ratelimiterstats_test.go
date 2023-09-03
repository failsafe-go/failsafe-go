package ratelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe/internal/testutil"
)

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
