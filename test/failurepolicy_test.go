package test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestHandleCustomErrorType(t *testing.T) {
	fb := fallback.BuilderWithResult(true).
		HandleErrors(&testutil.CompositeError{}).
		Build()

	result, err := failsafe.Get(func() (bool, error) {
		return false, testutil.NewCompositeError(errors.New("test"))
	}, fb)
	assert.True(t, result)
	assert.Nil(t, err)
}
