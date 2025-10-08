package priority

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/influxdata/tdigest"
	"github.com/stretchr/testify/assert"
)

func TestPriority_RandomLevel(t *testing.T) {
	level := High.RandomLevel()
	assert.GreaterOrEqual(t, level, 300)
	assert.LessOrEqual(t, level, 399)
}

func TestContextWithPriority(t *testing.T) {
	assert.Equal(t, High, ContextWithPriority(context.Background(), High).Value(PriorityKey))
}

func TestContextWithLevel(t *testing.T) {
	assert.Equal(t, 12, ContextWithLevel(context.Background(), 12).Value(LevelKey))
}

func TestFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected Priority
	}{
		{
			name:     "with an empty context",
			ctx:      context.Background(),
			expected: -1,
		},
		{
			name:     "with a context with priority",
			ctx:      ContextWithPriority(context.Background(), High),
			expected: High,
		},
		{
			name:     "with a context with an invalid priority type",
			ctx:      context.WithValue(context.Background(), PriorityKey, "foo"),
			expected: -1,
		},
		{
			name:     "with a context with an invalid priority value",
			ctx:      context.WithValue(context.Background(), PriorityKey, 10),
			expected: -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			priority := FromContext(tc.ctx)
			assert.Equal(t, tc.expected, priority)
		})
	}
}

func TestLevelFromContext(t *testing.T) {
	t.Run("with level", func(t *testing.T) {
		// Given
		expectedLevel := 250
		ctx := ContextWithLevel(context.Background(), expectedLevel)

		// When
		level := LevelFromContext(ctx)

		// Then
		assert.Equal(t, expectedLevel, level)
	})

	t.Run("with priority and no level", func(t *testing.T) {
		// Given
		ctx := ContextWithPriority(context.Background(), High)

		// When
		level := LevelFromContext(ctx)

		// Then
		assert.GreaterOrEqual(t, level, High.MinLevel())
		assert.LessOrEqual(t, level, High.MaxLevel())
	})

	t.Run("with no priority or level", func(t *testing.T) {
		// Given
		ctx := context.Background()

		// When
		level := LevelFromContext(ctx)

		// Then
		assert.Equal(t, -1, level)
	})
}

func TestWindowedLevelTracker_Empty(t *testing.T) {
	tracker := NewLevelTracker(1000)

	// Empty tracker should return 0
	assert.Equal(t, 0, tracker.GetLevel(0.5))
	assert.Equal(t, 0, tracker.GetLevel(0.0))
	assert.Equal(t, 0, tracker.GetLevel(1.0))
}

func TestWindowedLevelTracker_SingleValue(t *testing.T) {
	tracker := NewLevelTracker(1000)

	tracker.RecordLevel(250)

	// All quantiles should return the only value
	assert.Equal(t, 250, tracker.GetLevel(0.0))
	assert.Equal(t, 250, tracker.GetLevel(0.25))
	assert.Equal(t, 250, tracker.GetLevel(0.5))
	assert.Equal(t, 250, tracker.GetLevel(0.75))
	assert.Equal(t, 250, tracker.GetLevel(1.0))
}

func TestWindowedLevelTracker_BasicQuantiles(t *testing.T) {
	tracker := NewLevelTracker(1000)

	// Record levels in order: 100, 200, 300, 400
	levels := []int{400, 200, 100, 300}
	for _, level := range levels {
		tracker.RecordLevel(level)
	}

	// Test quantiles
	assert.Equal(t, 100, tracker.GetLevel(0.0))
	assert.Equal(t, 100, tracker.GetLevel(0.25))
	assert.Equal(t, 200, tracker.GetLevel(0.5))
	assert.Equal(t, 300, tracker.GetLevel(0.75))
	assert.Equal(t, 400, tracker.GetLevel(1.0))
}

func TestWindowedLevelTracker_UniformDistribution(t *testing.T) {
	tracker := NewLevelTracker(100)

	for i := 0; i < 100; i++ {
		tracker.RecordLevel(i)
	}

	assert.Equal(t, 24, tracker.GetLevel(0.25))
	assert.Equal(t, 49, tracker.GetLevel(0.50))
	assert.Equal(t, 74, tracker.GetLevel(0.75))
}

