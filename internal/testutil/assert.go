package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
)

func AssertDuration(t *testing.T, expectedDuration int, actualDuration time.Duration) {
	assert.Equal(t, time.Duration(expectedDuration), actualDuration)
}

func (w *Waiter) AssertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return assert.Equal(t, expected, actual, msgAndArgs)
}

func WaitAndAssertCanceled[R any](t *testing.T, waitDuration time.Duration, exec failsafe.Execution[R]) {
	assert.False(t, exec.IsCanceled())
	select {
	case <-time.After(waitDuration):
	case <-exec.Canceled():
		assert.True(t, exec.IsCanceled())
		return
	}
	assert.Fail(t, "Expected context to be canceled by timeout")
}
