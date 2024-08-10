package test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

// Tests Fallback.NewWithResult
func TestFallbackWithResult(t *testing.T) {
	fb := fallback.NewWithResult(true)

	testutil.Test[bool](t).
		With(fb).
		Get(testutil.GetFn(false, testutil.ErrInvalidArgument)).
		AssertSuccess(1, 1, true)
}

// Tests Fallback.NewWithError
func TestShouldFallbackWithError(t *testing.T) {
	fb := fallback.NewWithError[bool](testutil.ErrInvalidArgument)

	testutil.Test[bool](t).
		With(fb).
		Get(testutil.GetFn(false, testutil.ErrInvalidArgument)).
		AssertFailure(1, 1, testutil.ErrInvalidArgument)
}

// Tests Fallback.NewWithFunc
func TestShouldFallbackWithFunc(t *testing.T) {
	fb := fallback.NewWithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
		return false, testutil.CompositeError{Cause: exec.LastError()}
	})

	testutil.Test[bool](t).
		With(fb).
		Get(testutil.GetFn(false, testutil.ErrConnecting)).
		AssertFailureAs(1, 1, &testutil.CompositeError{Cause: testutil.ErrConnecting})
}

// Tests a successful execution that does not fallback
func TestShouldNotFallback(t *testing.T) {
	testutil.Test[bool](t).
		With(fallback.NewWithResult(true)).
		Get(testutil.GetFn(false, nil)).
		AssertSuccess(1, 1, false)
}

// Tests a fallback with failure conditions
func TestShouldFallbackWithFailureConditions(t *testing.T) {
	fb := fallback.NewBuilderWithResult[int](0).
		HandleResult(500).
		Build()

	// Fallback should not handle
	testutil.Test[int](t).
		With(fb).
		Get(testutil.GetFn(400, nil)).
		AssertSuccess(1, 1, 400)

	// Fallback should handle
	testutil.Test[int](t).
		With(fb).
		Get(testutil.GetFn(500, nil)).
		AssertSuccess(1, 1, 0)
}

// Asserts that the fallback result itself can cause an execution to be considered a failure.
func TestShouldVerifyFallbackResult(t *testing.T) {
	// Assert a failure is still a failure
	fb := fallback.NewWithError[any](testutil.ErrInvalidArgument)
	testutil.Test[any](t).
		With(fb).
		Get(testutil.GetFn[any](false, errors.New("test"))).
		AssertFailure(1, 1, testutil.ErrInvalidArgument)

	// Assert a success after a failure is a success
	fb = fallback.NewWithResult[any](true)
	testutil.Test[any](t).
		With(fb).
		Get(testutil.GetFn[any](false, errors.New("test"))).
		AssertSuccess(1, 1, true)
}

func TestShouldNotCallFallbackWhenCanceled(t *testing.T) {
	// Given
	fb := fallback.NewWithFunc(func(exec failsafe.Execution[any]) (any, error) {
		assert.Fail(t, "should not call fallback")
		return nil, nil
	})

	// When / Then
	testutil.Test[any](t).
		With(fb).
		Context(testutil.CanceledContextFn).
		Get(testutil.GetFn[any](nil, testutil.ErrInvalidArgument)).
		AssertFailure(1, 1, context.Canceled)
}
