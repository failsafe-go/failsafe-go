package test

import (
	"testing"

	"failsafe"
	"failsafe/circuitbreaker"
	"failsafe/internal/testutil"
)

func TestShouldRejectInitialExecutionWhenCircuitOpen(t *testing.T) {
	// Given
	cb := circuitbreaker.OfDefaults[any]()
	cb.Open()

	// When Then
	testutil.TestRunFailure(t, failsafe.With(cb),
		func(execution failsafe.Execution[any]) error {
			return testutil.InvalidArgumentError{}
		},
		1, 0, circuitbreaker.ErrCircuitBreakerOpen)
}
