package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestComposeAny(t *testing.T) {
	// Tests RetryPolicy -> CircuitBreaker
	t.Run("should handle success using ComposeAny", func(t *testing.T) {
		retryPolicy := retrypolicy.NewBuilder[string]().WithMaxRetries(2).Build()
		cb := circuitbreaker.NewBuilder[any]().Build()

		testutil.Test[string](t).
			With(retryPolicy).
			ComposeAny(cb).
			Get(testutil.GetFn[string]("success", nil)).
			AssertSuccess(1, 1, "success")
	})

	// Tests Circuitbreaker -> RetryPolicy
	t.Run("should handle success using WithAny", func(t *testing.T) {
		retryPolicy := retrypolicy.NewBuilder[string]().WithMaxRetries(2).Build()
		cb := circuitbreaker.NewBuilder[any]().Build()

		testutil.Test[string](t).
			WithAny(cb).
			Compose(retryPolicy).
			Get(testutil.GetFn[string]("success", nil)).
			AssertSuccess(1, 1, "success")
	})

	t.Run("should handle failure using ComposeAny", func(t *testing.T) {
		retryPolicy := retrypolicy.NewBuilder[string]().Build()
		cb := circuitbreaker.NewBuilder[any]().Build()

		testutil.Test[string](t).
			Before(func() {
				policytesting.Reset(cb)
			}).
			With(retryPolicy).
			ComposeAny(cb).
			Get(testutil.GetFn[string]("", testutil.ErrInvalidState)).
			AssertFailure(3, 1, circuitbreaker.ErrOpen, func() {
				assert.True(t, cb.IsOpen())
			})
	})

	t.Run("should handle failure using WithAny", func(t *testing.T) {
		retryPolicy := retrypolicy.NewBuilder[string]().Build()
		cb := circuitbreaker.NewBuilder[any]().Build()

		testutil.Test[string](t).
			Before(func() {
				policytesting.Reset(cb)
			}).
			WithAny(cb).
			Compose(retryPolicy).
			Get(testutil.GetFn[string]("", testutil.ErrInvalidState)).
			AssertFailure(3, 3, testutil.ErrInvalidState, func() {
				assert.True(t, cb.IsOpen())
			})
	})

	t.Run("should compose multiple any policies", func(t *testing.T) {
		retryPolicy := retrypolicy.NewBuilder[string]().WithMaxRetries(2).Build()
		cb := circuitbreaker.NewBuilder[any]().Build()
		rl := ratelimiter.NewBursty[any](100, time.Second)

		testutil.Test[string](t).
			With(retryPolicy).
			ComposeAny(cb).
			ComposeAny(rl).
			Get(testutil.GetFn[string]("success", nil)).
			AssertSuccess(1, 1, "success")
	})

	t.Run("should compose circuit breaker around differently typed execution results", func(t *testing.T) {
		cb := circuitbreaker.NewBuilder[any]().Build()

		testutil.Test[string](t).
			With(retrypolicy.NewWithDefaults[string]()).
			ComposeAny(cb).
			Get(testutil.GetFn[string]("success", nil)).
			AssertSuccess(1, 1, "success")

		testutil.Test[bool](t).
			With(retrypolicy.NewWithDefaults[bool]()).
			ComposeAny(cb).
			Get(testutil.GetFn[bool](true, nil)).
			AssertSuccess(1, 1, true)
	})
}
