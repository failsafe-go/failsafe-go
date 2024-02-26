package policy

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
	assert.True(t, policy.IsFailure(nil, testutil.ErrInvalidState))

	policy.HandleErrors(testutil.ErrInvalidArgument)
	assert.True(t, policy.IsFailure(nil, testutil.ErrInvalidArgument))
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
		return result == "test" || errors.Is(err, testutil.ErrInvalidArgument)
	})

	assert.True(t, policy.IsFailure("test", nil))
	assert.False(t, policy.IsFailure(0, nil))
	assert.True(t, policy.IsFailure(nil, testutil.ErrInvalidArgument))
	assert.False(t, policy.IsFailure(nil, testutil.ErrInvalidState))
}

func TestShouldComputeDelay(t *testing.T) {
	expected := 5 * time.Millisecond
	policy := BaseDelayablePolicy[any]{
		DelayFunc: func(exec failsafe.ExecutionAttempt[any]) time.Duration {
			return expected
		},
	}

	assert.Equal(t, expected, policy.ComputeDelay(testutil.TestExecution[any]{
		TheLastResult: true,
	}))
	assert.Equal(t, time.Duration(-1), policy.ComputeDelay(nil))
}

func TestIsAbortableNil(t *testing.T) {
	policy := BaseAbortablePolicy[any]{}

	assert.False(t, policy.IsAbortable(nil, nil))
}

func TestIsAbortableForError(t *testing.T) {
	policy := BaseAbortablePolicy[any]{}
	policy.AbortOnErrors(testutil.ErrInvalidArgument)

	assert.True(t, policy.IsAbortable(nil, testutil.ErrInvalidArgument))
	assert.True(t, policy.IsAbortable(nil, testutil.NewCompositeError(testutil.ErrInvalidArgument)))
	assert.False(t, policy.IsAbortable(nil, testutil.ErrConnecting))
}

func TestIsAbortableForResult(t *testing.T) {
	policy := BaseAbortablePolicy[any]{}
	policy.AbortOnResult(10)

	assert.True(t, policy.IsAbortable(10, nil))
	assert.False(t, policy.IsAbortable(5, nil))
	assert.False(t, policy.IsAbortable(5, testutil.ErrInvalidState))
}

func TestIsAbortableForPredicate(t *testing.T) {
	policy := BaseAbortablePolicy[any]{}
	policy.AbortIf(func(s any, err error) bool {
		return s == "test" || errors.Is(err, testutil.ErrInvalidArgument)
	})

	assert.True(t, policy.IsAbortable("test", nil))
	assert.False(t, policy.IsAbortable(0, nil))
	assert.True(t, policy.IsAbortable("", testutil.ErrInvalidArgument))
	assert.False(t, policy.IsAbortable("", testutil.ErrInvalidState))
}
