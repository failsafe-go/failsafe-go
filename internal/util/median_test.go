package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMedianFilter(t *testing.T) {
	t.Run("single value", func(t *testing.T) {
		filter := NewMedianFilter(3)
		result := filter.Add(5.0)
		assert.Equal(t, 5.0, result)
	})

	t.Run("full window", func(t *testing.T) {
		filter := NewMedianFilter(3)
		assert.Equal(t, 1.0, filter.Add(1.0))
		assert.Equal(t, 2.0, filter.Add(2.0))
		assert.Equal(t, 2.0, filter.Add(3.0))
	})

	t.Run("rolling median", func(t *testing.T) {
		filter := NewMedianFilter(3)
		filter.Add(1.0)
		filter.Add(2.0)
		filter.Add(3.0)
		result := filter.Add(4.0)
		assert.Equal(t, 3.0, result)
	})

	t.Run("unsorted input", func(t *testing.T) {
		filter := NewMedianFilter(5)
		filter.Add(5.0)
		filter.Add(2.0)
		filter.Add(8.0)
		filter.Add(1.0)
		result := filter.Add(9.0)
		assert.Equal(t, 5.0, result)
	})
}
