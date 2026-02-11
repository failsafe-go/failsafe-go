package util

import (
	"math/rand"
	"testing"

	"github.com/influxdata/tdigest"
)

// BenchmarkComparison_TDigest benchmarks TDigest performance
func BenchmarkComparison_TDigest(b *testing.B) {
	td := tdigest.NewWithCompression(100)
	rand.Seed(42)

	// Pre-fill
	for i := 0; i < 1000; i++ {
		td.Add(rand.Float64()*1000, 1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		td.Add(rand.Float64()*1000, 1)
		_ = td.Quantile(0.9)
	}
}

// BenchmarkComparison_QuantileWindow benchmarks QuantileWindow performance
func BenchmarkComparison_QuantileWindow(b *testing.B) {
	qw := NewQuantileWindow(0.9, 1000)
	rand.Seed(42)

	// Pre-fill
	for i := 0; i < 1000; i++ {
		qw.Add(rand.Float64() * 1000)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qw.Add(rand.Float64() * 1000)
		_ = qw.Value()
	}
}

// BenchmarkComparison_MovingQuantile benchmarks the existing MovingQuantile (EMA-based)
func BenchmarkComparison_MovingQuantile(b *testing.B) {
	mq := NewMovingQuantile(0.9, 0.01, 100)
	rand.Seed(42)

	// Pre-fill
	for i := 0; i < 1000; i++ {
		mq.Add(rand.Float64() * 1000)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mq.Add(rand.Float64() * 1000)
	}
}

// BenchmarkComparison_StableWorkload compares all three with stable latencies
func BenchmarkComparison_StableWorkload_TDigest(b *testing.B) {
	td := tdigest.NewWithCompression(100)
	rand.Seed(42)

	// Pre-fill with stable values around 100ms
	for i := 0; i < 1000; i++ {
		td.Add(100+rand.Float64()*10, 1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		td.Add(100+rand.Float64()*10, 1)
		_ = td.Quantile(0.9)
	}
}

func BenchmarkComparison_StableWorkload_QuantileWindow(b *testing.B) {
	qw := NewQuantileWindow(0.9, 1000)
	rand.Seed(42)

	// Pre-fill with stable values around 100ms
	for i := 0; i < 1000; i++ {
		qw.Add(100 + rand.Float64()*10)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qw.Add(100 + rand.Float64()*10)
		_ = qw.Value()
	}
}

func BenchmarkComparison_StableWorkload_MovingQuantile(b *testing.B) {
	mq := NewMovingQuantile(0.9, 0.01, 100)
	rand.Seed(42)

	// Pre-fill with stable values around 100ms
	for i := 0; i < 1000; i++ {
		mq.Add(100 + rand.Float64()*10)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mq.Add(100 + rand.Float64()*10)
	}
}

// TestComparison_Accuracy compares accuracy across all three approaches
func TestComparison_Accuracy(t *testing.T) {
	windowSize := 1000
	samples := 10000
	rand.Seed(42)

	// Generate samples
	values := make([]float64, samples)
	for i := 0; i < samples; i++ {
		values[i] = rand.Float64() * 1000
	}

	// Test each approach
	qw := NewQuantileWindow(0.9, windowSize)
	mq := NewMovingQuantile(0.9, 0.01, 100)
	td := tdigest.NewWithCompression(100)

	// Track results over time
	var qwResults, mqResults, tdResults []float64

	for i, v := range values {
		qw.Add(v)
		mq.Add(v)
		td.Add(v, 1)

		// Sample results every 100 values after window is full
		if i%100 == 99 && i >= windowSize {
			qwResults = append(qwResults, qw.Value())
			mqResults = append(mqResults, mq.Value())
			tdResults = append(tdResults, td.Quantile(0.9))
		}
	}

	t.Logf("Sampled %d quantile values after warmup", len(qwResults))
	t.Logf("QuantileWindow (exact): samples around %.2f", qwResults[len(qwResults)-1])
	t.Logf("MovingQuantile (EMA): samples around %.2f", mqResults[len(mqResults)-1])
	t.Logf("TDigest (approx): samples around %.2f", tdResults[len(tdResults)-1])
}

// BenchmarkComparison_WindowSizes tests performance with different window sizes
func BenchmarkComparison_WindowSize100_QuantileWindow(b *testing.B) {
	qw := NewQuantileWindow(0.9, 100)
	rand.Seed(42)
	for i := 0; i < 100; i++ {
		qw.Add(rand.Float64() * 1000)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qw.Add(rand.Float64() * 1000)
		_ = qw.Value()
	}
}

func BenchmarkComparison_WindowSize1000_QuantileWindow(b *testing.B) {
	qw := NewQuantileWindow(0.9, 1000)
	rand.Seed(42)
	for i := 0; i < 1000; i++ {
		qw.Add(rand.Float64() * 1000)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qw.Add(rand.Float64() * 1000)
		_ = qw.Value()
	}
}

func BenchmarkComparison_WindowSize5000_QuantileWindow(b *testing.B) {
	qw := NewQuantileWindow(0.9, 5000)
	rand.Seed(42)
	for i := 0; i < 5000; i++ {
		qw.Add(rand.Float64() * 1000)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		qw.Add(rand.Float64() * 1000)
		_ = qw.Value()
	}
}
