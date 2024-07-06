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

type Resetable interface {
	Reset()
}

type Tester[R any] struct {
	t *testing.T

	// Given
	given func() context.Context

	// When
	executor failsafe.Executor[R]
	run      WhenRun[R]
	get      WhenGet[R]

	// Then
	then               func()
	expectedAttempts   int
	expectedExecutions int
	expectedResult     R
	expectedError      error
	expectedSuccess    bool
	expectedFailure    bool
}

func Test[R any](t *testing.T) *Tester[R] {
	return &Tester[R]{
		t: t,
	}
}

func (t *Tester[R]) Setup(fn func()) *Tester[R] {
	t.given = func() context.Context {
		fn()
		return nil
	}
	return t
}

func (t *Tester[R]) Context(fn func() context.Context) *Tester[R] {
	t.given = fn
	return t
}

func (t *Tester[R]) Reset(stats ...Resetable) *Tester[R] {
	t.given = func() context.Context {
		for _, s := range stats {
			s.Reset()
		}
		return nil
	}
	return t
}

func (t *Tester[R]) With(policies ...failsafe.Policy[R]) *Tester[R] {
	t.executor = failsafe.NewExecutor[R](policies...)
	return t
}

func (t *Tester[R]) WithExecutor(executor failsafe.Executor[R]) *Tester[R] {
	t.executor = executor
	return t
}

func (t *Tester[R]) Run(when WhenRun[R]) *Tester[R] {
	t.run = when
	return t
}

func (t *Tester[R]) Get(when WhenGet[R]) *Tester[R] {
	t.get = when
	return t
}

func (t *Tester[R]) AssertSuccess(expectedAttempts int, expectedExecutions int, expectedResult R, then ...func()) {
	t.expectedSuccess = true
	t.expectedAttempts = expectedAttempts
	t.expectedExecutions = expectedExecutions
	t.expectedResult = expectedResult
	if len(then) > 0 {
		t.then = then[0]
	}
	t.do()
}

func (t *Tester[R]) AssertSuccessError(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.expectedError = expectedError
	t.AssertSuccess(expectedAttempts, expectedExecutions, *(new(R)), then...)
}

func (t *Tester[R]) AssertFailure(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.expectedFailure = true
	t.expectedAttempts = expectedAttempts
	t.expectedExecutions = expectedExecutions
	t.expectedError = expectedError
	if len(then) > 0 {
		t.then = then[0]
	}
	t.do()
}

func (t *Tester[R]) do() {
	test := func(async bool) {
		executorFn, assertFn := PrepareTest(t.t, t.given, t.executor)

		// Execute
		var result R
		var err error
		executor := executorFn()
		if t.run != nil {
			if !async {
				err = executor.RunWithExecution(t.run)
			} else {
				err = executor.RunWithExecutionAsync(t.run).Error()
			}
		} else {
			if !async {
				result, err = executor.GetWithExecution(t.get)
			} else {
				result, err = executor.GetWithExecutionAsync(t.get).Get()
			}
		}

		assertFn(t.expectedAttempts, t.expectedExecutions, t.expectedResult, result, t.expectedError, err, t.expectedSuccess, t.expectedFailure, t.then)
	}

	// Run sync
	fmt.Println("Testing sync")
	test(false)

	// Run async
	fmt.Println("\nTesting async")
	test(true)
}

type AssertFunc[R any] func(expectedAttempts int, expectedExecutions int, expectedResult R, result R, expectedErr error, err error, expectedSuccess bool, expectedFailure bool, then func())

func PrepareTest[R any](t *testing.T, given Given, executor failsafe.Executor[R]) (executorFn func() failsafe.Executor[R], assertFn AssertFunc[R]) {
	if given != nil {
		if ctx := given(); ctx != nil {
			executor = executor.WithContext(ctx)
		}
	}

	var doneEvent atomic.Pointer[failsafe.ExecutionDoneEvent[R]]
	var onSuccessCalled atomic.Bool
	var onFailureCalled atomic.Bool
	executorFn = func() failsafe.Executor[R] {
		return executor.OnDone(func(e failsafe.ExecutionDoneEvent[R]) {
			doneEvent.Store(&e)
		}).OnSuccess(func(e failsafe.ExecutionDoneEvent[R]) {
			onSuccessCalled.Store(true)
		}).OnFailure(func(e failsafe.ExecutionDoneEvent[R]) {
			onFailureCalled.Store(true)
		})
	}

	assertFn = func(expectedAttempts int, expectedExecutions int, expectedResult R, result R, expectedErr error, err error, expectedSuccess bool, expectedFailure bool, then func()) {
		if then != nil {
			then()
		}
		if doneEvent.Load() != nil {
			if expectedAttempts != -1 {
				assert.Equal(t, expectedAttempts, doneEvent.Load().Attempts(), "expected attempts did not match")
			}
			if expectedExecutions != -1 {
				assert.Equal(t, expectedExecutions, doneEvent.Load().Executions(), "expected executions did not match")
			}
		}
		assert.Equal(t, expectedResult, result, "expected result did not match")
		if expectedErr == nil {
			assert.Nil(t, err, " error should be nil")
		} else {
			assert.ErrorIs(t, err, expectedErr, "expected error did not match")
		}
		if expectedSuccess {
			assert.True(t, onSuccessCalled.Load(), "onSuccess should have been called")
			assert.False(t, onFailureCalled.Load(), "onFailure should not have been called")
		} else if expectedFailure {
			assert.False(t, onSuccessCalled.Load(), "onSuccess should not have been called")
			assert.True(t, onFailureCalled.Load(), "onFailure should have been called")
		}
	}

	return executorFn, assertFn
}
