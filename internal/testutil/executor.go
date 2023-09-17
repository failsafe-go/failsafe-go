package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
)

type Given func() context.Context
type WhenRun[R any] func(execution failsafe.Execution[R]) error
type WhenGet[R any] func(execution failsafe.Execution[R]) (R, error)

func TestRunSuccess[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int) {
	testRun(t, given, executor, when, expectedAttempts, expectedExecutions, nil)
}

func TestRunFailure[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	testRun(t, given, executor, when, expectedAttempts, expectedExecutions, expectedError)
}

func TestGetSuccess[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R) {
	testGet(t, given, executor, when, expectedAttempts, expectedExecutions, expectedResult, nil)
}

func TestGetFailure[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	testGet(t, given, executor, when, expectedAttempts, expectedExecutions, *(new(R)), expectedError)
}

func testRun[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error) {
	var completedEvent *failsafe.ExecutionCompletedEvent[R]
	executor = executor.OnComplete(func(e failsafe.ExecutionCompletedEvent[R]) {
		completedEvent = &e
	})
	givenFn := func() {
		if given != nil {
			executor = executor.WithContext(given())
		}
	}
	assertResult := func(err error) {
		if expectedAttempts != -1 {
			assert.Equal(t, expectedAttempts, completedEvent.Attempts(), "expected attempts did not match")
		}
		if expectedExecutions != -1 {
			assert.Equal(t, expectedExecutions, completedEvent.Executions(), "expected executions did not match")
		}
		assert.ErrorIs(t, err, expectedError, "expected error did not match")
	}

	// Run sync
	givenFn()
	assertResult(executor.RunWithExecution(when))

	// Run async
	givenFn()
	assertResult(executor.RunWithExecutionAsync(when).Error())
}

func testGet[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error) {
	var completedEvent *failsafe.ExecutionCompletedEvent[R]
	executor = executor.OnComplete(func(e failsafe.ExecutionCompletedEvent[R]) {
		completedEvent = &e
	})
	givenFn := func() {
		if given != nil {
			executor = executor.WithContext(given())
		}
	}
	assertResult := func(result R, err error) {
		assert.Equal(t, expectedAttempts, completedEvent.Attempts(), "expected attempts did not match")
		assert.Equal(t, expectedExecutions, completedEvent.Executions(), "expected executions did not match")
		assert.Equal(t, expectedResult, result, "expected result did not match")
		assert.ErrorIs(t, err, expectedError, "expected error did not match")
	}

	// Run sync
	fmt.Println("Testing sync")
	givenFn()
	assertResult(executor.GetWithExecution(when))

	// Run async
	fmt.Println("\nTesting async")
	givenFn()
	assertResult(executor.GetWithExecutionAsync(when).Get())
}
