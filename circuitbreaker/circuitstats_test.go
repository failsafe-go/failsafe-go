package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ stats = &countingStats{}
var _ stats = &timedStats{}

func TestCountingStatsShouldReturnUninitializedValues(t *testing.T) {
	stats := newCountingStats(100)
	for i := 0; i < 100; i++ {
		assert.Equal(t, -1, stats.setNext(true))
	}

	assert.Equal(t, 1, stats.setNext(true))
	assert.Equal(t, 1, stats.setNext(true))
}

func TestCountingStats(t *testing.T) {
	stats := newCountingStats(100)
	assert.Equal(t, uint(0), stats.successRate())
	assert.Equal(t, uint(0), stats.failureRate())
	assert.Equal(t, uint(0), stats.executionCount())

	recordExecutions(stats, 50, func(i int) bool {
		return i%3 == 0
	})

	assert.Equal(t, uint(17), stats.successCount())
	assert.Equal(t, uint(34), stats.successRate())
	assert.Equal(t, uint(33), stats.failureCount())
	assert.Equal(t, uint(66), stats.failureRate())
	assert.Equal(t, uint(50), stats.executionCount())

	recordSuccesses(stats, 100)

	assert.Equal(t, uint(100), stats.successCount())
	assert.Equal(t, uint(100), stats.successRate())
	assert.Equal(t, uint(0), stats.failureCount())
	assert.Equal(t, uint(0), stats.failureRate())
	assert.Equal(t, uint(100), stats.executionCount())

	recordExecutions(stats, 30, func(i int) bool { return i%3 == 0 })

	assert.Equal(t, uint(80), stats.successCount())
	assert.Equal(t, uint(80), stats.successRate())
	assert.Equal(t, uint(20), stats.failureCount())
	assert.Equal(t, uint(20), stats.failureRate())
	assert.Equal(t, uint(100), stats.executionCount())
}

func TestTimedStats(t *testing.T) {
	clock := &testutil.TestClock{
		CurrentTime: testutil.MillisToNanos(900),
	}

	// Given 4 buckets representing 1 second each
	stats := newTimedStats(4, 4*time.Second, clock)
	assert.Equal(t, uint(0), stats.successRate())
	assert.Equal(t, uint(0), stats.failureRate())
	assert.Equal(t, uint(0), stats.executionCount())

	// Record into bucket 1
	recordExecutions(stats, 50, func(i int) bool { // currentTime = 0
		return i%5 == 0
	})
	assert.Equal(t, int64(0), stats.head)
	assert.Equal(t, uint(10), stats.successCount())
	assert.Equal(t, uint(20), stats.successRate())
	assert.Equal(t, uint(40), stats.failureCount())
	assert.Equal(t, uint(80), stats.failureRate())
	assert.Equal(t, uint(50), stats.executionCount())

	// Record into bucket 2
	clock.CurrentTime = testutil.MillisToNanos(1000)
	recordSuccesses(stats, 10)
	assert.Equal(t, int64(1), stats.head)
	assert.Equal(t, uint(20), stats.successCount())
	assert.Equal(t, uint(33), stats.successRate())
	assert.Equal(t, uint(40), stats.failureCount())
	assert.Equal(t, uint(67), stats.failureRate())
	assert.Equal(t, uint(60), stats.executionCount())

	// Record into bucket 3
	clock.CurrentTime = testutil.MillisToNanos(2500)
	recordFailures(stats, 20)
	assert.Equal(t, int64(2), stats.head)
	assert.Equal(t, uint(20), stats.successCount())
	assert.Equal(t, uint(25), stats.successRate())
	assert.Equal(t, uint(60), stats.failureCount())
	assert.Equal(t, uint(75), stats.failureRate())
	assert.Equal(t, uint(80), stats.executionCount())

	// Record into bucket 4
	clock.CurrentTime = testutil.MillisToNanos(3100)
	recordExecutions(stats, 25, func(i int) bool {
		return i%5 == 0
	})
	assert.Equal(t, int64(3), stats.head)
	assert.Equal(t, uint(25), stats.successCount())
	assert.Equal(t, uint(24), stats.successRate())
	assert.Equal(t, uint(80), stats.failureCount())
	assert.Equal(t, uint(76), stats.failureRate())
	assert.Equal(t, uint(105), stats.executionCount())

	// Record into bucket 2, skipping bucket 1
	clock.CurrentTime = testutil.MillisToNanos(5400)
	recordSuccesses(stats, 8)
	assert.Equal(t, int64(5), stats.head)
	// Assert bucket 1 was skipped and reset based on its previous start time
	bucket1 := stats.buckets[0]
	assert.Equal(t, uint(0), bucket1.successes)
	assert.Equal(t, uint(0), bucket1.failures)
	assert.Equal(t, uint(13), stats.successCount())
	assert.Equal(t, uint(25), stats.successRate())
	assert.Equal(t, uint(40), stats.failureCount())
	assert.Equal(t, uint(75), stats.failureRate())
	assert.Equal(t, uint(53), stats.executionCount())

	// Record into bucket 4, skipping bucket 3
	clock.CurrentTime = testutil.MillisToNanos(7300)
	recordFailures(stats, 5)
	assert.Equal(t, int64(7), stats.head)
	// Assert bucket 3 was skipped and reset based on its previous start time
	bucket3 := stats.buckets[2]
	assert.Equal(t, uint(0), bucket3.successes)
	assert.Equal(t, uint(0), bucket3.failures)
	assert.Equal(t, uint(8), stats.successCount())
	assert.Equal(t, uint(62), stats.successRate())
	assert.Equal(t, uint(5), stats.failureCount())
	assert.Equal(t, uint(38), stats.failureRate())
	assert.Equal(t, uint(13), stats.executionCount())

	// Skip all buckets, starting at 1 again
	clock.CurrentTime = testutil.MillisToNanos(22500)
	stats.currentBucket()
	assert.Equal(t, int64(22), stats.head)
	for _, b := range stats.buckets {
		assert.Equal(t, uint(0), b.successes)
		assert.Equal(t, uint(0), b.failures)
	}
	assert.Equal(t, uint(0), stats.successRate())
	assert.Equal(t, uint(0), stats.failureRate())
	assert.Equal(t, uint(0), stats.executionCount())

	// Record into bucket 2
	clock.CurrentTime = testutil.MillisToNanos(23100)
	recordSuccesses(stats, 3)
	assert.Equal(t, int64(23), stats.head)
	assert.Equal(t, uint(3), stats.successCount())
}

func recordExecutions(stats stats, count int, successPredicate func(index int) bool) {
	for i := 0; i < count; i++ {
		if successPredicate(i) {
			stats.recordSuccess()
		} else {
			stats.recordFailure()
		}
	}
}

func recordSuccesses(stats stats, count int) {
	for i := 0; i < count; i++ {
		stats.recordSuccess()
	}
}

func recordFailures(stats stats, count int) {
	for i := 0; i < count; i++ {
		stats.recordFailure()
	}
}
