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
	defaultR := *(new(R))
	executorFn, assertResult := prepareTest(t, given, executor, expectedAttempts, expectedExecutions, defaultR, expectedError)

	// Run sync
	fmt.Println("Testing sync")
	assertResult(defaultR, executorFn().RunWithExecution(when))

	// Run async
	fmt.Println("\nTesting async")
	assertResult(executorFn().RunWithExecutionAsync(when).Get())
}

func testGet[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error) {
	executorFn, assertResult := prepareTest(t, given, executor, expectedAttempts, expectedExecutions, expectedResult, expectedError)

	// Run sync
	fmt.Println("Testing sync")
	assertResult(executorFn().GetWithExecution(when))

	// Run async
	fmt.Println("\nTesting async")
	assertResult(executorFn().GetWithExecutionAsync(when).Get())
}

func prepareTest[R any](t *testing.T, given Given, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error) (executorFn func() failsafe.Executor[R], assertResult func(R, error)) {
	var doneEvent *failsafe.ExecutionDoneEvent[R]
	onSuccessCalled := false
	onFailureCalled := false
	executor = executor.OnDone(func(e failsafe.ExecutionDoneEvent[R]) {
		doneEvent = &e
	}).OnSuccess(func(e failsafe.ExecutionDoneEvent[R]) {
		onSuccessCalled = true
	}).OnFailure(func(e failsafe.ExecutionDoneEvent[R]) {
		onFailureCalled = true
	})
	executorFn = func() failsafe.Executor[R] {
		if given != nil {
			executor = executor.WithContext(given())
		}
		return executor
	}
	assertResult = func(result R, err error) {
		if expectedAttempts != -1 {
			assert.Equal(t, expectedAttempts, doneEvent.Attempts(), "expected attempts did not match")
		}
		if expectedExecutions != -1 {
			assert.Equal(t, expectedExecutions, doneEvent.Executions(), "expected executions did not match")
		}
		assert.Equal(t, expectedResult, result, "expected result did not match")
		assert.ErrorIs(t, err, expectedError, "expected error did not match")
		if err != nil {
			assert.True(t, onFailureCalled, "onFailure should have been called")
			assert.False(t, onSuccessCalled, "onSuccess should not have been called")
		} else {
			assert.False(t, onFailureCalled, "onFailure should not have been called")
			assert.True(t, onSuccessCalled, "onSuccess should have been called")
		}
	}
	return
}
