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
	fb := fallback.BuilderWithResult(true).
		HandleErrors(io.EOF).
		Build()

	result, err := failsafe.Get(func() (bool, error) {
		return false, io.EOF
	}, fb)
	assert.True(t, result)
	assert.Nil(t, err)
}

func TestHandleErrorsAs(t *testing.T) {
	fb := fallback.BuilderWithResult(true).
		HandleErrorTypes(testutil.CompositeError{}).
		Build()

	result, err := failsafe.Get(func() (bool, error) {
		return false, testutil.CompositeError{Cause: errors.New("test")}
	}, fb)
	assert.True(t, result)
	assert.Nil(t, err)
}
