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

// Tests Fallback.WithResult
func TestFallbackOfResult(t *testing.T) {
	fb := fallback.WithResult(true)

	testutil.Test[bool](t).
		With(fb).
		Get(testutil.GetFn(false, testutil.ErrInvalidArgument)).
		AssertSuccess(1, 1, true)
}

// Tests Fallback.WithError
func TestShouldFallbackOfError(t *testing.T) {
	fb := fallback.WithError[bool](testutil.ErrInvalidArgument)

	testutil.Test[bool](t).
		With(fb).
		Get(testutil.GetFn(false, testutil.ErrInvalidArgument)).
		AssertFailure(1, 1, testutil.ErrInvalidArgument)
}

// Tests Fallback.WithFunc
func TestShouldFallbackOfFn(t *testing.T) {
	fb := fallback.WithFunc(func(exec failsafe.Execution[bool]) (bool, error) {
		return false, testutil.NewCompositeError(exec.LastError())
	})

	testutil.Test[bool](t).
		With(fb).
		Get(testutil.GetFn(false, testutil.ErrConnecting)).
		AssertFailure(1, 1, testutil.NewCompositeError(testutil.ErrConnecting))
}

// Tests a successful execution that does not fallback
func TestShouldNotFallback(t *testing.T) {
	testutil.Test[bool](t).
		With(fallback.WithResult(true)).
		Get(testutil.GetFn(false, nil)).
		AssertSuccess(1, 1, false)
}

// Tests a fallback with failure conditions
func TestShouldFallbackWithFailureConditions(t *testing.T) {
	fb := fallback.BuilderWithResult[int](0).
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
	fb := fallback.WithError[any](testutil.ErrInvalidArgument)
	testutil.Test[any](t).
		With(fb).
		Get(testutil.GetFn[any](false, errors.New("test"))).
		AssertFailure(1, 1, testutil.ErrInvalidArgument)

	// Assert a success after a failure is a success
	fb = fallback.WithResult[any](true)
	testutil.Test[any](t).
		With(fb).
		Get(testutil.GetFn[any](false, errors.New("test"))).
		AssertSuccess(1, 1, true)
}

func TestShouldNotCallFallbackWhenCanceled(t *testing.T) {
	// Given
	setup := testutil.SetupWithContextSleep(0)
	fb := fallback.WithFunc(func(exec failsafe.Execution[any]) (any, error) {
		assert.Fail(t, "should not call fallback")
		return nil, nil
	})

	// When / Then
	testutil.Test[any](t).
		With(fb).
		Context(setup).
		Get(testutil.GetFn[any](nil, testutil.ErrInvalidArgument)).
		AssertFailure(1, 1, context.Canceled)
}
