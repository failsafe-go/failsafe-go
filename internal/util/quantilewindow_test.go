package util

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

func TestQuantileWindow_Basic(t *testing.T) {
	qw := NewQuantileWindow(0.5, 10) // p50 (median) with window size 10

	// Add samples
	values := []float64{1, 2, 3, 4, 5}
	for _, v := range values {
		qw.Add(v)
	}

	// Median of [1,2,3,4,5] should be 3
	median := qw.Value()
	if median != 3 {
		t.Errorf("Expected median 3, got %f", median)
	}

	if qw.Size() != 5 {
		t.Errorf("Expected size 5, got %d", qw.Size())
	}
}

func TestQuantileWindow_P90(t *testing.T) {
	qw := NewQuantileWindow(0.9, 10) // p90

	// Add 10 values: 1, 2, 3, ..., 10
	for i := 1; i <= 10; i++ {
		qw.Add(float64(i))
	}

	// p90 of [1..10] = 9th value (0-indexed pos 8) = 9
	p90 := qw.Value()
	if p90 != 9 {
		t.Errorf("Expected p90 = 9, got %f", p90)
	}
}

func TestQuantileWindow_P70(t *testing.T) {
	qw := NewQuantileWindow(0.7, 10) // p70

	// Add 10 values: 1, 2, 3, ..., 10
	for i := 1; i <= 10; i++ {
		qw.Add(float64(i))
	}

	// p70 of [1..10] = 7th value (0-indexed pos 6) = 7
	p70 := qw.Value()
	if p70 != 7 {
		t.Errorf("Expected p70 = 7, got %f", p70)
	}
}

func TestQuantileWindow_SlidingWindow(t *testing.T) {
	qw := NewQuantileWindow(0.5, 5) // Median with window size 5

	// Add initial window: [1,2,3,4,5]
	for i := 1; i <= 5; i++ {
		qw.Add(float64(i))
	}
	if qw.Value() != 3 {
		t.Errorf("Initial median should be 3, got %f", qw.Value())
	}

	// Add 6, window becomes [2,3,4,5,6], median = 4
	qw.Add(6)
	if qw.Value() != 4 {
		t.Errorf("After adding 6, median should be 4, got %f", qw.Value())
	}

	// Add 7, window becomes [3,4,5,6,7], median = 5
	qw.Add(7)
	if qw.Value() != 5 {
		t.Errorf("After adding 7, median should be 5, got %f", qw.Value())
	}

	// Verify size stays at max
	if qw.Size() != 5 {
		t.Errorf("Window size should be 5, got %d", qw.Size())
	}
}

func TestQuantileWindow_UnsortedInput(t *testing.T) {
	qw := NewQuantileWindow(0.5, 10)

	// Add values in random order
	values := []float64{5, 1, 9, 3, 7, 2, 8, 4, 6, 10}
	for _, v := range values {
		qw.Add(v)
	}

	// Median of [1..10] should be 5.5, but with 0-indexed position it's 5
	median := qw.Value()
	expected := 5.0 // Position 4 (0-indexed) in sorted [1,2,3,4,5,6,7,8,9,10]
	if median != expected {
		t.Errorf("Expected median %f, got %f", expected, median)
	}
}

func TestQuantileWindow_DuplicateValues(t *testing.T) {
	qw := NewQuantileWindow(0.5, 10)

	// Add duplicate values
	values := []float64{5, 5, 5, 5, 5}
	for _, v := range values {
		qw.Add(v)
	}

	if qw.Value() != 5 {
		t.Errorf("Expected median 5 with all duplicates, got %f", qw.Value())
	}
}

func TestQuantileWindow_Reset(t *testing.T) {
	qw := NewQuantileWindow(0.5, 10)

	// Add values
	for i := 1; i <= 5; i++ {
		qw.Add(float64(i))
	}

	// Reset
	qw.Reset()

	if qw.Size() != 0 {
		t.Errorf("After reset, size should be 0, got %d", qw.Size())
	}

	if qw.Value() != 0 {
		t.Errorf("After reset, value should be 0, got %f", qw.Value())
	}

	// Add new values after reset
	qw.Add(10)
	qw.Add(20)
	qw.Add(30)

	if qw.Size() != 3 {
		t.Errorf("After reset and adding 3 values, size should be 3, got %d", qw.Size())
	}
}

func TestQuantileWindow_EmptyWindow(t *testing.T) {
	qw := NewQuantileWindow(0.9, 10)

	if qw.Value() != 0 {
		t.Errorf("Empty window should return 0, got %f", qw.Value())
	}

	if qw.Size() != 0 {
		t.Errorf("Empty window size should be 0, got %d", qw.Size())
	}
}

