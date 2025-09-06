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
		assert.Equal(t, .5, throttler.RejectionRate())
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
			assert.Equal(t, 1.0, math.Ceil(throttler.RejectionRate()))
			if errors.Is(err, ErrExceeded) {
				return
			}
		}
		assert.Fail(t, "should have returned an ErrExceeded")
	})

	t.Run("should respect max rejection rate", func(t *testing.T) {
		// Given
		maxRate := .5
		throttler := NewBuilder[any]().WithMaxRejectionRate(maxRate).Build()

		// When
		recordFailures(throttler, 20)
		throttler.AcquirePermit()

		// Then
		assert.LessOrEqual(t, throttler.RejectionRate(), maxRate)
	})
}

func TestComputeRejectionRate(t *testing.T) {
	t.Run("with a system performing better than the successRateThreshold", func(t *testing.T) {
		// System is actually doing better than required
		executions := 100.0
		successes := 95.0
		successRateThreshold := 0.9
		maxRejectionRate := 0.8

		result := computeRejectionRate(executions, successes, successRateThreshold, maxRejectionRate)

		assert.Equal(t, 0.0, result)
	})

	t.Run("with a slightly overloaded system", func(t *testing.T) {
		executions := 120.0
		successes := 90.0
		successRateThreshold := 0.9
		maxRejectionRate := 0.8

		result := computeRejectionRate(executions, successes, successRateThreshold, maxRejectionRate)

		// maxAllowedExecutions = 90 / 0.9 = 100
		// excessExecutions = max(0, 120 - 100) = 20
		// rejectionRate = 20 / (120 + 1) ≈ 0.1653
		expected := 20.0 / (120.0 + executionPadding)
		assert.InDelta(t, expected, result, 0.001)
	})

	t.Run("with a heavily overloaded system", func(t *testing.T) {
		executions := 200.0
		successes := 50.0
		successRateThreshold := 0.9
		maxRejectionRate := 0.8

		result := computeRejectionRate(executions, successes, successRateThreshold, maxRejectionRate)

		// With 50 successes and 0.9 threshold, we should have had ~56 max executions
		// We had 200, so we're massively over capacity
		// Should result in a rejection rate around 70-75%
		assert.True(t, result > 0.7)
		assert.True(t, result < 0.8)
	})

	t.Run("system with max rejection rate cap", func(t *testing.T) {
		executions := 1000.0
		successes := 10.0
		successRateThreshold := 0.9
		maxRejectionRate := 0.8

		result := computeRejectionRate(executions, successes, successRateThreshold, maxRejectionRate)

		assert.Equal(t, maxRejectionRate, result)
	})

	t.Run("with zero executions", func(t *testing.T) {
		executions := 0.0
		successes := 0.0
		successRateThreshold := 0.9
		maxRejectionRate := 0.8

		result := computeRejectionRate(executions, successes, successRateThreshold, maxRejectionRate)

		assert.Equal(t, 0.0, result)
	})

	t.Run("with zero successes", func(t *testing.T) {
		executions := 100.0
		successes := 0.0
		successRateThreshold := 0.9
		maxRejectionRate := 0.8

		result := computeRejectionRate(executions, successes, successRateThreshold, maxRejectionRate)

		assert.Equal(t, maxRejectionRate, result)
	})

	t.Run("with decreasing success rates", func(t *testing.T) {
		// Given
		executions := 100.0
		successRateThreshold := 0.9
		maxRejectionRate := 0.8

		// When / Then
		healthyRejection := computeRejectionRate(executions, 90.0, successRateThreshold, maxRejectionRate)
		assert.Equal(t, 0.0, healthyRejection)
		strugglingRejection := computeRejectionRate(executions, 70.0, successRateThreshold, maxRejectionRate)
		assert.Greater(t, strugglingRejection, healthyRejection)
		unhealthyRejection := computeRejectionRate(executions, 50.0, successRateThreshold, maxRejectionRate)
		assert.Greater(t, unhealthyRejection, strugglingRejection)
	})
}

func recordFailures(throttler AdaptiveThrottler[any], failures int) {
	for i := 0; i < failures; i++ {
		throttler.RecordFailure()
	}
}
