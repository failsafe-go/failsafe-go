package adaptivelimiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPriorityLimiter_AcquirePermit(t *testing.T) {
	p := NewPrioritizer().(*prioritizer)
	l := NewBuilder[any]().BuildPrioritized(p)

	t.Run("with no rejection threshold", func(t *testing.T) {
		permit, err := l.AcquirePermitWithPriority(context.Background(), PriorityLow)
		assert.NotNil(t, permit)
		assert.NoError(t, err)
	})

	t.Run("below prioritizer rejection threshold", func(t *testing.T) {
		p.rejectionThreshold.Store(200)
		permit, err := l.AcquirePermitWithPriority(context.Background(), PriorityLow)
		assert.Nil(t, permit)
		assert.Error(t, err, ErrExceeded)
	})

	t.Run("above prioritizer rejection threshold", func(t *testing.T) {
		p.rejectionThreshold.Store(200)
		permit, err := l.AcquirePermitWithPriority(context.Background(), PriorityHigh)
		assert.NotNil(t, permit)
		assert.NoError(t, err)
	})

	// Asserts that AcquirePermit fails after the max number of requests is queued, even if the request exceeds the rejection threshold
	t.Run("above max queued requests", func(t *testing.T) {
		l := NewBuilder[any]().WithLimits(1, 1, 1).BuildPrioritized(p)
		p.rejectionThreshold.Store(200)

		// Add a request and 3 waiters
		for i := 0; i < 4; i++ {
			go func() {
				permit, err := l.AcquirePermitWithPriority(context.Background(), PriorityHigh)
				assert.NotNil(t, permit)
				assert.NoError(t, err)
			}()
		}

		assert.Eventually(t, func() bool {
			return l.Queued() == 3
		}, 300*time.Millisecond, 10*time.Millisecond)
		permit, err := l.AcquirePermitWithPriority(context.Background(), PriorityHigh)
		assert.Nil(t, permit)
		assert.ErrorIs(t, err, ErrExceeded)
	})
}
