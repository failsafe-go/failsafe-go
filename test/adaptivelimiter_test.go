package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestAdaptiveLimiter(t *testing.T) {
	t.Run("when limit not exceeded", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().WithLimits(2, 2, 2).Build()

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Get(testutil.GetFn[any]("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("should acquire permit after wait", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[string]().
			WithLimits(2, 2, 2).
			WithMaxWaitTime(time.Second).
			Build()
		before := func() {
			shouldAcquireAndDropAfterWait(t, limiter, 2, 100*time.Millisecond)
		}

		// When / Then
		testutil.Test[string](t).
			With(limiter).
			Before(before).
			Get(testutil.GetFn("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("when limit exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		lb := adaptivelimiter.NewBuilder[any]().WithLimits(2, 2, 2)
		limiter := policytesting.WithAdaptiveLimiterStatsAndLogs(lb, stats, true).Build()
		var p1, p2 adaptivelimiter.Permit
		before := func() {
			p1 = shouldAcquire(t, limiter)
			p2 = shouldAcquire(t, limiter) // limiter should be full
		}
		after := func() {
			p1.Drop()
			p2.Drop()
		}

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Before(before).
			After(after).
			Reset(stats).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded, func() {
				assert.Equal(t, 1, stats.LimitsExceeded())
			})
	})

	// Asserts that an exceeded maxWaitTime causes ErrExceeded.
	t.Run("when maxWaitTime exceeded", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().
			WithLimits(2, 2, 2).
			WithMaxWaitTime(20 * time.Millisecond).
			Build()
		shouldAcquire(t, limiter)
		shouldAcquire(t, limiter) // limiter should be full

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded)
	})

	// Asserts that a short maxWaitTime still allows a permit to be claimed.
	t.Run("with short maxWaitTime", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().WithLimits(2, 2, 2).Build()

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Run(testutil.RunFn(nil)).
			AssertSuccess(1, 1, nil)
	})
}

