package adaptivelimiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveLimiter(t *testing.T) {
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
		assert.Equal(t, 0.0, limiter.shortRTT.Count())
		assert.Equal(t, 0.0, limiter.longRTT.Value())
		assert.Equal(t, 0, limiter.Inflight())
		assert.Equal(t, 0, limiter.Blocked())
	})
}

func TestAdaptiveLimiter_AcquirePermitAndRecord(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()

	permit, err := limiter.AcquirePermit(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, limiter.Inflight())
	permit.Record()
	assert.Equal(t, 0, limiter.Inflight())
}

func TestAdaptiveLimiter_TryAcquirePermitAndRecord(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()

	permit, ok := limiter.TryAcquirePermit()
	require.True(t, ok)
	assert.Equal(t, 1, limiter.Inflight())
	_, ok = limiter.TryAcquirePermit()
	require.False(t, ok)
	permit.Record()
	assert.Equal(t, 0, limiter.Inflight())
}

func TestAdaptiveLimiter_CanAcquirePermit(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()

	require.True(t, limiter.CanAcquirePermit())
	permit, err := limiter.AcquirePermit(context.Background())
	require.NoError(t, err)
	require.False(t, limiter.CanAcquirePermit())
	permit.Record()
	require.True(t, limiter.CanAcquirePermit())
}

// Asserts that blocking requests are counted.
func TestAdaptiveLimiter_Blocked(t *testing.T) {
	limiter := NewBuilder[any]().WithLimits(1, 20, 1).Build()
	permit, err := limiter.AcquirePermit(context.Background())
	require.NoError(t, err)

	go func() {
		limiter.AcquirePermit(context.Background())
	}()
	assert.Eventually(t, func() bool {
		return limiter.Blocked() == 1
	}, 100*time.Millisecond, 10*time.Millisecond)
	permit.Record()
	require.Equal(t, 0, limiter.Blocked())
}

func TestAdaptiveLimiter_record(t *testing.T) {
	createLimiter := func() (*adaptiveLimiter[any], time.Time) {
		limiter := NewBuilder[any]().Build().(*adaptiveLimiter[any])
		now := time.UnixMilli(0)
		limiter.nextUpdateTime = now
		limiter.WithShortWindow(time.Second, time.Second, 1)
		for i := 0; i < warmupSamples; i++ {
			limiter.longRTT.Add(float64(time.Second))
		}
		return limiter, now
	}

	recordFn := func(limiter *adaptiveLimiter[any], startTime time.Time, rtt time.Duration, inflight int) time.Time {
		// Simulate recording a sample after 2 seconds
		now := startTime.Add(2 * time.Second)
		require.NoError(t, limiter.semaphore.Acquire(context.Background()))
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
		assert.Equal(t, float64(200*time.Millisecond), limiter.smoothedShortRTT.Value())
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
