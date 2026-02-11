package util

import (
	"testing"
	"time"
)

// TestQuantileWindow_TimeBasedExpiration verifies that entries expire based on time
func TestQuantileWindow_TimeBasedExpiration(t *testing.T) {
	qw := NewQuantileWindow(0.9, 1*time.Minute)
	baseTime := time.Now()

	// Add 10 samples at t=0
	for i := 1; i <= 10; i++ {
		qw.AddWithTime(float64(i*10), baseTime)
	}

	if qw.Size() != 10 {
		t.Errorf("Expected 10 samples, got %d", qw.Size())
	}

	// Add a sample at t=30s (all previous samples still valid)
	qw.AddWithTime(100, baseTime.Add(30*time.Second))
	if qw.Size() != 11 {
		t.Errorf("At t=30s: Expected 11 samples, got %d", qw.Size())
	}

	// Add a sample at t=61s (samples at t=0 should expire)
	qw.AddWithTime(110, baseTime.Add(61*time.Second))
	if qw.Size() != 2 { // Only samples at t=30s and t=61s remain
		t.Errorf("At t=61s: Expected 2 samples, got %d", qw.Size())
	}

	// Add a sample at t=120s (sample at t=30s should expire)
	qw.AddWithTime(120, baseTime.Add(120*time.Second))
	if qw.Size() != 2 { // Only samples at t=61s and t=120s remain
		t.Errorf("At t=120s: Expected 2 samples, got %d", qw.Size())
	}
}

// TestQuantileWindow_TimeBasedWithVaryingLoad demonstrates memory adapting to load
func TestQuantileWindow_TimeBasedWithVaryingLoad(t *testing.T) {
	qw := NewQuantileWindow(0.9, 1*time.Minute)
	baseTime := time.Now()

	// Low load: 1 sample per second for 60 seconds = 60 samples
	for i := 0; i < 60; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		qw.AddWithTime(float64(i), timestamp)
	}

	if qw.Size() != 60 {
		t.Errorf("Low load: Expected 60 samples, got %d", qw.Size())
	}

	// High load: 10 samples per second for next 60 seconds
	for i := 0; i < 600; i++ {
		timestamp := baseTime.Add(60*time.Second + time.Duration(i)*100*time.Millisecond)
		qw.AddWithTime(float64(100+i), timestamp)
	}

	// Should have ~600 samples (last minute at 10/sec)
	// First 60 samples expired
	if qw.Size() < 590 || qw.Size() > 610 {
		t.Errorf("High load: Expected ~600 samples, got %d", qw.Size())
	}
}

// TestQuantileWindow_TimeBasedQuantileAccuracy verifies quantile correctness with time-based windows
func TestQuantileWindow_TimeBasedQuantileAccuracy(t *testing.T) {
	qw := NewQuantileWindow(0.9, 1*time.Minute)
	baseTime := time.Now()

	// Add 100 samples over 50 seconds (within window)
	for i := 1; i <= 100; i++ {
		timestamp := baseTime.Add(time.Duration(i) * 500 * time.Millisecond)
		qw.AddWithTime(float64(i), timestamp)
	}

	// p90 of [1..100] should be 90
	p90 := qw.Value()
	expected := 90.0
	if p90 != expected {
		t.Errorf("Expected p90 = %.0f, got %.0f", expected, p90)
	}

	// All samples are still within 1 minute window
	if qw.Size() != 100 {
		t.Errorf("Expected 100 samples in window, got %d", qw.Size())
	}
}

// TestQuantileWindow_RealWorldScenario simulates aggregated latency samples
func TestQuantileWindow_RealWorldScenario(t *testing.T) {
	// Scenario: Aggregate latency samples every 1 second, keep 1 minute of history
	qw := NewQuantileWindow(0.9, 1*time.Minute)
	baseTime := time.Now()

	// Simulate 120 seconds of operation (aggregating 1 sample/second)
	for second := 0; second < 120; second++ {
		// Simulate varying latency
		latency := 50.0 + float64(second%30) // Oscillates between 50-80ms
		timestamp := baseTime.Add(time.Duration(second) * time.Second)
		qw.AddWithTime(latency, timestamp)
	}

	// Should have ~60 samples (last 60 seconds) - might be 60 or 61 depending on boundary
	if qw.Size() < 60 || qw.Size() > 61 {
		t.Errorf("Expected ~60 samples in 1-minute window, got %d", qw.Size())
	}

	// p90 should be reasonable
	p90 := qw.Value()
	if p90 < 50 || p90 > 80 {
		t.Errorf("Expected p90 in range [50, 80], got %.0f", p90)
	}

	t.Logf("After 120 seconds with 1 sample/sec: window has %d samples, p90 = %.2f", qw.Size(), p90)
}

// BenchmarkQuantileWindow_TimeBasedRealistic benchmarks with realistic aggregated samples
func BenchmarkQuantileWindow_TimeBasedRealistic(b *testing.B) {
	qw := NewQuantileWindow(0.9, 1*time.Minute)
	baseTime := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate 1 sample per second
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		qw.AddWithTime(float64(50+i%30), timestamp)
		_ = qw.Value()
	}
}
