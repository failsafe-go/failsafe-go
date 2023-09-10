package failsafe_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestRunWithSuccess(t *testing.T) {
	rp := retrypolicy.WithDefaults[any]()
	err := failsafe.Run(func() error {
		return nil
	}, rp)
	assert.Nil(t, err)
}

func TestGetWithSuccess(t *testing.T) {
	rp := retrypolicy.WithDefaults[string]()
	result, err := failsafe.Get(func() (string, error) {
		return "test", nil
	}, rp)
	assert.Equal(t, "test", result)
	assert.Nil(t, err)
}

func TestGetWithFailure(t *testing.T) {
	rp := retrypolicy.WithDefaults[string]()
	result, err := failsafe.Get(func() (string, error) {
		return "", testutil.ErrInvalidArgument
	}, rp)

	assert.Empty(t, result)
	assert.ErrorIs(t, err, testutil.ErrInvalidArgument)
}

func TestGetWithExecution(t *testing.T) {
	rp := retrypolicy.WithDefaults[string]()
	fb := fallback.WithResult[string]("fallback")
	var lasteExec failsafe.Execution[string]
	result, err := failsafe.GetWithExecution(func(exec failsafe.Execution[string]) (string, error) {
		lasteExec = exec
		return "", testutil.ErrInvalidArgument
	}, fb, rp)

	assert.Equal(t, "fallback", result)
	assert.Nil(t, err)
	assert.Equal(t, 3, lasteExec.Attempts())
	assert.Equal(t, 2, lasteExec.Executions())
	assert.Equal(t, "", lasteExec.LastResult())
	assert.Equal(t, testutil.ErrInvalidArgument, lasteExec.LastError())
}

// Asserts that configuring a context returns a new copy of the Executor.
func TestWithContext(t *testing.T) {
	ctx1 := context.Background()
	ctx2 := context.Background()
	executor1 := failsafe.With[any](retrypolicy.WithDefaults[any]()).WithContext(ctx1)
	executor2 := executor1.WithContext(ctx2)
	assert.NotSame(t, executor1, executor2)
}

func TestExecutionWithNoPolicies(t *testing.T) {
	result, err := failsafe.Get(func() (string, error) {
		return "test", testutil.ErrInvalidArgument
	})

	assert.Equal(t, "test", result)
	assert.ErrorIs(t, testutil.ErrInvalidArgument, err)
}
