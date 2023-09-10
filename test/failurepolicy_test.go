package test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestHandleRetriesExceededError(t *testing.T) {
	fb := fallback.BuilderWithResult(true).
		HandleErrors(retrypolicy.ErrRetriesExceeded, testutil.ErrInvalidArgument).
		Build()
	rp := retrypolicy.WithDefaults[bool]()

	result, err := failsafe.Get(func() (bool, error) {
		return false, errors.New("test")
	}, fb, rp)
	assert.True(t, result)
	assert.Nil(t, err)
}
