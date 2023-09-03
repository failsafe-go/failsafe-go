package test

import (
	"testing"

	"failsafe"
	"failsafe/fallback"
	"failsafe/internal/testutil"
)

// Tests Fallback.OfResult
func TestFallbackOfResult(t *testing.T) {
	fb := fallback.OfResult[bool](true)

	testutil.TestGetSuccess(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		1, 1, true)
}

// Tests Fallback.OfError
func TestShouldFallbackOfError(t *testing.T) {
	fb := fallback.OfError[bool](testutil.InvalidArgumentError{})

	testutil.TestGetFailure(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		1, 1, testutil.InvalidArgumentError{})
}

// Tests Fallback.OfFn
func TestShouldFallbackOfFn(t *testing.T) {
	fb := fallback.OfFn[bool](func(event failsafe.ExecutionAttemptedEvent[bool]) (bool, error) {
		return false, testutil.InvalidArgumentError{
			Cause: event.LastErr,
		}
	})

	testutil.TestGetFailure(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ConnectionError{}
		},
		1, 1, testutil.InvalidArgumentError{
			Cause: testutil.ConnectionError{},
		})
}

// Tests a successful execution that does not fallback
func TestShouldNotFallback(t *testing.T) {
	testutil.TestGetSuccess(t, failsafe.With[bool](fallback.OfResult[bool](true)),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, nil
		},
		1, 1, false)
}

// Tests a fallback with failure conditions
func TestShouldFallbackWithFailureConditions(t *testing.T) {
	fb := fallback.BuilderOfResult[bool](true).
		HandleErrors(testutil.InvalidStateError{}).
		Build()

	// Fallback should not handle
	testutil.TestGetFailure(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		1, 1, testutil.InvalidArgumentError{})

	// Fallback should handle
	testutil.TestGetSuccess(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidStateError{}
		},
		1, 1, true)
}