func TestQueueingLimiter(t *testing.T) {
	// This test verifies that executions work when below the limit.
	t.Run("when limit not exceeded", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().
			WithLimits(2, 2, 2).
			WithQueueing(2, 2).
			Build()

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Get(testutil.GetFn[any]("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	// This test fills the queue, adds another execution which waits, then eventually empties the queue.
	t.Run("should acquire permit after wait with queueing", func(t *testing.T) {
		limiter := adaptivelimiter.NewBuilder[string]().
			WithLimits(2, 2, 2). // limit of 2
			WithQueueing(1.5, 1.5). // queue of 1.5 * 2 = 3
			WithMaxWaitTime(time.Second).
			Build()
		before := func() {
			shouldAcquireAndDropAfterWait(t, limiter, 2, 100*time.Millisecond) // fill limiter
			shouldAcquireAndDropAsync(t, limiter, 2)                           // fill queue
			assertQueued(t, limiter, 2)
		}

		// Then
		testutil.Test[string](t).
			With(limiter).
			Before(before).
			Get(testutil.GetFn("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	// This test asserts that executions fail when the limit and queue are exceeded.
	t.Run("when limit and queue exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		lb := adaptivelimiter.NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1)
		limiter := policytesting.WithAdaptiveLimiterStatsAndLogs(lb, stats, true).Build()
		before := func() {
			shouldAcquireAndDropAfterWait(t, limiter, 1, 100*time.Millisecond) // fill limiter
			shouldAcquireAndDropAsync(t, limiter, 1)                           // fill queue
			assertQueued(t, limiter, 1)
		}
		after := func() {
			waitForLimiterToEmpty(t, limiter)
		}

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Before(before).
			After(after).
			Reset(stats).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded, func() {
				assert.Equal(t, 1, stats.LimitsExceeded())
			})
	})

	// This test asserts that executions fail when the limit and queue are exceeded with a maxWaitTime
	t.Run("when maxWaitTime exceeded", func(t *testing.T) {
		// Given
		limiter := adaptivelimiter.NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			WithMaxWaitTime(20 * time.Millisecond).
			Build()
		before := func() {
			shouldAcquireAndDropAfterWait(t, limiter, 1, 100*time.Millisecond) // fill limiter
			shouldAcquireAndDropAsync(t, limiter, 1)                           // fill queue
			assertQueued(t, limiter, 1)
		}
		after := func() {
			waitForLimiterToEmpty(t, limiter)
		}

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Before(before).
			After(after).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded)
	})
}

func TestPriorityLimiter(t *testing.T) {
	// Asserts that executions work when below the limit.
	t.Run("when limit not exceeded", func(t *testing.T) {
		// Given
		p := adaptivelimiter.NewPrioritizer()
		limiter := adaptivelimiter.NewBuilder[any]().
			WithLimits(2, 2, 2).
			WithQueueing(2, 2).
			BuildPrioritized(p)

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Get(testutil.GetFn[any]("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("should acquire permit when priority is above rejection threshold", func(t *testing.T) {
		// Given
		p := adaptivelimiter.NewPrioritizer()
		rejectionThreshold := testutil.GetPrioritizerRejectionThreshold(p)
		limiter := adaptivelimiter.NewBuilder[string]().
			WithLimits(2, 2, 2).
			WithMaxWaitTime(time.Second).
			BuildPrioritized(p)
		ctx := context.WithValue(context.Background(), adaptivelimiter.PriorityKey, adaptivelimiter.PriorityHigh)
		ctxFn := testutil.ContextFn(ctx)
		rejectionThreshold.Store(200)

		// When / Then
		testutil.Test[string](t).
			With(limiter).
			Context(ctxFn).
			Get(testutil.GetFn("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("should not acquire permit when priority is below rejection threshold", func(t *testing.T) {
		// Given
		p := adaptivelimiter.NewPrioritizer()
		rejectionThreshold := testutil.GetPrioritizerRejectionThreshold(p)
		limiter := adaptivelimiter.NewBuilder[string]().
			WithLimits(1, 1, 1).
			WithMaxWaitTime(time.Second).
			BuildPrioritized(p)
		limiter.AcquirePermit(context.Background()) // fill the limiter
		ctx := context.WithValue(context.Background(), adaptivelimiter.PriorityKey, adaptivelimiter.PriorityLow)
		ctxFn := testutil.ContextFn(ctx)
		rejectionThreshold.Store(200)

		// When / Then
		testutil.Test[string](t).
			With(limiter).
			Context(ctxFn).
			Get(testutil.GetFn("test", nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded)
	})

	// This test asserts that executions fail when the limit and queue are exceeded.
	t.Run("when limit and queue exceeded", func(t *testing.T) {
		// Given
		p := adaptivelimiter.NewPrioritizer()
		stats := &policytesting.Stats{}
		lb := adaptivelimiter.NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1)
		limiter := policytesting.WithAdaptiveLimiterStatsAndLogs(lb, stats, true).BuildPrioritized(p)
		before := func() {
			shouldAcquireAndDropAfterWait(t, limiter, 1, 100*time.Millisecond) // fill limiter
			shouldAcquireAndDropAsync(t, limiter, 1)                           // fill queue
			assertQueued(t, limiter, 1)
		}
		after := func() {
			waitForLimiterToEmpty(t, limiter)
		}

		// When / Then
		testutil.Test[any](t).
			With(limiter).
			Before(before).
			After(after).
			Reset(stats).
			Run(testutil.RunFn(nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded, func() {
				assert.Equal(t, 1, stats.LimitsExceeded())
			})
	})
}

type blockingLimiter interface {
	AcquirePermitWithMaxWait(ctx context.Context, maxWaitTime time.Duration) (adaptivelimiter.Permit, error)
}

func shouldAcquire(t *testing.T, limiter blockingLimiter) adaptivelimiter.Permit {
	permit, err := limiter.AcquirePermitWithMaxWait(context.Background(), time.Second)
	require.NoError(t, err)
	require.NotNil(t, permit)
	return permit
}

// Useful for filling an adaptivelimiter's semaphore.
func shouldAcquireAndDropAfterWait(t *testing.T, limiter blockingLimiter, permitCount int, sleepTime time.Duration) {
	var permits = make([]adaptivelimiter.Permit, permitCount)
	for i := 0; i < permitCount; i++ {
		permits[i] = shouldAcquire(t, limiter)
	}
	go func() {
		time.Sleep(sleepTime)
		for i := 0; i < permitCount; i++ {
			permits[i].Drop()
		}
	}()
}

// Useful for queueing executions, since this needs to be done async.
func shouldAcquireAndDropAsync(t *testing.T, limiter blockingLimiter, permitCount int) {
	for i := 0; i < permitCount; i++ {
		go func() {
			permit, err := limiter.AcquirePermitWithMaxWait(context.Background(), time.Second)
			require.NoError(t, err)
			require.NotNil(t, permit)
			permit.Drop()
		}()
	}
}

func assertQueued(t *testing.T, metrics adaptivelimiter.Metrics, queued int) {
	assert.Eventually(t, func() bool {
		return metrics.Queued() == queued
	}, 300*time.Millisecond, 10*time.Millisecond)
}

func waitForLimiterToEmpty(t *testing.T, metrics adaptivelimiter.Metrics) {
	assert.Eventually(t, func() bool {
		return metrics.Inflight() == 0
	}, 300*time.Millisecond, 10*time.Millisecond)
}
