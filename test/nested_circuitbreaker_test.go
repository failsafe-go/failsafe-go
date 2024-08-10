package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

// Tests that multiple circuit breakers handle failures as expected, regardless of order.
func TestNestedCircuitBreakers(t *testing.T) {
	innerCb := circuitbreaker.NewBuilder[any]().HandleErrors(testutil.ErrInvalidArgument).Build()
	outerCb := circuitbreaker.NewBuilder[any]().HandleErrors(testutil.ErrInvalidState).Build()

	failsafe.RunWithExecution(testutil.RunFn(testutil.ErrInvalidArgument), innerCb, outerCb)
	assert.True(t, innerCb.IsOpen())
	assert.True(t, outerCb.IsClosed())

	innerCb.Close()
	failsafe.RunWithExecution(testutil.RunFn(testutil.ErrInvalidArgument), innerCb, outerCb)
	assert.True(t, innerCb.IsOpen())
	assert.True(t, outerCb.IsClosed())
}

// CircuitBreaker -> CircuitBreaker
func TestCircuitBreakerCircuitBreaker(t *testing.T) {
	// Given
	cb1 := circuitbreaker.NewBuilder[any]().HandleErrors(testutil.ErrInvalidState).Build()
	cb2 := circuitbreaker.NewBuilder[any]().HandleErrors(testutil.ErrInvalidArgument).Build()
	setup := func() {
		policytesting.ResetCircuitBreaker(cb1)
		policytesting.ResetCircuitBreaker(cb2)
	}

	testutil.Test[any](t).
		With(cb2, cb1).
		Setup(setup).
		Run(testutil.RunFn(testutil.ErrInvalidState)).
		AssertFailure(1, 1, testutil.ErrInvalidState)
	assert.Equal(t, uint(1), cb1.Metrics().Failures())
	assert.Equal(t, uint(0), cb2.Metrics().Failures())
	assert.True(t, cb1.IsOpen())
	assert.True(t, cb2.IsClosed())

	testutil.Test[any](t).
		With(cb2, cb1).
		Setup(setup).
		Run(testutil.RunFn(testutil.ErrInvalidArgument)).
		AssertFailure(1, 1, testutil.ErrInvalidArgument)
	assert.Equal(t, uint(0), cb1.Metrics().Failures())
	assert.Equal(t, uint(1), cb2.Metrics().Failures())
	assert.True(t, cb1.IsClosed())
	assert.True(t, cb2.IsOpen())
}
