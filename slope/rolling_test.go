package slope

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRollingSum_CalculateSlope(t *testing.T) {
	tests := []struct {
		name       string
		values     []float64
		capacity   uint
		expected   float64
		shouldNaN  bool
		windowSize int
	}{
		{
			name:       "wrapping negative slope",
			values:     []float64{5, 2, 3, 7, 6},
			windowSize: 5,
			expected:   .7,
		},
		{
			name:     "wrapping negative with small window",
			values:   []float64{5, 2, 3, 7, 6},
			expected: 1.5,
		},
		{
			name:       "wrapping negative with small window",
			values:     []float64{3, 7, 6},
			windowSize: 5,
			expected:   1.5,
		},
		{
			name:     "decreasing",
			values:   []float64{5, 4, 3, 2, 1},
			expected: -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			windowSize := tc.windowSize
			if windowSize == 0 {
				windowSize = 3
			}
			w := NewRollingSlope(windowSize)
			var slope float64
			for i, v := range tc.values {
				// w.addToSum(v)
				slope = w.AddValue(v)
				fmt.Println(fmt.Sprintf("Index %d, Window Size %d: Slope = %f", i, i+1, slope))
			}

			//	slope := w.calculateSlope()

			if tc.shouldNaN {
				assert.True(t, math.IsNaN(slope))
			} else {
				assert.InDelta(t, tc.expected, slope, 0.0001)
			}
		})
	}
}
