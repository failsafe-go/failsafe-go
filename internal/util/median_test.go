package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMovingMedian(t *testing.T) {
	t.Run("not full window", func(t *testing.T) {
		mm := NewMovingMedian(3)
		median := mm.Add(5.0)
		assert.Equal(t, 5.0, median)
		assert.Equal(t, 0.0, mm.Median())
	})

	t.Run("full window", func(t *testing.T) {
		mm := NewMovingMedian(3)
		assert.Equal(t, 1.0, mm.Add(1.0))
		assert.Equal(t, 2.0, mm.Add(2.0))
		assert.Equal(t, 2.0, mm.Add(3.0))
		assert.Equal(t, 2.0, mm.Median())
	})

	t.Run("moving median", func(t *testing.T) {
		mm := NewMovingMedian(3)
		mm.Add(1.0)
		mm.Add(2.0)
		mm.Add(3.0)
		median := mm.Add(4.0)
		assert.Equal(t, 3.0, median)
	})

	t.Run("unsorted input", func(t *testing.T) {
		mm := NewMovingMedian(5)
		mm.Add(5.0)
		mm.Add(2.0)
		mm.Add(8.0)
		mm.Add(1.0)
		median := mm.Add(9.0)
		assert.Equal(t, 5.0, median)
	})
}

func TestMovingMedian_Reset(t *testing.T) {
	mm := NewMovingMedian(3)
	mm.Add(5.0)
	mm.Add(2.0)
	mm.Add(8.0)
	assert.NotEqual(t, 0.0, mm.Median())

	mm.Reset()
	assert.Equal(t, 0.0, mm.Median())
}
