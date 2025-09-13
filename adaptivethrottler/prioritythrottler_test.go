package adaptivethrottler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/priority"
)

func TestPriorityThrottler_AcquirePermitWithPriority(t *testing.T) {
	t.Run("with no rejection threshold", func(t *testing.T) {
		throttler := NewBuilder[any]().BuildPrioritized(NewPrioritizer())
		err := throttler.AcquirePermitWithPriority(priority.Low)
		assert.NoError(t, err)
	})

	t.Run("above prioritizer rejection threshold", func(t *testing.T) {
		// Given
		p := NewPrioritizer().(*priority.BasePrioritizer[*throttlerStats])
		throttler := NewBuilder[any]().BuildPrioritized(p)
		p.RejectionThresh.Store(200)

		// When
		err := throttler.AcquirePermitWithPriority(priority.High)

		// Then
		assert.NoError(t, err)
	})

	t.Run("below prioritizer rejection threshold", func(t *testing.T) {
		// Given
		p := NewPrioritizer().(*priority.BasePrioritizer[*throttlerStats])
		throttler := NewBuilder[any]().BuildPrioritized(p)
		p.RejectionThresh.Store(200)

		// When
		err := throttler.AcquirePermitWithPriority(priority.Low)

		// Then
		assert.Error(t, err, ErrExceeded)
	})
}
