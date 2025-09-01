package adaptivelimiter

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdaptiveLimiter_Defaults(t *testing.T) {
	t.Run("should initialize limit", func(t *testing.T) {
		limiter := NewBuilder[any]().Build()
		assert.Equal(t, 20, limiter.Limit())
	})

	t.Run("should initialize specific limit", func(t *testing.T) {
		limiter := NewBuilder[any]().WithLimits(1, 10, 5).Build()
		assert.Equal(t, 5, limiter.Limit())
	})

	t.Run("should initialize empty", func(t *testing.T) {
		limiter := NewBuilder[any]().Build().(*adaptiveLimiter[any])
		assert.Equal(t, 0.0, limiter.recentRTT.Count())
		assert.Equal(t, 0.0, limiter.baselineRTT.Value())
		assert.Equal(t, 0, limiter.Inflight())
		assert.Equal(t, 0, limiter.Queued())
	})
}

func TestAdaptiveLimiter_AcquirePermitAndRecord(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()

	permit, err := limiter.AcquirePermit(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, limiter.Inflight())
	permit.Record()
	assert.Equal(t, 0, limiter.Inflight())
}

func TestAdaptiveLimiter_AcquirePermitWithMaxWaitAndRecord(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()

	permit, err := limiter.AcquirePermitWithMaxWait(context.Background(), 0)
	assert.NoError(t, err)
	assert.Equal(t, 1, limiter.Inflight())
	permit.Record()
	assert.Equal(t, 0, limiter.Inflight())
}

func TestAdaptiveLimiter_TryAcquirePermitAndRecord(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()

	permit, ok := limiter.TryAcquirePermit()
	assert.True(t, ok)
	assert.Equal(t, 1, limiter.Inflight())
	_, ok = limiter.TryAcquirePermit()
	assert.False(t, ok)
	permit.Record()
	assert.Equal(t, 0, limiter.Inflight())
}

func TestAdaptiveLimiter_CanAcquirePermit(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()

	assert.True(t, limiter.CanAcquirePermit())
	permit, err := limiter.AcquirePermit(context.Background())
	assert.NoError(t, err)
	assert.False(t, limiter.CanAcquirePermit())
	permit.Record()
	assert.True(t, limiter.CanAcquirePermit())
}

// Asserts that queued executions are counted.
func TestAdaptiveLimiter_Queued(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()
	permit, err := limiter.AcquirePermit(context.Background())
	assert.NoError(t, err)

	acquireAsync(limiter)
	assertQueued(t, limiter, 1)
	permit.Record()
	assert.Equal(t, 0, limiter.Queued())
}

func TestAdaptiveLimiter_record(t *testing.T) {
	createLimiter := func() (*adaptiveLimiter[any], time.Time) {
		limiter := NewBuilder[any]().Build().(*adaptiveLimiter[any])
		now := time.UnixMilli(0)
		limiter.nextUpdateTime = now
		limiter.WithRecentWindow(time.Second, time.Second, 1)
		for i := 0; i < warmupSamples; i++ {
			limiter.baselineRTT.Add(float64(time.Second))
		}
		return limiter, now
	}

	recordFn := func(limiter *adaptiveLimiter[any], startTime time.Time, rtt time.Duration, inflight int) time.Time {
		// Simulate recording a sample after 2 seconds
		now := startTime.Add(2 * time.Second)
		assert.NoError(t, limiter.semaphore.Acquire(context.Background()))
		limiter.record(now, rtt, inflight, false)
		return now
	}

	t.Run("should smooth and filter RTTs", func(t *testing.T) {
		limiter, now := createLimiter()

		now = recordFn(limiter, now, 100*time.Millisecond, 10)
		now = recordFn(limiter, now, 300*time.Millisecond, 10)
		now = recordFn(limiter, now, 200*time.Millisecond, 10)
		now = recordFn(limiter, now, 200*time.Millisecond, 10)
		now = recordFn(limiter, now, 250*time.Millisecond, 10)

		assert.Equal(t, float64(200*time.Millisecond), limiter.medianFilter.Median())
		assert.Equal(t, float64(200*time.Millisecond), limiter.smoothedRecentRTT.Value())
	})

	t.Run("should increase limit", func(t *testing.T) {
		limiter, now := createLimiter()

		now = recordFn(limiter, now, time.Second, 5) // queue size 0
		assert.Equal(t, 21, limiter.Limit())

		now = recordFn(limiter, now, 500*time.Millisecond, 5) // queue size -6
		assert.Equal(t, 22, limiter.Limit())
	})

	t.Run("should decrease limit", func(t *testing.T) {
		limiter, now := createLimiter()

		now = recordFn(limiter, now, 2*time.Second, 10) // queue size 10
		assert.Equal(t, 19, limiter.Limit())

		now = recordFn(limiter, now, 2*time.Second, 10) // queue size 9
		assert.Equal(t, 18, limiter.Limit())
	})

	t.Run("should hold limit", func(t *testing.T) {
		limiter, now := createLimiter()

		now = recordFn(limiter, now, 1300*time.Millisecond, 10) // queue size 5
		assert.Equal(t, 20, limiter.Limit())

		now = recordFn(limiter, now, 1300*time.Millisecond, 10) // queue size 5
		assert.Equal(t, 20, limiter.Limit())
	})
}

