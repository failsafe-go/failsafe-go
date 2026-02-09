package util

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Asserts that Value and Count return zero before any samples are added.
func TestMovingQuantile_InitialState(t *testing.T) {
	mq := NewMovingQuantile(0.95, 0.01, 30)
	assert.Equal(t, 0.0, mq.Value())
	assert.Equal(t, 0, mq.Count())
}

// Asserts that the first sample is used as the initial estimate.
func TestMovingQuantile_FirstSample(t *testing.T) {
	mq := NewMovingQuantile(0.95, 0.01, 30)

	result := mq.Add(123.45)

	assert.Equal(t, 123.45, result)
	assert.Equal(t, 123.45, mq.Value())
	assert.Equal(t, 1, mq.Count())
}

// Asserts that the quantile estimate converges to known quantiles for a uniform distribution.
func TestMovingQuantile_ConvergesUniform(t *testing.T) {
	tests := []struct {
		name      string
		quantile  float64
		expected  float64
		tolerance float64
	}{
		{"p50", 0.50, 500, 50},
		{"p95", 0.95, 950, 100},
		{"p99", 0.99, 990, 250},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mq := NewMovingQuantile(tc.quantile, 0.01, 30)
			rng := rand.New(rand.NewSource(42))

			// Feed uniform [0, 1000) samples
			for i := 0; i < 10000; i++ {
				mq.Add(rng.Float64() * 1000)
			}

			assert.InDelta(t, tc.expected, mq.Value(), tc.tolerance, "quantile estimate should converge")
		})
	}
}

// Asserts that the quantile estimate converges to known quantiles for a normal distribution.
func TestMovingQuantile_ConvergesNormal(t *testing.T) {
	tests := []struct {
		name     string
		quantile float64
		mean     float64
		stddev   float64
		expected float64
	}{
		{"p50 normal", 0.50, 100, 10, 100},
		{"p95 normal", 0.95, 100, 10, 116.45},
		{"p99 normal", 0.99, 100, 10, 123.26},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mq := NewMovingQuantile(tc.quantile, 0.01, 30)
			rng := rand.New(rand.NewSource(42))

			for i := 0; i < 20000; i++ {
				sample := rng.NormFloat64()*tc.stddev + tc.mean
				mq.Add(sample)
			}

			assert.InDelta(t, tc.expected, mq.Value(), tc.stddev*0.5, "quantile estimate should converge for normal distribution")
		})
	}
}

// Asserts that a constant input produces a constant quantile estimate.
func TestMovingQuantile_ConstantValue(t *testing.T) {
	mq := NewMovingQuantile(0.95, 0.01, 30)

	for i := 0; i < 100; i++ {
		mq.Add(42)
	}

	assert.Equal(t, 42.0, mq.Value(), "constant input should return constant value")
}

// Asserts that Reset clears the value and count.
func TestMovingQuantile_Reset(t *testing.T) {
	mq := NewMovingQuantile(0.95, 0.01, 30)

	for i := 0; i < 100; i++ {
		mq.Add(float64(i))
	}

	assert.NotEqual(t, 0.0, mq.Value())
	assert.Equal(t, 100, mq.Count())

	mq.Reset()
	assert.Equal(t, 0.0, mq.Value())
	assert.Equal(t, 0, mq.Count())
}

// Asserts that the estimate adapts when the underlying distribution shifts.
func TestMovingQuantile_AdaptsToShift(t *testing.T) {
	mq := NewMovingQuantile(0.50, 0.01, 30)
	rng := rand.New(rand.NewSource(42))

	// Phase 1: values around 100
	for i := 0; i < 5000; i++ {
		mq.Add(rng.Float64()*20 + 90) // [90, 110)
	}
	assert.InDelta(t, 100, mq.Value(), 15, "should converge to p50 of first distribution")

	// Phase 2: shift to values around 500
	for i := 0; i < 10000; i++ {
		mq.Add(rng.Float64()*20 + 490) // [490, 510)
	}
	assert.InDelta(t, 500, mq.Value(), 15, "should adapt to shifted distribution")
}

// Asserts that the estimate works with duration-scale values simulating latency tracking.
func TestMovingQuantile_DurationValues(t *testing.T) {
	// Simulate latency tracking with time.Duration stored as float64
	mq := NewMovingQuantile(0.95, 0.01, 30)
	rng := rand.New(rand.NewSource(42))

	// Simulate latencies: mostly 10-20ms, with occasional spikes to 100ms
	for i := 0; i < 10000; i++ {
		var latencyMs float64
		if rng.Float64() >= 0.05 {
			latencyMs = 10 + rng.Float64()*10 // normal: 10-20ms
		} else {
			latencyMs = 100 + rng.Float64()*20 // spike: 100-120ms
		}
		mq.Add(latencyMs * 1e6) // store as nanoseconds
	}

	estimateMs := mq.Value() / 1e6
	// p95 should be above the normal range but not necessarily at the spike level
	assert.True(t, estimateMs > 15, "p95 should be above median of normal range, got %f", estimateMs)
	assert.True(t, !math.IsNaN(estimateMs) && !math.IsInf(estimateMs, 0), "estimate should be finite")
}
