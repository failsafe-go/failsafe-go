package test

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestHandleErrors(t *testing.T) {
	fb := fallback.NewBuilderWithResult(true).
		HandleErrors(io.EOF).
		Build()

	result, err := failsafe.With(fb).Get(func() (bool, error) {
		return false, io.EOF
	})
	assert.True(t, result)
	assert.Nil(t, err)
}

func TestHandleErrorsAs(t *testing.T) {
	fb := fallback.NewBuilderWithResult(true).
		HandleErrorTypes(testutil.CompositeError{}).
		Build()

	result, err := failsafe.With(fb).Get(func() (bool, error) {
		return false, testutil.CompositeError{Cause: errors.New("test")}
	})
	assert.True(t, result)
	assert.Nil(t, err)
}
