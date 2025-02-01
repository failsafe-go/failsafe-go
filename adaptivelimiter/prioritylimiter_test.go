package adaptivelimiter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPriorityLimiter_AcquirePermit(t *testing.T) {
	p := NewPrioritizer().(*prioritizer)
	limiter := NewBuilder[any]().BuildPrioritized(p)

	t.Run("with no rejection threshold", func(t *testing.T) {
		permit, err := limiter.AcquirePermit(context.Background(), PriorityLow)
		require.NotNil(t, permit)
		require.NoError(t, err)
	})

	t.Run("below rejection threshold", func(t *testing.T) {
		p.priorityThreshold.Store(200)
		permit, err := limiter.AcquirePermit(context.Background(), PriorityLow)
		require.Nil(t, permit)
		require.Error(t, err, ErrExceeded)
	})

	t.Run("above rejection threshold", func(t *testing.T) {
		p.priorityThreshold.Store(200)
		permit, err := limiter.AcquirePermit(context.Background(), PriorityHigh)
		require.NotNil(t, permit)
		require.NoError(t, err)
	})
}
