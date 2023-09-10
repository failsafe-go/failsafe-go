package test

import (
	"testing"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

// Tests Fallback.WithResult
func TestFallbackOfResult(t *testing.T) {
	fb := fallback.WithResult[bool](true)

	testutil.TestGetSuccess(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrInvalidArgument
		},
		1, 1, true)
}

// Tests Fallback.WithError
func TestShouldFallbackOfError(t *testing.T) {
	fb := fallback.WithError[bool](testutil.ErrInvalidArgument)

	testutil.TestGetFailure(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrInvalidArgument
		},
		1, 1, testutil.ErrInvalidArgument)
}

// Tests Fallback.WithFn
func TestShouldFallbackOfFn(t *testing.T) {
	fb := fallback.WithFn[bool](func(exec failsafe.Execution[bool]) (bool, error) {
		return false, &testutil.CompositeError{
			Cause: exec.LastError(),
		}
	})

	testutil.TestGetFailure(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrConnecting
		},
		1, 1, &testutil.CompositeError{
			Cause: testutil.ErrConnecting,
		})
}

// Tests a successful execution that does not fallback
func TestShouldNotFallback(t *testing.T) {
	testutil.TestGetSuccess(t, failsafe.With[bool](fallback.WithResult[bool](true)),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, nil
		},
		1, 1, false)
}

// Tests a fallback with failure conditions
func TestShouldFallbackWithFailureConditions(t *testing.T) {
	fb := fallback.BuilderWithResult[bool](true).
		HandleErrors(testutil.ErrInvalidState).
		Build()

	// Fallback should not handle
	testutil.TestGetFailure(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrInvalidArgument
		},
		1, 1, testutil.ErrInvalidArgument)

	// Fallback should handle
	testutil.TestGetSuccess(t, failsafe.With[bool](fb),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrInvalidState
		},
		1, 1, true)
}
