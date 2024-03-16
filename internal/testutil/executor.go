package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
)

// Given performs pre-test setup that may involve resetting state so that the same fixtures can be used for sync and async tests.
type Given func() context.Context
type WhenRun[R any] func(execution failsafe.Execution[R]) error
type WhenGet[R any] func(execution failsafe.Execution[R]) (R, error)

func TestRunSuccess[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, then ...func()) {
	testRun(t, given, executor, when, expectedAttempts, expectedExecutions, nil, then...)
}

func TestRunFailure[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testRun(t, given, executor, when, expectedAttempts, expectedExecutions, expectedError, then...)
}

func TestGetSuccess[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, then ...func()) {
	testGet(t, given, executor, when, expectedAttempts, expectedExecutions, expectedResult, nil, then...)
}

func TestGetFailure[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testGet(t, given, executor, when, expectedAttempts, expectedExecutions, *(new(R)), expectedError, then...)
}

func testRun[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	defaultR := *(new(R))
	executorFn, assertResult := PrepareTest(t, given, executor, expectedAttempts, expectedExecutions, defaultR, &expectedError, expectedError == nil, then...)

	// Run sync
	fmt.Println("Testing sync")
	assertResult(defaultR, executorFn().RunWithExecution(when))

	// Run async
	fmt.Println("\nTesting async")
	assertResult(executorFn().RunWithExecutionAsync(when).Get())
}

func testGet[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error, then ...func()) {
	executorFn, assertResult := PrepareTest(t, given, executor, expectedAttempts, expectedExecutions, expectedResult, &expectedError, expectedError == nil, then...)

	// Run sync
	fmt.Println("Testing sync")
	assertResult(executorFn().GetWithExecution(when))

	// Run async
	fmt.Println("\nTesting async")
	assertResult(executorFn().GetWithExecutionAsync(when).Get())
}

func PrepareTest[R any](t *testing.T, given Given, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError *error, expectedSuccess bool, then ...func()) (executorFn func() failsafe.Executor[R], assertResult func(R, error)) {
	var doneEvent *failsafe.ExecutionDoneEvent[R]
	var onSuccessCalled atomic.Bool
	var onFailureCalled atomic.Bool
	executor = executor.OnDone(func(e failsafe.ExecutionDoneEvent[R]) {
		doneEvent = &e
	}).OnSuccess(func(e failsafe.ExecutionDoneEvent[R]) {
		onSuccessCalled.Store(true)
	}).OnFailure(func(e failsafe.ExecutionDoneEvent[R]) {
		onFailureCalled.Store(true)
	})
	executorFn = func() failsafe.Executor[R] {
		if given != nil {
			executor = executor.WithContext(given())
		}
		return executor
	}
	assertResult = func(result R, err error) {
		if len(then) > 0 && then[0] != nil {
			then[0]()
		}
		if doneEvent != nil {
			if expectedAttempts != -1 {
				assert.Equal(t, expectedAttempts, doneEvent.Attempts(), "expected attempts did not match")
			}
			if expectedExecutions != -1 {
				assert.Equal(t, expectedExecutions, doneEvent.Executions(), "expected executions did not match")
			}
		}

		assert.Equal(t, expectedResult, result, "expected result did not match")
		assert.ErrorIs(t, err, *expectedError, "expected error did not match")
		if expectedSuccess {
			assert.True(t, onSuccessCalled.Load(), "onSuccess should have been called")
			assert.False(t, onFailureCalled.Load(), "onFailure should not have been called")
		} else {
			assert.False(t, onSuccessCalled.Load(), "onSuccess should not have been called")
			assert.True(t, onFailureCalled.Load(), "onFailure should have been called")
		}
	}
	return
}
