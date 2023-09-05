package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ circuitStats = &countingCircuitStats{}
var _ circuitStats = &timedCircuitStats{}

func TestCountingStatsShouldReturnUninitializedValues(t *testing.T) {
	stats := newCountingCircuitStats(100)
	for i := 0; i < 100; i++ {
		assert.Equal(t, -1, stats.setNext(true))
	}

	assert.Equal(t, 1, stats.setNext(true))
	assert.Equal(t, 1, stats.setNext(true))
}

func TestCountingStats(t *testing.T) {
	stats := newCountingCircuitStats(100)
	assert.Equal(t, uint(0), stats.getSuccessRate())
	assert.Equal(t, uint(0), stats.getFailureRate())
	assert.Equal(t, uint(0), stats.getExecutionCount())

	recordExecutions(stats, 50, func(i int) bool {
		return i%3 == 0
	})

	assert.Equal(t, uint(17), stats.getSuccessCount())
	assert.Equal(t, uint(34), stats.getSuccessRate())
	assert.Equal(t, uint(33), stats.getFailureCount())
	assert.Equal(t, uint(66), stats.getFailureRate())
	assert.Equal(t, uint(50), stats.getExecutionCount())

	recordSuccesses(stats, 100)

	assert.Equal(t, uint(100), stats.getSuccessCount())
	assert.Equal(t, uint(100), stats.getSuccessRate())
	assert.Equal(t, uint(0), stats.getFailureCount())
	assert.Equal(t, uint(0), stats.getFailureRate())
	assert.Equal(t, uint(100), stats.getExecutionCount())
}

func TestTimedStats(t *testing.T) {
	clock := &testutil.TestClock{}

	// Given 4 buckets representing 1 second each
	stats := newTimedCircuitStats(4, 4*time.Second, clock)
	assert.Equal(t, uint(0), stats.getSuccessRate())
	assert.Equal(t, uint(0), stats.getFailureRate())
	assert.Equal(t, uint(0), stats.getExecutionCount())

	// Record into bucket 1
	recordExecutions(stats, 50, func(i int) bool { // currentTime = 0
		return i%5 == 0
	})
	assert.Equal(t, 0, stats.currentIndex)
	assert.Equal(t, int64(0), stats.getCurrentBucket().startTime)
	assert.Equal(t, uint(10), stats.getSuccessCount())
	assert.Equal(t, uint(20), stats.getSuccessRate())
	assert.Equal(t, uint(40), stats.getFailureCount())
	assert.Equal(t, uint(80), stats.getFailureRate())
	assert.Equal(t, uint(50), stats.getExecutionCount())

	// Record into bucket 2
	clock.CurrentTime = testutil.MillisToNanos(1000)
	recordSuccesses(stats, 10)
	assert.Equal(t, 1, stats.currentIndex)
	assert.Equal(t, testutil.MillisToNanos(1000), stats.getCurrentBucket().startTime)
	assert.Equal(t, uint(20), stats.getSuccessCount())
	assert.Equal(t, uint(33), stats.getSuccessRate())
	assert.Equal(t, uint(40), stats.getFailureCount())
	assert.Equal(t, uint(67), stats.getFailureRate())
	assert.Equal(t, uint(60), stats.getExecutionCount())

	// Record into bucket 3
	clock.CurrentTime = testutil.MillisToNanos(2500)
	recordFailures(stats, 20)
	assert.Equal(t, 2, stats.currentIndex)
	assert.Equal(t, testutil.MillisToNanos(2000), stats.getCurrentBucket().startTime)
	assert.Equal(t, uint(20), stats.getSuccessCount())
	assert.Equal(t, uint(25), stats.getSuccessRate())
	assert.Equal(t, uint(60), stats.getFailureCount())
	assert.Equal(t, uint(75), stats.getFailureRate())
	assert.Equal(t, uint(80), stats.getExecutionCount())

	// Record into bucket 4
	clock.CurrentTime = testutil.MillisToNanos(3100)
	recordExecutions(stats, 25, func(i int) bool {
		return i%5 == 0
	})
	assert.Equal(t, 3, stats.currentIndex)
	assert.Equal(t, testutil.MillisToNanos(3000), stats.getCurrentBucket().startTime)
	assert.Equal(t, uint(25), stats.getSuccessCount())
	assert.Equal(t, uint(24), stats.getSuccessRate())
	assert.Equal(t, uint(80), stats.getFailureCount())
	assert.Equal(t, uint(76), stats.getFailureRate())
	assert.Equal(t, uint(105), stats.getExecutionCount())

	// Record into bucket 2, skipping bucket 1
	clock.CurrentTime = testutil.MillisToNanos(5400)
	recordSuccesses(stats, 8)
	assert.Equal(t, 1, stats.currentIndex)
	// Assert bucket 1 was skipped and reset based on its previous start time
	bucket1 := stats.buckets[0]
	assert.Equal(t, testutil.MillisToNanos(4000), bucket1.startTime)
	assert.Equal(t, uint(0), bucket1.successes)
	assert.Equal(t, uint(0), bucket1.failures)
	assert.Equal(t, testutil.MillisToNanos(5000), stats.getCurrentBucket().startTime)
	assert.Equal(t, uint(13), stats.getSuccessCount())
	assert.Equal(t, uint(25), stats.getSuccessRate())
	assert.Equal(t, uint(40), stats.getFailureCount())
	assert.Equal(t, uint(75), stats.getFailureRate())
	assert.Equal(t, uint(53), stats.getExecutionCount())

	// Record into bucket 4, skipping bucket 3
	clock.CurrentTime = testutil.MillisToNanos(7300)
	recordFailures(stats, 5)
	assert.Equal(t, 3, stats.currentIndex)
	// Assert bucket 3 was skipped and reset based on its previous start time
	bucket3 := stats.buckets[2]
	assert.Equal(t, testutil.MillisToNanos(6000), bucket3.startTime)
	assert.Equal(t, uint(0), bucket3.successes)
	assert.Equal(t, uint(0), bucket3.failures)
	assert.Equal(t, testutil.MillisToNanos(7000), stats.getCurrentBucket().startTime)
	assert.Equal(t, uint(8), stats.getSuccessCount())
	assert.Equal(t, uint(62), stats.getSuccessRate())
	assert.Equal(t, uint(5), stats.getFailureCount())
	assert.Equal(t, uint(38), stats.getFailureRate())
	assert.Equal(t, uint(13), stats.getExecutionCount())

	// Skip all buckets, starting at 1 again
	startTime := testutil.MillisToNanos(22500)
	clock.CurrentTime = startTime
	stats.getCurrentBucket()
	assert.Equal(t, 0, stats.currentIndex)
	for _, b := range stats.buckets {
		assert.Equal(t, startTime, b.startTime)
		assert.Equal(t, uint(0), b.successes)
		assert.Equal(t, uint(0), b.failures)
		startTime += testutil.MillisToNanos(1000)
	}
	assert.Equal(t, uint(0), stats.getSuccessRate())
	assert.Equal(t, uint(0), stats.getFailureRate())
	assert.Equal(t, uint(0), stats.getExecutionCount())
}

func recordExecutions(stats circuitStats, count int, successPredicate func(index int) bool) {
	for i := 0; i < count; i++ {
		if successPredicate(i) {
			stats.recordSuccess()
		} else {
			stats.recordFailure()
		}
	}
}

func recordSuccesses(stats circuitStats, count int) {
	for i := 0; i < count; i++ {
		stats.recordSuccess()
	}
}

func recordFailures(stats circuitStats, count int) {
	for i := 0; i < count; i++ {
		stats.recordFailure()
	}
}
