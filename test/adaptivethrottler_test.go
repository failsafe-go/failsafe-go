package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/adaptivethrottler"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/priority"
)

func TestAdaptiveThrottler(t *testing.T) {
	t.Run("should allow execution when no failures", func(t *testing.T) {
		// Given
		throttler := adaptivethrottler.NewBuilder[string]().Build()

		// When / Then
		testutil.Test[string](t).
			With(throttler).
			Get(testutil.GetFn("success", nil)).
			AssertSuccess(1, 1, "success")
	})

	t.Run("should eventually reject with high failure rate", func(t *testing.T) {
		// Given
		throttler := adaptivethrottler.NewBuilder[string]().
			WithFailureRateThreshold(0.1, 1, time.Minute).
			WithMaxRejectionRate(1.0).
			Build()
		recordFailures(throttler, 50)

		// Try many attempts to reject
		for attempt := 0; attempt < 50; attempt++ {
			result, err := testutil.Test[string](t).
				With(throttler).
				Get(testutil.GetFn("success", nil)).
				Exec(false)
			if result == "" && errors.Is(err, adaptivethrottler.ErrExceeded) {
				return
			}
		}
		assert.Fail(t, "should have rejected at least once")
	})

	// Tests a fallback with failure conditions
	t.Run("should succeed or fail based on failure conditions", func(t *testing.T) {
		throttler := adaptivethrottler.NewBuilder[int]().
			HandleResult(500).
			Build()
		reset := func() {
			policytesting.Reset(throttler)
		}

		// Should record as success
		testutil.Test[int](t).
			With(throttler).
			Get(testutil.GetFn(400, nil)).
			AssertSuccess(1, 1, 400)

		// Should record as a failure
		testutil.Test[int](t).
			With(throttler).
			Before(reset).
			Get(testutil.GetFn(0, testutil.ErrInvalidState)).
			AssertFailure(1, 1, testutil.ErrInvalidState)
	})
}

func TestPriorityThrottler(t *testing.T) {
	t.Run("should allow execution when no failures", func(t *testing.T) {
		// Given
		throttler := adaptivethrottler.NewBuilder[string]().BuildPrioritized(adaptivethrottler.NewPrioritizer())

		// When / Then
		testutil.Test[string](t).
			With(throttler).
			Get(testutil.GetFn("success", nil)).
			AssertSuccess(1, 1, "success")
	})

	t.Run("should acquire permit when priority is above rejection threshold", func(t *testing.T) {
		// Given
		p := adaptivethrottler.NewPrioritizer()
		rejectionThreshold := testutil.GetPrioritizerRejectionThreshold(p)
		throttler := adaptivethrottler.NewBuilder[string]().
			WithFailureRateThreshold(0.1, 1, time.Minute).
			WithMaxRejectionRate(1.0).
			BuildPrioritized(p)
		recordPriorityFailures(throttler, 50)
		rejectionThreshold.Store(200)
		ctx := priority.High.AddTo(context.Background())

		// When / Then
		testutil.Test[string](t).
			With(throttler).
			Context(testutil.ContextFn(ctx)).
			Get(testutil.GetFn("test", nil)).
			AssertSuccess(1, 1, "test")
	})

	t.Run("should not acquire permit when priority is below rejection threshold", func(t *testing.T) {
		// Given
		p := adaptivethrottler.NewPrioritizer()
		rejectionThreshold := testutil.GetPrioritizerRejectionThreshold(p)
		throttler := adaptivethrottler.NewBuilder[string]().
			WithFailureRateThreshold(0.1, 1, time.Minute).
			WithMaxRejectionRate(1.0).
			BuildPrioritized(p)
		recordPriorityFailures(throttler, 50)
		rejectionThreshold.Store(200)
		ctx := priority.Low.AddTo(context.Background())

		// Should record as a failure
		testutil.Test[string](t).
			With(throttler).
			Context(testutil.ContextFn(ctx)).
			Get(testutil.GetFn("", testutil.ErrInvalidState)).
			AssertFailure(1, 0, adaptivethrottler.ErrExceeded)
	})
}

func recordFailures[R any](throttler adaptivethrottler.AdaptiveThrottler[R], count int) {
	for i := 0; i < count; i++ {
		throttler.RecordFailure()
	}
}

func recordPriorityFailures[R any](throttler adaptivethrottler.PriorityThrottler[R], count int) {
	for i := 0; i < count; i++ {
		throttler.RecordFailure()
	}
}
