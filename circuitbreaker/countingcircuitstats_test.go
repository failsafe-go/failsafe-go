package circuitbreaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldReturnUninitializedValues(t *testing.T) {
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
