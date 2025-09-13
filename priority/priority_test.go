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
