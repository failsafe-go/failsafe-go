package util

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestRollingSum(t *testing.T) {
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
			w := NewRollingSum(tc.capacity)
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
}

func TestRollingSumSliding(t *testing.T) {
	w := NewRollingSum(3)

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
}

func TestCorrelationWindowSliding(t *testing.T) {
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
}

func TestUsageWindow(t *testing.T) {
	clock := testutil.NewTestClock(900)

	// Given 4 buckets representing 1 second each
	window := NewUsageWindow(4, 4*time.Second, clock)
	assert.Equal(t, int64(0), window.TotalUsage())
	assert.Equal(t, uint32(0), window.Samples())

	// Record into bucket 1
	recordUsage(window, []int64{100, 200, 150, 300, 250}) // currentTime = 0
	assert.Equal(t, int64(0), window.headTime)
	assert.Equal(t, int64(1000), window.TotalUsage())
	assert.Equal(t, uint32(5), window.Samples())

	// Record into bucket 2
	clock.SetTime(1000)
	recordUsage(window, []int64{400, 500})
	assert.Equal(t, int64(1), window.headTime)
	assert.Equal(t, int64(1900), window.TotalUsage()) // 1000 + 900
	assert.Equal(t, uint32(7), window.Samples())

	// Record into bucket 3
	clock.SetTime(2500)
	recordUsage(window, []int64{600, 700, 800})
	assert.Equal(t, int64(2), window.headTime)
	assert.Equal(t, int64(4000), window.TotalUsage()) // 1900 + 2100
	assert.Equal(t, uint32(10), window.Samples())

	// Record into bucket 4
	clock.SetTime(3100)
	recordUsage(window, []int64{50, 75})
	assert.Equal(t, int64(3), window.headTime)
	assert.Equal(t, int64(4125), window.TotalUsage()) // 4000 + 125
	assert.Equal(t, uint32(12), window.Samples())

	// Record into bucket 2, skipping bucket 1
	clock.SetTime(5400)
	recordUsage(window, []int64{500})
	assert.Equal(t, int64(5), window.headTime)

	// Assert bucket 1 was skipped and reset based on its previous start time
	bucket1 := window.buckets[0]
	assert.Equal(t, int64(0), bucket1.totalUsage)
	assert.Equal(t, uint32(0), bucket1.samples)

	// Should have lost bucket 1's data (1000) and gained new data (500)
	assert.Equal(t, int64(2725), window.TotalUsage()) // 4125 - 1000 - 900 + 500
	assert.Equal(t, uint32(6), window.Samples())      // 12 - 5 - 2 + 1

	// Record into bucket 4, skipping bucket 3
	clock.SetTime(7300)
	recordUsage(window, []int64{300, 400})
	assert.Equal(t, int64(7), window.headTime)

	// Assert bucket 3 was skipped and reset
	bucket3 := window.buckets[2]
	assert.Equal(t, int64(0), bucket3.totalUsage)
	assert.Equal(t, uint32(0), bucket3.samples)

	// Should have lost bucket 3's data (2100) and gained new data (700)
	assert.Equal(t, int64(1200), window.TotalUsage()) // 2725 - 2100 - 125 + 700
	assert.Equal(t, uint32(3), window.Samples())      // 6 - 3 - 2 + 2

	// Skip all buckets by jumping way ahead in time
	clock.SetTime(22500)
	window.currentBucket() // Force bucket calculation
	assert.Equal(t, int64(22), window.headTime)

	// All buckets should be reset
	for _, b := range window.buckets {
		assert.Equal(t, int64(0), b.totalUsage)
		assert.Equal(t, uint32(0), b.samples)
	}
	assert.Equal(t, int64(0), window.TotalUsage())
	assert.Equal(t, uint32(0), window.Samples())

	// Record into bucket 2 after reset
	clock.SetTime(23100)
	recordUsage(window, []int64{500, 750, 250})
	assert.Equal(t, int64(23), window.headTime)
	assert.Equal(t, int64(1500), window.TotalUsage())
	assert.Equal(t, uint32(3), window.Samples())
}

func recordUsage(window *UsageWindow, usages []int64) {
	for _, usage := range usages {
		window.RecordUsage(usage)
	}
}
