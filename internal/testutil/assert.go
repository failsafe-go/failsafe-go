package testutil

import (
	"context"
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
	timer := time.NewTimer(waitDuration)
	select {
	case <-timer.C:
	case <-exec.Canceled():
		timer.Stop()
		assert.True(t, exec.IsCanceled())
		if exec.Context() != nil {
			assert.NotNil(t, exec.Context().Err(), "execution Context Err should be not nil")
		}
		return
	}
	assert.Fail(t, "Expected context to be canceled")
}

// SetupWithContextSleep returns a setup function that provides a context that is canceled after the sleepTime.
func SetupWithContextSleep(sleepTime time.Duration) func() context.Context {
	return func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(sleepTime)
			cancel()
		}()
		return ctx
	}
}
