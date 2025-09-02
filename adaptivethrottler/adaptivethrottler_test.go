package adaptivethrottler

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdaptiveThrottler_AcquirePermit(t *testing.T) {
	t.Run("should calculate rejection rate after failures", func(t *testing.T) {
		// Given
		throttler := NewBuilder[any]().Build()
		err := throttler.AcquirePermit()
		assert.NoError(t, err)

		// When
		throttler.RecordFailure()
		throttler.AcquirePermit()

		// Then
		assert.Equal(t, float32(.5), throttler.RejectionRate())
	})

	// This test case has a 99.99% chance of succeeding, but could possibly fail.
	t.Run("should return ErrExceeded when AcquirePermit fails", func(t *testing.T) {
		throttler := NewBuilder[any]().
			WithMaxRejectionRate(1.0).
			WithFailureRateThreshold(0.1, time.Minute).
			Build()
		recordFailures(throttler, 50)

		// When / Then
		for i := 0; i < 50; i++ {
			err := throttler.AcquirePermit()
			assert.Equal(t, 1.0, math.Ceil(float64(throttler.RejectionRate())))
			if errors.Is(err, ErrExceeded) {
				return
			}
		}
		assert.Fail(t, "should have returned an ErrExceeded")
	})

	t.Run("should respect max rejection rate", func(t *testing.T) {
		// Given
		maxRate := float32(0.5)
		throttler := NewBuilder[any]().WithMaxRejectionRate(maxRate).Build()

		// When
		recordFailures(throttler, 20)
		throttler.AcquirePermit()

		// Then
		assert.LessOrEqual(t, throttler.RejectionRate(), maxRate)
	})
}

func recordFailures(throttler AdaptiveThrottler[any], failures int) {
	for i := 0; i < failures; i++ {
		throttler.RecordFailure()
	}
}
