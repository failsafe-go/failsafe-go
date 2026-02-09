package util

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMovingSum(t *testing.T) {
	tests := []struct {
		name              string
		values            []float64
		capacity          uint
		expectedVariation float64
		expectedSize      int
		shouldNaN         bool
	}{
		{
			name:         "less than two positive values returns NaN",
			values:       []float64{1.0, -1.0},
			capacity:     3,
			expectedSize: 2,
			shouldNaN:    true,
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
			name:              "leading zeros are ignored",
			values:            []float64{0.0, 0.0, 5.0, 10.0},
			capacity:          3,
			expectedVariation: 0.3333,
			expectedSize:      2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewMovingSum(tc.capacity)
			var cv float64
			for _, v := range tc.values {
				w.Add(v)
				cv, _, _ = w.CalculateCV()
			}

			if tc.shouldNaN {
				assert.True(t, math.IsNaN(cv))
			} else {
				assert.InDelta(t, tc.expectedVariation, cv, 0.0001)
			}
			assert.Equal(t, tc.expectedSize, w.size)
		})
	}

	t.Run("should slide", func(t *testing.T) {
		w := NewMovingSum(3)

		w.Add(10.0)
		w.Add(20.0)
		w.Add(30.0)
		cv, _, _ := w.CalculateCV()
		assert.InDelta(t, 0.4082, cv, 0.0001)
		w.Add(40.0)
		cv, _, _ = w.CalculateCV()
		assert.InDelta(t, 0.2722, cv, 0.0001)
		w.Add(0.0)
		cv, _, _ = w.CalculateCV()
		assert.InDelta(t, 0.2722, cv, 0.0001)
		assert.Equal(t, 3, w.size)
	})
}

func TestCorrelationWindow(t *testing.T) {
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
			name:                "low variation returns zero",
			capacity:            3,
			xValues:             []float64{100.0, 100.1, 100.05},
			yValues:             []float64{200.0, 200.1, 200.05},
			expectedCorrelation: 0.0,
			expectedSize:        3,
		},
		{
			name:                "rolling window",
			capacity:            3,
			xValues:             []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			yValues:             []float64{10.0, 20.0, 30.0, 40.0, 50.0},
			expectedCorrelation: 1.0,
			expectedSize:        3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewCorrelationWindow(tc.capacity, 0)
			var correlation float64
			for i := range tc.xValues {
				correlation, _, _ = w.Add(tc.xValues[i], tc.yValues[i])
			}

			assert.InDelta(t, tc.expectedCorrelation, correlation, 0.0001)
			assert.Equal(t, tc.expectedSize, w.xSamples.size)

			w.Reset()
			require.Equal(t, 0.0, w.corrSumXY)
		})
	}

	t.Run("should slide", func(t *testing.T) {
		w := NewCorrelationWindow(3, 0)

		corr, _, _ := w.Add(1.0, 10.0)
		assert.InDelta(t, 0.0, corr, 0.0001)
		corr, _, _ = w.Add(2.0, 20.0)
		assert.InDelta(t, 1.0, corr, 0.0001)
		corr, _, _ = w.Add(3.0, 30.0)
		assert.InDelta(t, 1.0, corr, 0.0001)
		corr, _, _ = w.Add(4.0, 40.0)
		assert.InDelta(t, 1.0, corr, 0.0001)
		assert.Equal(t, 3, w.xSamples.size)
	})
}

func TestMaxWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("should slide", func(t *testing.T) {
		w := NewMaxWindow(5 * time.Minute)

		assert.Equal(t, 1030, w.Add(1030, now))
		assert.Equal(t, 1030, w.Add(691, now.Add(30*time.Second)))
		assert.Equal(t, 1030, w.Add(849, now.Add(60*time.Second)))
		assert.Equal(t, 1030, w.Add(836, now.Add(90*time.Second)))
		assert.Equal(t, 1030, w.Add(1028, now.Add(120*time.Second)))
		assert.Equal(t, 1030, w.Add(700, now.Add(150*time.Second)))

		assert.Equal(t, 1028, w.Add(650, now.Add(5*time.Minute+1*time.Second))) // Peak from t=0 expires at t=5m
		assert.Equal(t, 700, w.Add(400, now.Add(7*time.Minute+1*time.Second)))  // Peak from t=2m expires at t=7m
		assert.Equal(t, 400, w.Add(300, now.Add(10*time.Minute+1*time.Second))) // Peak from t=5m expires at t=10m
		assert.Equal(t, 300, w.Add(100, now.Add(12*time.Minute+1*time.Second))) // Peak from t=7m expires at t=12m
	})

	t.Run("should slide and reset", func(t *testing.T) {
		w := NewMaxWindow(5 * time.Minute)

		assert.Equal(t, 700, w.Add(700, now))
		assert.Equal(t, 1000, w.Add(1000, now.Add(1*time.Minute))) // Higher value becomes new max
		assert.Equal(t, 1000, w.Add(600, now.Add(2*time.Minute)))  // Lower value doesn't change max
		assert.Equal(t, 1000, w.Add(500, now.Add(3*time.Minute)))  // Much lower value still doesn't change max
		assert.Equal(t, 800, w.Add(800, now.Add(7*time.Minute)))   // After peak expires, next highest becomes max
		w.Reset()
		assert.Equal(t, 42, w.Add(42, now.Add(10*time.Minute)))
	})

	t.Run("should expire all entries after long delay", func(t *testing.T) {
		w := NewMaxWindow(1 * time.Minute)
		w.Add(1000, now)

		assert.Equal(t, 50, w.Add(50, now.Add(10*time.Minute)))
	})
}
