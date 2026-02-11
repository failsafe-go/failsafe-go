package util

import (
	"math/rand"
	"runtime"
	"testing"
	"time"
)

// TestMemoryReuse_WarmupVsSteadyState demonstrates the memory reuse optimization
func TestMemoryReuse_WarmupVsSteadyState(t *testing.T) {
	const windowSize = 100
	qw := NewQuantileWindow(0.9, time.Duration(windowSize)*time.Second)
	rand.Seed(42)
	baseTime := time.Now()

	// Phase 1: Warmup - Fill the window (should allocate)
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	for i := 0; i < windowSize; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		qw.AddWithTime(rand.Float64()*1000, timestamp)
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	warmupAllocs := m2.Mallocs - m1.Mallocs
	warmupBytes := m2.TotalAlloc - m1.TotalAlloc

	// Phase 2: Steady State - Add same number of entries (should reuse, not allocate)
	runtime.GC()
	var m3 runtime.MemStats
	runtime.ReadMemStats(&m3)

	for i := 0; i < windowSize; i++ {
		timestamp := baseTime.Add(time.Duration(windowSize+i) * time.Second)
		qw.AddWithTime(rand.Float64()*1000, timestamp)
	}

	runtime.GC()
	var m4 runtime.MemStats
	runtime.ReadMemStats(&m4)
	steadyAllocs := m4.Mallocs - m3.Mallocs
	steadyBytes := m4.TotalAlloc - m3.TotalAlloc

	t.Logf("\n=== Memory Reuse Demonstration ===")
	t.Logf("Window size: %d samples", windowSize)
	t.Logf("Node size: 48 bytes (with int64 optimization)")
	t.Logf("")
	t.Logf("Phase 1 - WARMUP (filling window):")
	t.Logf("  Operations:  %d adds", windowSize)
	t.Logf("  Allocations: %d allocs", warmupAllocs)
	t.Logf("  Memory:      %d bytes (~%.2f KB)", warmupBytes, float64(warmupBytes)/1024)
	t.Logf("  Per-op:      ~%.0f bytes/op, ~%.2f allocs/op", float64(warmupBytes)/float64(windowSize), float64(warmupAllocs)/float64(windowSize))
	t.Logf("")
	t.Logf("Phase 2 - STEADY STATE (reusing nodes):")
	t.Logf("  Operations:  %d adds", windowSize)
	t.Logf("  Allocations: %d allocs  ✨ (%.1f%% reduction!)", steadyAllocs, 100.0*(float64(warmupAllocs-steadyAllocs)/float64(warmupAllocs)))
	t.Logf("  Memory:      %d bytes (~%.2f KB)  ✨ (%.1f%% reduction!)", steadyBytes, float64(steadyBytes)/1024, 100.0*(float64(warmupBytes-steadyBytes)/float64(warmupBytes)))
	t.Logf("  Per-op:      ~%.0f bytes/op, ~%.2f allocs/op", float64(steadyBytes)/float64(windowSize), float64(steadyAllocs)/float64(windowSize))
	t.Logf("")
	t.Logf("Result: Node reuse eliminates ~%.0f%% of allocations in steady state!", 100.0*(float64(warmupAllocs-steadyAllocs)/float64(warmupAllocs)))
}

// BenchmarkMemoryReuse_Phases benchmarks warmup vs steady state separately
func BenchmarkMemoryReuse_Phases(b *testing.B) {
	b.Run("Warmup", func(b *testing.B) {
		rand.Seed(42)
		baseTime := time.Now()
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Each iteration creates a fresh window (warmup phase)
			qw := NewQuantileWindow(0.9, 100*time.Second)
			for j := 0; j < 100; j++ {
				timestamp := baseTime.Add(time.Duration(j) * time.Second)
				qw.AddWithTime(rand.Float64()*1000, timestamp)
			}
		}
	})

	b.Run("SteadyState", func(b *testing.B) {
		rand.Seed(42)
		baseTime := time.Now()

		// Pre-fill window (warmup)
		qw := NewQuantileWindow(0.9, 100*time.Second)
		for j := 0; j < 100; j++ {
			timestamp := baseTime.Add(time.Duration(j) * time.Second)
			qw.AddWithTime(rand.Float64()*1000, timestamp)
		}

		b.ReportAllocs()
		b.ResetTimer()

		// Benchmark steady state (reusing nodes)
		for i := 0; i < b.N; i++ {
			timestamp := baseTime.Add(time.Duration(100+i) * time.Second)
			qw.AddWithTime(rand.Float64()*1000, timestamp)
		}
	})
}