func TestAdaptiveLimiter_BuilderValidation(t *testing.T) {
	t.Run("should panic on invalid WithRecentWindow", func(t *testing.T) {
		assert.Panicsf(t, func() {
			NewBuilder[any]().WithRecentWindow(time.Minute, time.Second, 1)
		}, "expected panic with invalid recent window")
	})

	t.Run("should panic on invalid WithRecentQuantile", func(t *testing.T) {
		assert.Panicsf(t, func() {
			NewBuilder[any]().WithRecentQuantile(-1)
		}, "expected panic with invalid sample recentQuantile")
	})

	t.Run("should panic on invalid WithLimits", func(t *testing.T) {
		tests := []struct {
			name    string
			min     uint
			max     uint
			initial uint
		}{
			{"min greater than max", 2, 2, 1},
			{"initial less than min", 1, 2, 0},
			{"initial greater than max", 1, 2, 3},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Panicsf(t, func() {
					NewBuilder[any]().WithLimits(tt.min, tt.max, tt.initial)
				}, "expected panic with invalid limits")
			})
		}
	})

	t.Run("should panic on invalid WithMaxLimitFactor", func(t *testing.T) {
		assert.Panicsf(t, func() {
			NewBuilder[any]().WithMaxLimitFactor(.5)
		}, "expected panic with invalid max limit factor")
	})

	t.Run("should panic on invalid WithQueueing", func(t *testing.T) {
		tests := []struct {
			name    string
			initial float32
			max     float32
		}{
			{"initial < 1", .5, 2},
			{"max < 1", 2, .5},
			{"initial greater than max", 2, 1},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Panicsf(t, func() {
					NewBuilder[any]().WithQueueing(tt.initial, tt.max)
				}, "expected panic with invalid limits")
			})
		}
	})
}

func TestAdaptiveLimiter_WithLogger(t *testing.T) {
	// Given
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	limiter := NewBuilder[any]().
		WithLimits(1, 100, 5).
		WithRecentWindow(time.Millisecond, time.Second, 1).
		WithLogger(logger).
		Build().(*adaptiveLimiter[any])

	// When
	permit, err := limiter.AcquirePermit(context.Background())
	assert.NoError(t, err)
	permit.Record()

	// Then
	assert.Contains(t, buf.String(), "limit update")
}

func TestAdaptiveLimiter_Reset(t *testing.T) {
	// Given
	limiter := NewBuilder[any]().WithLimits(5, 100, 10).Build().(*adaptiveLimiter[any])
	limiter.limit = 50
	limiter.recentRTT.Add(time.Millisecond*100, 5)
	limiter.baselineRTT.Add(50.0)
	limiter.smoothedRecentRTT.Add(75.0)
	limiter.medianFilter.Add(80.0)
	limiter.rttCorrelation.Add(10.0, 90.0)
	limiter.throughputCorrelation.Add(15.0, 120.0)

	// When
	limiter.Reset()

	// Then
	assert.Equal(t, 10, limiter.Limit())
	assert.Equal(t, uint(0), limiter.recentRTT.Size)
	assert.Equal(t, 0.0, limiter.recentRTT.Count())
	assert.Equal(t, 0.0, limiter.baselineRTT.Value())
	assert.Equal(t, 0.0, limiter.smoothedRecentRTT.Value())
	assert.Equal(t, 0.0, limiter.medianFilter.Median())
	corr, _, _ := limiter.rttCorrelation.Add(1.0, 1.0)
	assert.Equal(t, 0.0, corr)
	corr, _, _ = limiter.throughputCorrelation.Add(1.0, 1.0)
	assert.Equal(t, 0.0, corr)
}

func acquireAsync(limiter AdaptiveLimiter[any]) {
	go limiter.AcquirePermit(context.Background())
}

func assertQueued(t *testing.T, metrics Metrics, queued int) {
	assert.Eventually(t, func() bool {
		return metrics.Queued() == queued
	}, 300*time.Millisecond, 10*time.Millisecond)
}
