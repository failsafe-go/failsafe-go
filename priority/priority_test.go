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
