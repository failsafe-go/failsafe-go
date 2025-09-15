package priority

import (
	"context"
	"testing"

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

func TestLevelTracker_RecordLevel(t *testing.T) {
	// Given
	tracker := NewLevelTracker()
	assert.Equal(t, 0.0, tracker.GetLevel(0.5))

	// When
	tracker.RecordLevel(250)

	// Then
	assert.Equal(t, 250.0, tracker.GetLevel(0.5))
}

func TestLevelTracker_GetLevel(t *testing.T) {
	tracker := NewLevelTracker()

	// Given
	levels := []int{100, 200, 300, 400}
	for _, level := range levels {
		tracker.RecordLevel(level)
	}

	// When
	p25 := tracker.GetLevel(0.25)
	p75 := tracker.GetLevel(0.75)

	// Then
	assert.True(t, p25 <= p75)
	assert.True(t, p25 >= 100.0)
	assert.True(t, p75 <= 400.0)
}
