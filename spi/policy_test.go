package spi

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestIsFailureForNil(t *testing.T) {
	policy := BaseFailurePolicy[any]{}

	assert.False(t, policy.IsFailure(nil, nil))
}

func TestIsFailureForError(t *testing.T) {
	policy := BaseFailurePolicy[any]{}
	assert.True(t, policy.IsFailure(nil, errors.New("test")))
	assert.True(t, policy.IsFailure(nil, testutil.InvalidStateError{}))

	policy.HandleErrors(testutil.InvalidArgumentError{})
	assert.True(t, policy.IsFailure(nil, testutil.InvalidArgumentError{}))
	assert.False(t, policy.IsFailure(nil, errors.New("test")))
}

func TestIsFailureForResult(t *testing.T) {
	policy := BaseFailurePolicy[any]{}
	policy.HandleResult(10)

	assert.True(t, policy.IsFailure(10, nil))
	assert.False(t, policy.IsFailure(5, nil))
}

func TestIsFailureForPredicate(t *testing.T) {
	policy := BaseFailurePolicy[any]{}
	policy.HandleIf(func(result any, err error) bool {
		return result == "test" || errors.Is(err, testutil.InvalidArgumentError{})
	})

	assert.True(t, policy.IsFailure("test", nil))
	assert.False(t, policy.IsFailure(0, nil))
	assert.True(t, policy.IsFailure(nil, testutil.InvalidArgumentError{}))
	assert.False(t, policy.IsFailure(nil, testutil.InvalidStateError{}))
}

func TestShouldComputeDelay(t *testing.T) {
	expected := 5 * time.Millisecond
	policy := BaseDelayablePolicy[any]{
		DelayFn: func(exec failsafe.Execution[any]) time.Duration {
			return expected
		},
	}

	assert.Equal(t, expected, policy.ComputeDelay(testutil.TestExecution[any]{
		TheLastResult: true,
	}))
	assert.Equal(t, time.Duration(-1), policy.ComputeDelay(nil))
}
