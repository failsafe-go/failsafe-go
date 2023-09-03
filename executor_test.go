package failsafe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"failsafe"
	"failsafe/fallback"
	"failsafe/internal/testutil"
	"failsafe/retrypolicy"
)

func TestGetWithSuccess(t *testing.T) {
	rp := retrypolicy.WithDefaults[string]()
	result, err := failsafe.With[string](rp).Get(func() (string, error) {
		return "test", nil
	})
	assert.Equal(t, "test", result)
	assert.Nil(t, err)
}

func TestGetWithFailure(t *testing.T) {
	rp := retrypolicy.WithDefaults[string]()
	result, err := failsafe.With[string](rp).Get(func() (string, error) {
		return "", testutil.InvalidArgumentError{}
	})
	assert.Empty(t, result)
	assert.ErrorIs(t, err, testutil.InvalidArgumentError{})
}

func TestGetWithExecution(t *testing.T) {
	rp := retrypolicy.WithDefaults[string]()
	fb := fallback.WithResult[string]("fallback")
	var lasteExec failsafe.Execution[string]
	result, err := failsafe.With[string](fb, rp).GetWithExecution(func(exec failsafe.Execution[string]) (string, error) {
		lasteExec = exec
		return "", testutil.InvalidArgumentError{}
	})
	assert.Equal(t, "fallback", result)
	assert.Nil(t, err)
	assert.Equal(t, lasteExec.Attempts, 3)
	assert.Equal(t, lasteExec.Executions, 2)
	assert.Equal(t, lasteExec.LastResult, "")
	assert.Equal(t, lasteExec.LastErr, testutil.InvalidArgumentError{})
}
