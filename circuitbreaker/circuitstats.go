package circuitbreaker

import (
	"math"
	"time"

	"github.com/bits-and-blooms/bitset"

	"github.com/failsafe-go/failsafe-go/internal/util"
)

// Stats for a CircuitBreaker.
// Implementations are not concurrency safe and must be guarded externally.
type stats interface {
	executionCount() uint
	failureCount() uint
	failureRate() uint
	successCount() uint
	successRate() uint
	recordFailure()
	recordSuccess()
	reset()
}

// The default number of buckets to aggregate time-based stats into.
const defaultBucketCount = 10

// A stats implementation that counts execution results using a BitSet.
type countingStats struct {
	bitSet *bitset.BitSet
	size   uint

	// Index to write next entry to
	currentIndex uint
	occupiedBits uint
	successes    uint
	failures     uint
}

func newStats[R any](config *config[R], supportsTimeBased bool, capacity uint) stats {
	if supportsTimeBased && config.failureThresholdingPeriod != 0 {
		return newTimedStats(defaultBucketCount, config.failureThresholdingPeriod, config.clock)
	}
	return newCountingStats(capacity)
}

func newCountingStats(size uint) *countingStats {
	return &countingStats{
		bitSet: bitset.New(size),
		size:   size,
	}
}

/*
Sets the value of the next bit in the bitset, returning the previous value, else -1 if no previous value was set for the bit.

value is true if positive/success, false if negative/failure
*/
func (c *countingStats) setNext(value bool) int {
	previousValue := -1
	if c.occupiedBits < c.size {
		c.occupiedBits++
	} else {
		if c.bitSet.Test(c.currentIndex) {
			previousValue = 1
		} else {
			previousValue = 0
		}
	}

	c.bitSet.SetTo(c.currentIndex, value)
	c.currentIndex = c.indexAfter(c.currentIndex)

	if value {
		if previousValue != 1 {
			c.successes++
		}
		if previousValue == 0 {
			c.failures--
		}
	} else {
		if previousValue != 0 {
			c.failures++
		}
		if previousValue == 1 {
			c.successes--
		}
	}

	return previousValue
}

func (c *countingStats) indexAfter(index uint) uint {
	if index == c.size-1 {
		return 0
	}
	return index + 1
}

func (c *countingStats) executionCount() uint {
	return c.occupiedBits
}

func (c *countingStats) failureCount() uint {
	return c.failures
}

func (c *countingStats) failureRate() uint {
	if c.occupiedBits == 0 {
		return 0
	}
	return uint(math.Round(float64(c.failures) / float64(c.occupiedBits) * 100.0))
}

func (c *countingStats) successCount() uint {
	return c.successes
}

func (c *countingStats) successRate() uint {
	if c.occupiedBits == 0 {
		return 0
	}
	return uint(math.Round(float64(c.successes) / float64(c.occupiedBits) * 100.0))
}

func (c *countingStats) recordFailure() {
	c.setNext(false)
}

func (c *countingStats) recordSuccess() {
	c.setNext(true)
}

func (c *countingStats) reset() {
	c.bitSet.ClearAll()
	c.currentIndex = 0
	c.occupiedBits = 0
	c.successes = 0
	c.failures = 0
}

// A stats implementation that counts execution results within a time period, and buckets results to minimize overhead.
type timedStats struct {
	clock      util.Clock
	bucketSize time.Duration
	windowSize time.Duration

	// Mutable state
	buckets      []bucket
	summary      stat
	currentIndex int
}

type bucket struct {
	stat
	startTime int64
}

type stat struct {
	successes uint
	failures  uint
}

func (s *stat) reset() {
	s.successes = 0
	s.failures = 0
}

func (s *stat) add(bucket *bucket) {
	s.successes += bucket.successes
	s.failures += bucket.failures
}

func (s *stat) remove(bucket *bucket) {
	s.successes -= bucket.successes
	s.failures -= bucket.failures
}

func newTimedStats(bucketCount int, thresholdingPeriod time.Duration, clock util.Clock) *timedStats {
	buckets := make([]bucket, bucketCount)
	for i := 0; i < bucketCount; i++ {
		buckets[i] = bucket{
			stat:      stat{},
			startTime: -1,
		}
	}
	buckets[0].startTime = clock.CurrentUnixNano()
	result := &timedStats{
		buckets:    buckets,
		windowSize: thresholdingPeriod,
		bucketSize: thresholdingPeriod / time.Duration(bucketCount),
		clock:      clock,
		summary:    stat{},
	}
	return result
}

func (s *timedStats) getCurrentBucket() *bucket {
	currentBucket := &s.buckets[s.currentIndex]
	timeDiff := s.clock.CurrentUnixNano() - currentBucket.startTime
	bucketsToMove := int(timeDiff / s.bucketSize.Nanoseconds())

	if bucketsToMove > len(s.buckets) {
		// Reset all buckets
		s.reset()
	} else {
		// Reset some buckets
		for i := 0; i < bucketsToMove; i++ {
			previousBucket := currentBucket
			currentBucket = &s.buckets[s.nextIndex()]
			s.summary.remove(currentBucket)
			currentBucket.reset()
			if currentBucket.startTime == -1 {
				currentBucket.startTime = previousBucket.startTime + s.bucketSize.Nanoseconds()
			} else {
				currentBucket.startTime = currentBucket.startTime + s.windowSize.Nanoseconds()
			}
		}
	}

	return currentBucket
}

func (s *timedStats) nextIndex() int {
	s.currentIndex = (s.currentIndex + 1) % len(s.buckets)
	return s.currentIndex
}

func (s *timedStats) executionCount() uint {
	return s.summary.successes + s.summary.failures
}

func (s *timedStats) failureCount() uint {
	return s.summary.failures
}

func (s *timedStats) failureRate() uint {
	executions := s.executionCount()
	if executions == 0 {
		return 0
	}
	return uint(math.Round(float64(s.summary.failures) / float64(executions) * 100.0))
}

func (s *timedStats) successCount() uint {
	return s.summary.successes
}

func (s *timedStats) successRate() uint {
	executions := s.executionCount()
	if executions == 0 {
		return 0
	}
	return uint(math.Round(float64(s.summary.successes) / float64(executions) * 100.0))
}

func (s *timedStats) recordFailure() {
	s.getCurrentBucket().failures++
	s.summary.failures++
}

func (s *timedStats) recordSuccess() {
	s.getCurrentBucket().successes++
	s.summary.successes++
}

func (s *timedStats) reset() {
	startTime := s.clock.CurrentUnixNano()
	for i := range s.buckets {
		bucket := &s.buckets[i]
		bucket.reset()
		bucket.startTime = startTime
		startTime += s.bucketSize.Nanoseconds()
	}
	s.summary.reset()
	s.currentIndex = 0
}
