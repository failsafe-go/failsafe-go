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
	"github.com/failsafe-go/failsafe-go/priority"
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
		shouldAcquire(t, limiter)
		shouldAcquire(t, limiter) // limiter should be full

		// When / Then
		testutil.Test[any](t).
			With(limiter).
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
		rejectionThreshold.Store(200)
		ctx := priority.High.AddTo(context.Background())

		// When / Then
		testutil.Test[string](t).
			With(limiter).
			Context(testutil.ContextFn(ctx)).
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
		rejectionThreshold.Store(200)
		ctx := priority.Low.AddTo(context.Background())

		// When / Then
		testutil.Test[string](t).
			With(limiter).
			Context(testutil.ContextFn(ctx)).
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

	t.Run("with usage tracker", func(t *testing.T) {
		// Given
		tracker := priority.NewUsageTracker(time.Minute, 10)
		p := adaptivelimiter.NewPrioritizerBuilder().
			WithUsageTracker(tracker).
			Build()
		limiter := adaptivelimiter.NewBuilder[string]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			BuildPrioritized(p)

		// Prepare to fill/drain the limiter before/after each execution
		before := func() {
			shouldAcquireAndDropAfterWait(t, limiter, 1, 100*time.Millisecond) // fill limiter
		}
		after := func() {
			waitForLimiterToEmpty(t, limiter)
		}

		// Add some usage
		tracker.RecordUsage("user1", 100*time.Millisecond.Nanoseconds())
		tracker.RecordUsage("user2", 200*time.Millisecond.Nanoseconds())
		tracker.Calibrate()

		// Set the rejection threshold
		testutil.GetPrioritizerRejectionThreshold(p).Store(275)

		// When / Then - user 1's level is above the threshold
		mediumCtx := priority.ContextWithPriority(context.Background(), priority.Medium)
		userCtx := priority.ContextWithUser(mediumCtx, "user1") // Should get level 200
		testutil.Test[string](t).
			With(limiter).
			Before(before).
			After(after).
			Context(testutil.ContextFn(userCtx)).
			Get(testutil.GetFn("test", nil)).
			AssertSuccess(1, 1, "test")

		// When / Then - user 2's level is below the threshold
		userCtx = priority.ContextWithUser(mediumCtx, "user2") // Should get level 250
		testutil.Test[string](t).
			With(limiter).
			Before(before).
			After(after).
			Context(testutil.ContextFn(userCtx)).
			Get(testutil.GetFn("test", nil)).
			AssertFailure(1, 0, adaptivelimiter.ErrExceeded)
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
