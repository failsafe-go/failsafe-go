package test

import (
	"testing"

	"failsafe"
	"failsafe/fallback"
	"failsafe/internal/testutil"
)

// Tests Fallback.WithResult
func TestFallbackWithResult(t *testing.T) {
	fb := fallback.WithResult[bool](true)

	testutil.TestGetSuccess(t, failsafe.WithResult[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		1, 1, true)
}

// Tests Fallback.WithError
func TestShouldFallbackWithError(t *testing.T) {
	fb := fallback.WithError[bool](testutil.InvalidArgumentError{})

	testutil.TestGetFailure(t, failsafe.WithResult[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		1, 1, testutil.InvalidArgumentError{})
}

// Tests Fallback.WithErrorFn
func TestShouldFallbackWithErrorFn(t *testing.T) {
	fb := fallback.WithErrorFn[bool](func(err error) error {
		return testutil.InvalidArgumentError{
			Cause: err,
		}
	})

	testutil.TestGetFailure(t, failsafe.WithResult[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ConnectionError{}
		},
		1, 1, testutil.InvalidArgumentError{
			Cause: testutil.ConnectionError{},
		})
}

// Tests a successful execution that does not fallback
func TestShouldNotFallback(t *testing.T) {
	testutil.TestGetSuccess(t, failsafe.WithResult[bool](fallback.WithResult[bool](true)),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, nil
		},
		1, 1, false)
}

// Tests a fallback with failure conditions
func TestShouldFallbackWithFailureConditions(t *testing.T) {
	fb := fallback.BuilderWithResult[bool](true).
		Handle(testutil.InvalidStateError{}).
		Build()

	// Fallback should not handle
	testutil.TestGetFailure(t, failsafe.WithResult[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		1, 1, testutil.InvalidArgumentError{})

	// Fallback should handle
	testutil.TestGetSuccess(t, failsafe.WithResult[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidStateError{}
		},
		1, 1, true)
}
