package adaptivelimiter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextWithPriority(t *testing.T) {
	assert.Equal(t, PriorityHigh, ContextWithPriority(context.Background(), PriorityHigh).Value(PriorityKey))
}

func TestContextWithLevel(t *testing.T) {
	assert.Equal(t, 12, ContextWithLevel(context.Background(), 12).Value(LevelKey))
}

func TestPriorityLimiter_AcquirePermitWithPriority(t *testing.T) {
	t.Run("with no rejection threshold", func(t *testing.T) {
		limiter := NewBuilder[any]().BuildPrioritized(NewPrioritizer())
		permit, err := limiter.AcquirePermitWithPriority(context.Background(), PriorityLow)
		assert.NotNil(t, permit)
		assert.NoError(t, err)
	})

	t.Run("above prioritizer rejection threshold", func(t *testing.T) {
		// Given
		p := NewPrioritizer().(*prioritizer)
		limiter := NewBuilder[any]().BuildPrioritized(p)
		p.rejectionThreshold.Store(200)

		// When
		permit, err := limiter.AcquirePermitWithPriority(context.Background(), PriorityHigh)

		// Then
		assert.NotNil(t, permit)
		assert.NoError(t, err)
	})

	t.Run("below prioritizer rejection threshold", func(t *testing.T) {
		// Given
		p := NewPrioritizer().(*prioritizer)
		limiter := NewBuilder[any]().
			WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			BuildPrioritized(p)
		limiter.AcquirePermit(context.Background()) // fill limiter
		p.rejectionThreshold.Store(200)

		// When
		permit, err := limiter.AcquirePermitWithPriority(context.Background(), PriorityLow)

		// Then
		assert.Nil(t, permit)
		assert.Error(t, err, ErrExceeded)
	})

	// Asserts that AcquirePermitWithPriority fails after the max number of executions is queued, even if the execution is
	// within the rejection threshold.
	t.Run("above max queued executions", func(t *testing.T) {
		p := NewPrioritizer().(*prioritizer)
		limiter := NewBuilder[any]().WithLimits(1, 1, 1).
			WithQueueing(1, 1).
			BuildPrioritized(p)
		shouldAcquireWithPriority(t, limiter, PriorityHigh)    // fill the limiter
		go shouldAcquireWithPriority(t, limiter, PriorityHigh) // fill the queue
		assertQueued(t, limiter, 1)

		permit, err := limiter.AcquirePermitWithPriority(context.Background(), PriorityHigh)
		assert.Nil(t, permit)
		assert.ErrorIs(t, err, ErrExceeded)
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

func shouldAcquireWithPriority[R any](t *testing.T, limiter PriorityLimiter[R], priority Priority) Permit {
	permit, err := limiter.AcquirePermitWithPriority(context.Background(), priority)
	require.NotNil(t, permit)
	require.NoError(t, err)
	return permit
}