func TestWindowedLevelTracker_SlidingWindow(t *testing.T) {
	tracker := NewLevelTracker(3)

	// Window should be [1, 2, 3]
	tracker.RecordLevel(1)
	tracker.RecordLevel(2)
	tracker.RecordLevel(3)
	assert.Equal(t, 2, tracker.GetLevel(0.5))

	// Window should be [2, 3, 4]
	tracker.RecordLevel(4)
	assert.Equal(t, 3, tracker.GetLevel(0.5))

	// Window should be [4, 5, 6]
	tracker.RecordLevel(5)
	tracker.RecordLevel(6)
	assert.Equal(t, 5, tracker.GetLevel(0.5))
}

func TestWindowedLevelTracker_RejectionThreshold(t *testing.T) {
	tracker := NewLevelTracker(100)
	for i := 0; i < 70; i++ {
		tracker.RecordLevel(i % 51) // levels 0-50
	}
	for i := 0; i < 20; i++ {
		tracker.RecordLevel(51 + (i % 50)) // levels 51-100
	}
	for i := 0; i < 10; i++ {
		tracker.RecordLevel(101 + (i % 50)) // levels 101-150
	}

	assert.True(t, tracker.GetLevel(0.3) <= 50)
	assert.True(t, tracker.GetLevel(0.9) > 50)
}

func BenchmarkLevelTracker_Record(b *testing.B) {
	b.Run("TDigest", func(b *testing.B) {
		tracker := newTDigestLeveLTracker()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			level := rand.Intn(499)
			tracker.RecordLevel(level)
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		tracker := NewLevelTracker(1000)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			level := rand.Intn(499)
			tracker.RecordLevel(level)
		}
	})
}

func BenchmarkLevelTracker_GetLevel(b *testing.B) {
	b.Run("TDigest", func(b *testing.B) {
		tracker := newTDigestLeveLTracker()

		for i := 0; i < 1000; i++ {
			tracker.RecordLevel(rand.Intn(499))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = tracker.GetLevel(0.3)
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		tracker := NewLevelTracker(1000)

		for i := 0; i < 1000; i++ {
			tracker.RecordLevel(rand.Intn(499))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = tracker.GetLevel(0.3)
		}
	})
}

func BenchmarkLevelTracker_RecordAndGet(b *testing.B) {
	b.Run("TDigest", func(b *testing.B) {
		tracker := newTDigestLeveLTracker()

		for i := 0; i < 1000; i++ {
			tracker.RecordLevel(rand.Intn(499))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tracker.RecordLevel(rand.Intn(499))
			if i%100 == 0 {
				_ = tracker.GetLevel(0.3)
			}
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		tracker := NewLevelTracker(1000)

		for i := 0; i < 1000; i++ {
			tracker.RecordLevel(rand.Intn(499))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tracker.RecordLevel(rand.Intn(499))
			if i%100 == 0 {
				_ = tracker.GetLevel(0.3)
			}
		}
	})
}

func BenchmarkLevelTracker_RealisticWorkload(b *testing.B) {
	priorityDist := func() int {
		r := rand.Float64()
		if r < 0.7 { // 70% high priority
			return High.RandomLevel()
		} else if r < 0.9 { // 20% medium priority
			return Medium.RandomLevel()
		} else { // 10% low priority
			return Low.RandomLevel()
		}
	}

	b.Run("TDigest", func(b *testing.B) {
		tracker := newTDigestLeveLTracker()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tracker.RecordLevel(priorityDist())
			if i%100 == 0 {
				_ = tracker.GetLevel(0.3)
			}
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		tracker := NewLevelTracker(1000)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tracker.RecordLevel(priorityDist())
			if i%100 == 0 {
				_ = tracker.GetLevel(0.3)
			}
		}
	})
}

func BenchmarkLevelTracker_Memory(b *testing.B) {
	b.Run("TDigest", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = newTDigestLeveLTracker()
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewLevelTracker(1000)
		}
	})
}

// tDigestLevelTracker provides a reference point for comparing the windowedLevelTracker.
type tDigestLevelTracker struct {
	mu     sync.Mutex
	digest *tdigest.TDigest
}

func newTDigestLeveLTracker() LevelTracker {
	return &tDigestLevelTracker{
		digest: tdigest.NewWithCompression(100),
	}
}

func (lt *tDigestLevelTracker) RecordLevel(level int) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.digest.Add(float64(level), 1.0)
}

func (lt *tDigestLevelTracker) GetLevel(quantile float64) int {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	if level := lt.digest.Quantile(quantile); !math.IsNaN(level) {
		return int(level)
	}
	return 0
}