func TestQuantileWindow_SingleValue(t *testing.T) {
	qw := NewQuantileWindow(0.9, 10)

	qw.Add(42)

	if qw.Value() != 42 {
		t.Errorf("Single value window should return that value, got %f", qw.Value())
	}

	if qw.Size() != 1 {
		t.Errorf("Single value window size should be 1, got %d", qw.Size())
	}
}

func TestQuantileWindow_VariousQuantiles(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		quantile float64
		expected float64
	}{
		{0.0, 1},   // min (position 0)
		{0.25, 3},  // p25 (position floor(9*0.25) = 2, value at index 2 = 3)
		{0.5, 5},   // p50 (position floor(9*0.5) = 4, value at index 4 = 5)
		{0.75, 7},  // p75 (position floor(9*0.75) = 6, value at index 6 = 7)
		{0.9, 9},   // p90 (position floor(9*0.9) = 8, value at index 8 = 9)
		{0.95, 9},  // p95 (position floor(9*0.95) = 8, value at index 8 = 9)
		{0.99, 9},  // p99 (position floor(9*0.99) = 8, value at index 8 = 9)
	}

	for _, tt := range tests {
		qw := NewQuantileWindow(tt.quantile, 10)
		for _, v := range values {
			qw.Add(v)
		}

		result := qw.Value()
		if result != tt.expected {
			t.Errorf("Quantile %.2f: expected %f, got %f", tt.quantile, tt.expected, result)
		}
	}
}

// TestQuantileWindow_StressTest adds many values and verifies correctness
func TestQuantileWindow_StressTest(t *testing.T) {
	windowSize := 100
	numSamples := 1000
	qw := NewQuantileWindow(0.9, windowSize)

	rand.Seed(42)
	values := make([]float64, numSamples)
	for i := 0; i < numSamples; i++ {
		values[i] = rand.Float64() * 1000
	}

	for i, v := range values {
		qw.Add(v)

		// Verify correctness every 100 samples
		if i%100 == 99 && i >= windowSize-1 {
			// Get current window
			windowStart := max(0, i-windowSize+1)
			currentWindow := values[windowStart : i+1]

			// Calculate expected p90
			sorted := make([]float64, len(currentWindow))
			copy(sorted, currentWindow)
			sort.Float64s(sorted)
			expectedPos := int(float64(len(sorted)-1) * 0.9)
			expected := sorted[expectedPos]

			result := qw.Value()
			if math.Abs(result-expected) > 0.001 {
				t.Errorf("At sample %d: expected p90 = %f, got %f", i, expected, result)
			}
		}
	}
}

// TestQuantileWindow_ConsistencyWithSorting verifies that the quantile window
// produces the same results as sorting the window
func TestQuantileWindow_ConsistencyWithSorting(t *testing.T) {
	windowSize := 50
	qw := NewQuantileWindow(0.7, windowSize)

	rand.Seed(123)
	window := make([]float64, 0, windowSize)

	for i := 0; i < 200; i++ {
		value := rand.Float64() * 100
		qw.Add(value)

		// Maintain a reference window
		window = append(window, value)
		if len(window) > windowSize {
			window = window[1:]
		}

		// Calculate expected p70 from sorted window
		if len(window) > 0 {
			sorted := make([]float64, len(window))
			copy(sorted, window)
			sort.Float64s(sorted)
			expectedPos := int(float64(len(sorted)-1) * 0.7)
			expected := sorted[expectedPos]

			result := qw.Value()
			if result != expected {
				t.Errorf("At iteration %d: expected p70 = %f, got %f", i, expected, result)
				t.Errorf("Window: %v", window)
				t.Errorf("Sorted: %v", sorted)
			}
		}
	}
}

// BenchmarkQuantileWindow_Add benchmarks the Add operation
func BenchmarkQuantileWindow_Add(b *testing.B) {
	qw := NewQuantileWindow(0.9, 1000)
	rand.Seed(42)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qw.Add(rand.Float64() * 1000)
	}
}

// BenchmarkQuantileWindow_AddWithQuery benchmarks Add + Value operations
func BenchmarkQuantileWindow_AddWithQuery(b *testing.B) {
	qw := NewQuantileWindow(0.9, 1000)
	rand.Seed(42)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qw.Add(rand.Float64() * 1000)
		_ = qw.Value()
	}
}

// BenchmarkQuantileWindow_StableDistribution benchmarks with stable latencies
// (simulates the common case where new values cluster near the quantile)
func BenchmarkQuantileWindow_StableDistribution(b *testing.B) {
	qw := NewQuantileWindow(0.9, 1000)
	rand.Seed(42)

	// Pre-fill window with values around 100ms
	for i := 0; i < 1000; i++ {
		qw.Add(100 + rand.Float64()*10) // Values in range [100, 110]
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// New values cluster near existing quantile
		qw.Add(100 + rand.Float64()*10)
		_ = qw.Value()
	}
}
