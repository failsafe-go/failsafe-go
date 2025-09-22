package adaptivelimiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	priorityInternal "github.com/failsafe-go/failsafe-go/internal/priority"
	"github.com/failsafe-go/failsafe-go/priority"
)

func TestPriorityLimiter_AcquirePermitWithPriority(t *testing.T) {
	t.Run("with no rejection threshold", func(t *testing.T) {
		limiter := NewBuilder[any]().BuildPrioritized(NewPrioritizer())
		permit, err := limiter.AcquirePermitWithPriority(context.Background(), priority.Low)
		assert.NotNil(t, permit)
		assert.NoError(t, err)
	})

	t.Run("above prioritizer rejection threshold", func(t *testing.T) {
		// Given
		p := NewPrioritizer().(*priorityInternal.BasePrioritizer[*queueStats])
		limiter := NewBuilder[any]().BuildPrioritized(p)
		p.RejectionThresh.Store(200)

		// When
		permit, err := limiter.AcquirePermitWithPriority(context.Background(), priority.High)

		// Then
		assert.NotNil(t, permit)
		assert.NoError(t, err)
	})

	t.Run("below prioritizer rejection threshold", func(t *testing.T) {
		// Given
		p := NewPrioritizer().(*priorityInternal.BasePrioritizer[*queueStats])
		limiter := NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			BuildPrioritized(p)
		limiter.AcquirePermit(context.Background()) // fill the limiter
		p.RejectionThresh.Store(200)

		// When
		permit, err := limiter.AcquirePermitWithPriority(context.Background(), priority.Low)

		// Then
		assert.Nil(t, permit)
		assert.Error(t, err, ErrExceeded)
	})

	// Asserts that AcquirePermitWithPriority fails after the max number of executions is queued, even if the execution is
	// within the rejection threshold.
	t.Run("above max queued executions", func(t *testing.T) {
		p := NewPrioritizer()
		limiter := NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			BuildPrioritized(p)
		shouldAcquireWithPriority(t, limiter, priority.High)    // fill the limiter
		go shouldAcquireWithPriority(t, limiter, priority.High) // fill the queue
		assertQueued(t, limiter, 1)

		permit, err := limiter.AcquirePermitWithPriority(context.Background(), priority.High)
		assert.Nil(t, permit)
		assert.Error(t, err, ErrExceeded)
	})
}

func TestPriorityLimiter_CanAcquirePermit(t *testing.T) {
	t.Run("with usage tracker", func(t *testing.T) {
		// Given
		tracker := priority.NewUsageTracker(time.Minute, 10)
		p := NewPrioritizerBuilder().
			WithUsageTracker(tracker).
			Build().(*priorityInternal.BasePrioritizer[*queueStats])
		limiter := NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			BuildPrioritized(p)
		shouldAcquireWithPriority(t, limiter, priority.High) // fill the limiter
		p.RejectionThresh.Store(275)

		tracker.RecordUsage("user1", 100)
		tracker.RecordUsage("user2", 200)
		tracker.Calibrate()

		// When / Then - user 1's level is above the threshold
		mediumCtx := priority.ContextWithPriority(context.Background(), priority.Medium)
		userCtx := priority.ContextWithUser(mediumCtx, "user1")
		assert.True(t, limiter.CanAcquirePermit(userCtx))

		// When / Then - user 2's level is below the threshold
		userCtx = priority.ContextWithUser(mediumCtx, "user2")
		assert.False(t, limiter.CanAcquirePermit(userCtx))
	})
}

func TestPriorityLimiter_AcquirePermitWithMaxWaitAndRecord(t *testing.T) {
	// Given
	limiter := NewBuilder[any]().WithLimits(1, 1, 1).BuildPrioritized(NewPrioritizer())

	// When
	permit, err := limiter.AcquirePermitWithMaxWait(context.Background(), 0)

	// Then
	assert.NoError(t, err)
	assert.Equal(t, 1, limiter.Inflight())
	permit.Record()
	assert.Equal(t, 0, limiter.Inflight())
}

func shouldAcquireWithPriority[R any](t *testing.T, limiter PriorityLimiter[R], priority priority.Priority) Permit {
	permit, err := limiter.AcquirePermitWithPriority(context.Background(), priority)
	require.NotNil(t, permit)
	require.NoError(t, err)
	return permit
}
