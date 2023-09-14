package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

// Tests that multiple circuit breakers handle failures as expected, regardless of order.
func TestNestedCircuitBreakers(t *testing.T) {
	innerCb := circuitbreaker.Builder[any]().HandleErrors(testutil.ErrInvalidArgument).Build()
	outerCb := circuitbreaker.Builder[any]().HandleErrors(testutil.ErrInvalidState).Build()

	failsafe.Run(testutil.RunFn(testutil.ErrInvalidArgument), innerCb, outerCb)
	assert.True(t, innerCb.IsOpen())
	assert.True(t, outerCb.IsClosed())

	innerCb.Close()
	failsafe.Run(testutil.RunFn(testutil.ErrInvalidArgument), innerCb, outerCb)
	assert.True(t, innerCb.IsOpen())
	assert.True(t, outerCb.IsClosed())
}

// CircuitBreaker -> CircuitBreaker
func TestCircuitBreakerCircuitBreaker(t *testing.T) {
	// Given
	cb1 := circuitbreaker.Builder[any]().HandleErrors(testutil.ErrInvalidState).Build()
	cb2 := circuitbreaker.Builder[any]().HandleErrors(testutil.ErrInvalidArgument).Build()

	testutil.TestRunFailure[any](t, failsafe.NewExecutor[any](cb2, cb1),
		func(execution failsafe.Execution[any]) error {
			return testutil.ErrInvalidState
		},
		1, 1, testutil.ErrInvalidState)
	assert.Equal(t, uint(1), cb1.Metrics().FailureCount())
	assert.Equal(t, uint(0), cb2.Metrics().FailureCount())
	assert.True(t, cb1.IsOpen())
	assert.True(t, cb2.IsClosed())

	cb1.Close()
	testutil.TestRunFailure[any](t, failsafe.NewExecutor[any](cb2, cb1),
		func(execution failsafe.Execution[any]) error {
			return testutil.ErrInvalidArgument
		},
		1, 1, testutil.ErrInvalidArgument)
	assert.Equal(t, uint(0), cb1.Metrics().FailureCount())
	assert.Equal(t, uint(1), cb2.Metrics().FailureCount())
	assert.True(t, cb1.IsClosed())
	assert.True(t, cb2.IsOpen())
}
