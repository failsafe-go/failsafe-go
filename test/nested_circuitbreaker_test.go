package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"failsafe"
	"failsafe/circuitbreaker"
	"failsafe/internal/testutil"
)

// Tests that multiple circuit breakers handle failures as expected, regardless of order.
func TestNestedCircuitBreakers(t *testing.T) {
	innerCb := circuitbreaker.Builder[any]().Handle(testutil.InvalidArgumentError{}).Build()
	outerCb := circuitbreaker.Builder[any]().Handle(testutil.InvalidStateError{}).Build()

	failsafe.With[any](outerCb, innerCb).Run(testutil.RunFn(testutil.InvalidArgumentError{}))
	assert.True(t, innerCb.IsOpen())
	assert.True(t, outerCb.IsClosed())

	innerCb.Close()
	failsafe.With[any](innerCb, outerCb).Run(testutil.RunFn(testutil.InvalidArgumentError{}))
	assert.True(t, innerCb.IsOpen())
	assert.True(t, outerCb.IsClosed())
}

// CircuitBreaker -> CircuitBreaker
func TestCircuitBreakerCircuitBreaker(t *testing.T) {
	// Given
	cb1 := circuitbreaker.Builder[any]().Handle(testutil.InvalidStateError{}).Build()
	cb2 := circuitbreaker.Builder[any]().Handle(testutil.InvalidArgumentError{}).Build()

	testutil.TestRunFailure[any](t, failsafe.With[any](cb2, cb1),
		func(execution failsafe.Execution[any]) error {
			return testutil.InvalidStateError{}
		},
		1, 1, testutil.InvalidStateError{})
	assert.Equal(t, uint(1), cb1.GetFailureCount())
	assert.Equal(t, uint(0), cb2.GetFailureCount())
	assert.True(t, cb1.IsOpen())
	assert.True(t, cb2.IsClosed())

	cb1.Close()
	testutil.TestRunFailure[any](t, failsafe.With[any](cb2, cb1),
		func(execution failsafe.Execution[any]) error {
			return testutil.InvalidArgumentError{}
		},
		1, 1, testutil.InvalidArgumentError{})
	assert.Equal(t, uint(0), cb1.GetFailureCount())
	assert.Equal(t, uint(1), cb2.GetFailureCount())
	assert.True(t, cb1.IsClosed())
	assert.True(t, cb2.IsOpen())
}
