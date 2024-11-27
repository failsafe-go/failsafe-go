package adaptivelimiter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRTTWindow(t *testing.T) {
	w := newRTTWindow()
	w.add(100 * time.Millisecond)
	w.add(200 * time.Millisecond)
	w.add(300 * time.Millisecond)

	t.Run("should get minRTT", func(t *testing.T) {
		assert.Equal(t, 100*time.Millisecond, w.minRTT)
	})

	t.Run("should get average", func(t *testing.T) {
		assert.Equal(t, 200*time.Millisecond, w.average())
	})
}

func TestVariationWindow(t *testing.T) {
	tests := []struct {
		name              string
		values            []float64
		capacity          int
		expectedVariation float64
		expectedSize      int
	}{
		{
			name:              "single value returns max variation",
			values:            []float64{5.0},
			capacity:          3,
			expectedVariation: 1.0,
			expectedSize:      1,
		},
		{
			name:              "zeros are ignored",
			values:            []float64{5.0, 0.0, 10.0},
			capacity:          3,
			expectedVariation: 0.3333,
			expectedSize:      2,
		},
		{
			name:              "window fills up to max capacity",
			values:            []float64{5.0, 10.0, 15.0, 20.0},
			capacity:          3,
			expectedVariation: 0.2722,
			expectedSize:      3,
		},
		{
			name:              "identical values give zero variation",
			values:            []float64{10.0, 10.0, 10.0},
			capacity:          3,
			expectedVariation: 0.0,
			expectedSize:      3,
		},
		{
			name:              "zero mean returns max variation",
			values:            []float64{1.0, -1.0},
			capacity:          3,
			expectedVariation: 1.0,
			expectedSize:      2,
		},
		{
			name:              "leading zeros are ignored",
			values:            []float64{0.0, 0.0, 5.0, 10.0},
			capacity:          3,
			expectedVariation: 0.3333,
			expectedSize:      2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newVariationWindow(tc.capacity)
			var variation float64
			for _, v := range tc.values {
				variation = w.add(v)
			}

			assert.InDelta(t, tc.expectedVariation, variation, 0.0001)
			assert.Equal(t, tc.expectedSize, w.size)
		})
	}
}

func TestVariationWindowSliding(t *testing.T) {
	w := newVariationWindow(3)

	w.add(10.0)
	w.add(20.0)
	cv := w.add(30.0)
	assert.InDelta(t, 0.4082, cv, 0.0001)
	cv = w.add(40.0)
	assert.InDelta(t, 0.2722, cv, 0.0001)
	cv = w.add(0.0)
	assert.InDelta(t, 0.2722, cv, 0.0001)
	assert.Equal(t, 3, w.size)
}

func TestCovarianceWindow(t *testing.T) {
	tests := []struct {
		name                string
		capacity            uint
		xValues             []float64
		yValues             []float64
		expectedCorrelation float64
		expectedSize        int
	}{
		{
			name:                "single pair returns zero",
			capacity:            3,
			xValues:             []float64{1.0},
			yValues:             []float64{2.0},
			expectedCorrelation: 0.0,
			expectedSize:        1,
		},
		{
			name:                "perfect positive correlation",
			capacity:            3,
			xValues:             []float64{1.0, 2.0, 3.0},
			yValues:             []float64{10.0, 20.0, 30.0},
			expectedCorrelation: 1.0,
			expectedSize:        3,
		},
		{
			name:                "perfect negative correlation",
			capacity:            3,
			xValues:             []float64{1.0, 2.0, 3.0},
			yValues:             []float64{30.0, 20.0, 10.0},
			expectedCorrelation: -1.0,
			expectedSize:        3,
		},
		{
			name:                "weak correlation returns zero",
			capacity:            3,
			xValues:             []float64{1.0, 2.0, 1.5},
			yValues:             []float64{10.0, 11.0, 9.0},
			expectedCorrelation: 0.0,
			expectedSize:        3,
		},
		{
			name:                "low variation returns zero",
			capacity:            3,
			xValues:             []float64{100.0, 100.1, 100.05},
			yValues:             []float64{200.0, 200.1, 200.05},
			expectedCorrelation: 0.0,
			expectedSize:        3,
		},
		{
			name:                "sliding window",
			capacity:            3,
			xValues:             []float64{1.0, 2.0, 3.0, 4.0},
			yValues:             []float64{10.0, 20.0, 30.0, 40.0},
			expectedCorrelation: 1.0,
			expectedSize:        3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newCovarianceWindow(tc.capacity)
			var correlation float64
			for i := range tc.xValues {
				correlation = w.add(tc.xValues[i], tc.yValues[i])
			}

			assert.InDelta(t, tc.expectedCorrelation, correlation, 0.0001)
			assert.Equal(t, tc.expectedSize, w.xSamples.size)
		})
	}
}

func TestCovarianceWindowSliding(t *testing.T) {
	w := newCovarianceWindow(3)

	corr := w.add(1.0, 10.0)
	assert.InDelta(t, 0.0, corr, 0.0001)
	corr = w.add(2.0, 20.0)
	assert.InDelta(t, 1.0, corr, 0.0001)
	corr = w.add(3.0, 30.0)
	assert.InDelta(t, 1.0, corr, 0.0001)
	corr = w.add(4.0, 40.0)
	assert.InDelta(t, 1.0, corr, 0.0001)
	assert.Equal(t, 3, w.xSamples.size)
}
