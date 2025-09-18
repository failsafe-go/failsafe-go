package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ ExecutionStats = &countingStats{}
var _ ExecutionStats = &timedStats{}

func TestCountingStatsShouldReturnUninitializedValues(t *testing.T) {
	stats := NewCountingStats(100).(*countingStats)
	for i := 0; i < 100; i++ {
		assert.Equal(t, -1, stats.setNext(true))
	}

	assert.Equal(t, 1, stats.setNext(true))
	assert.Equal(t, 1, stats.setNext(true))
}

func TestCountingStats(t *testing.T) {
	stats := NewCountingStats(100)
	assert.Equal(t, 0.0, stats.SuccessRate())
	assert.Equal(t, 0.0, stats.FailureRate())
	assert.Equal(t, uint(0), stats.ExecutionCount())

	recordExecutions(stats, 50, func(i int) bool {
		return i%3 == 0
	})

	assert.Equal(t, uint(17), stats.SuccessCount())
	assert.Equal(t, .34, stats.SuccessRate())
	assert.Equal(t, uint(33), stats.FailureCount())
	assert.Equal(t, .66, stats.FailureRate())
	assert.Equal(t, uint(50), stats.ExecutionCount())

	recordSuccesses(stats, 100)

	assert.Equal(t, uint(100), stats.SuccessCount())
	assert.Equal(t, 1.0, stats.SuccessRate())
	assert.Equal(t, uint(0), stats.FailureCount())
	assert.Equal(t, 0.0, stats.FailureRate())
	assert.Equal(t, uint(100), stats.ExecutionCount())
}

func TestTimedStats(t *testing.T) {
	clock := testutil.NewTestClock(900)

	// Given 4 Buckets representing 1 second each
	stats := NewTimedStats(4, 4*time.Second, clock).(*timedStats)
	assert.Equal(t, 0.0, stats.SuccessRate())
	assert.Equal(t, 0.0, stats.FailureRate())
	assert.Equal(t, uint(0), stats.ExecutionCount())

	// Record into bucket 1
	recordExecutions(stats, 50, func(i int) bool { // currentTime = 0
		return i%5 == 0
	})
	assert.Equal(t, int64(0), stats.HeadTime)
	assert.Equal(t, uint(10), stats.SuccessCount())
	assert.Equal(t, .2, stats.SuccessRate())
	assert.Equal(t, uint(40), stats.FailureCount())
	assert.Equal(t, .8, stats.FailureRate())
	assert.Equal(t, uint(50), stats.ExecutionCount())

	// Record into bucket 2
	clock.SetTime(1000)
	recordSuccesses(stats, 10)
	assert.Equal(t, int64(1), stats.HeadTime)
	assert.Equal(t, uint(20), stats.SuccessCount())
	assert.Equal(t, .33, stats.SuccessRate())
	assert.Equal(t, uint(40), stats.FailureCount())
	assert.Equal(t, .67, stats.FailureRate())
	assert.Equal(t, uint(60), stats.ExecutionCount())

	// Record into bucket 3
	clock.SetTime(2500)
	recordFailures(stats, 20)
	assert.Equal(t, int64(2), stats.HeadTime)
	assert.Equal(t, uint(20), stats.SuccessCount())
	assert.Equal(t, .25, stats.SuccessRate())
	assert.Equal(t, uint(60), stats.FailureCount())
	assert.Equal(t, .75, stats.FailureRate())
	assert.Equal(t, uint(80), stats.ExecutionCount())

	// Record into bucket 4
	clock.SetTime(3100)
	recordExecutions(stats, 25, func(i int) bool {
		return i%5 == 0
	})
	assert.Equal(t, int64(3), stats.HeadTime)
	assert.Equal(t, uint(25), stats.SuccessCount())
	assert.Equal(t, .24, stats.SuccessRate())
	assert.Equal(t, uint(80), stats.FailureCount())
	assert.Equal(t, .76, stats.FailureRate())
	assert.Equal(t, uint(105), stats.ExecutionCount())

	// Record into bucket 2, skipping bucket 1
	clock.SetTime(5400)
	recordSuccesses(stats, 8)
	assert.Equal(t, int64(5), stats.HeadTime)
	// Assert bucket 1 was skipped and Reset based on its previous start time
	bucket1 := stats.Buckets[0]
	assert.Equal(t, uint(0), bucket1.successes)
	assert.Equal(t, uint(0), bucket1.failures)
	assert.Equal(t, uint(13), stats.SuccessCount())
	assert.Equal(t, .25, stats.SuccessRate())
	assert.Equal(t, uint(40), stats.FailureCount())
	assert.Equal(t, .75, stats.FailureRate())
	assert.Equal(t, uint(53), stats.ExecutionCount())

	// Record into bucket 4, skipping bucket 3
	clock.SetTime(7300)
	recordFailures(stats, 5)
	assert.Equal(t, int64(7), stats.HeadTime)
	// Assert bucket 3 was skipped and Reset based on its previous start time
	bucket3 := stats.Buckets[2]
	assert.Equal(t, uint(0), bucket3.successes)
	assert.Equal(t, uint(0), bucket3.failures)
	assert.Equal(t, uint(8), stats.SuccessCount())
	assert.Equal(t, .62, stats.SuccessRate())
	assert.Equal(t, uint(5), stats.FailureCount())
	assert.Equal(t, .38, stats.FailureRate())
	assert.Equal(t, uint(13), stats.ExecutionCount())

	// Skip all Buckets, starting at 1 again
	clock.SetTime(22500)
	stats.ExpireBuckets()
	assert.Equal(t, int64(22), stats.HeadTime)
	for _, b := range stats.Buckets {
		assert.Equal(t, uint(0), b.successes)
		assert.Equal(t, uint(0), b.failures)
	}
	assert.Equal(t, 0.0, stats.SuccessRate())
	assert.Equal(t, 0.0, stats.FailureRate())
	assert.Equal(t, uint(0), stats.ExecutionCount())

	// Record into bucket 2
	clock.SetTime(23100)
	recordSuccesses(stats, 3)
	assert.Equal(t, int64(23), stats.HeadTime)
	assert.Equal(t, uint(3), stats.SuccessCount())
}

func recordExecutions(stats ExecutionStats, count int, successPredicate func(index int) bool) {
	for i := 0; i < count; i++ {
		if successPredicate(i) {
			stats.RecordSuccess()
		} else {
			stats.RecordFailure()
		}
	}
}

func recordSuccesses(stats ExecutionStats, count int) {
	for i := 0; i < count; i++ {
		stats.RecordSuccess()
	}
}

func recordFailures(stats ExecutionStats, count int) {
	for i := 0; i < count; i++ {
		stats.RecordFailure()
	}
}
