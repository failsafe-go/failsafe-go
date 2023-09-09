package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
)

type WhenGet[R any] func(execution failsafe.Execution[R]) (R, error)
type WhenRun[R any] func(execution failsafe.Execution[R]) error

func TestGetSuccess[R any](t *testing.T, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R) {
	testGet(t, executor, when, expectedAttempts, expectedExecutions, expectedResult, nil)
}

func TestGetFailure[R any](t *testing.T, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	testGet(t, executor, when, expectedAttempts, expectedExecutions, *(new(R)), expectedError)
}

func TestRunSuccess[R any](t *testing.T, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int) {
	testRun(t, executor, when, expectedAttempts, expectedExecutions, nil)
}

func TestRunFailure[R any](t *testing.T, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	testRun(t, executor, when, expectedAttempts, expectedExecutions, expectedError)
}

func testGet[R any](t *testing.T, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error) {
	var completedEvent *failsafe.ExecutionCompletedEvent[R]
	result, err := executor.OnComplete(func(e failsafe.ExecutionCompletedEvent[R]) {
		completedEvent = &e
	}).GetWithExecution(when)
	assert.Equal(t, expectedAttempts, completedEvent.Attempts(), "expected attempts did not match")
	assert.Equal(t, expectedExecutions, completedEvent.Executions(), "expected executions did not match")
	assert.Equal(t, expectedResult, result, "expected result did not match")
	assert.ErrorIs(t, expectedError, err, "expected error did not match")
}

func testRun[R any](t *testing.T, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	var completedEvent *failsafe.ExecutionCompletedEvent[R]
	err := executor.OnComplete(func(e failsafe.ExecutionCompletedEvent[R]) {
		completedEvent = &e
	}).RunWithExecution(when)
	if expectedAttempts != -1 {
		assert.Equal(t, expectedAttempts, completedEvent.Attempts(), "expected attempts did not match")
	}
	if expectedExecutions != -1 {
		assert.Equal(t, expectedExecutions, completedEvent.Executions(), "expected executions did not match")
	}
	assert.ErrorIs(t, expectedError, err, "expected error did not match")
}
