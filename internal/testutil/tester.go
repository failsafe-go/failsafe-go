package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
)

type ContextFn func() context.Context
type WhenRun[R any] func(execution failsafe.Execution[R]) error
type WhenGet[R any] func(execution failsafe.Execution[R]) (R, error)

type Resetable interface {
	Reset()
}

type Tester[R any] struct {
	T       *testing.T
	SetupFn func()
	ContextFn
	Policies []failsafe.Policy[R]
	Executor failsafe.Executor[R]
	run      WhenRun[R]
	get      WhenGet[R]
}

func Test[R any](t *testing.T) *Tester[R] {
	return &Tester[R]{
		T: t,
	}
}

func (t *Tester[R]) Setup(fn func()) *Tester[R] {
	t.SetupFn = fn
	return t
}

func (t *Tester[R]) Context(fn func() context.Context) *Tester[R] {
	t.ContextFn = fn
	return t
}

func (t *Tester[R]) Reset(stats ...Resetable) *Tester[R] {
	t.SetupFn = func() {
		for _, s := range stats {
			s.Reset()
		}
	}
	return t
}

func (t *Tester[R]) With(policies ...failsafe.Policy[R]) *Tester[R] {
	t.Policies = policies
	return t
}

func (t *Tester[R]) WithExecutor(executor failsafe.Executor[R]) *Tester[R] {
	t.Executor = executor
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
	t.assertResult(expectedAttempts, expectedExecutions, expectedResult, nil, true, false, then...)
}

func (t *Tester[R]) AssertSuccessError(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, *new(R), expectedError, true, false, then...)
}

func (t *Tester[R]) AssertFailure(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, *new(R), expectedError, false, false, then...)
}

func (t *Tester[R]) AssertFailureAs(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, *new(R), expectedError, false, true, then...)
}

func (t *Tester[R]) assertResult(expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error, expectedSuccess bool, errorAs bool, then ...func()) {
	t.T.Helper()
	if t.Executor == nil {
		t.Executor = failsafe.NewExecutor[R](t.Policies...)
	}
	test := func(async bool) {
		executorFn, assertFn := PrepareTest(t.T, t.SetupFn, t.ContextFn, t.Executor)

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

		assertFn(expectedAttempts, expectedExecutions, expectedResult, result, expectedError, err, expectedSuccess, !expectedSuccess, errorAs, then...)
	}

	// Run sync
	fmt.Println("Testing sync")
	test(false)

	// Run async
	fmt.Println("\nTesting async")
	test(true)
}

type AssertFunc[R any] func(expectedAttempts int, expectedExecutions int, expectedResult R, result R, expectedErr error, err error, expectedSuccess bool, expectedFailure bool, errorAs bool, thens ...func())

func PrepareTest[R any](t *testing.T, setupFn func(), contextFn ContextFn, executor failsafe.Executor[R]) (executorFn func() failsafe.Executor[R], assertFn AssertFunc[R]) {
	var doneEvent atomic.Pointer[failsafe.ExecutionDoneEvent[R]]
	var onSuccessCalled atomic.Bool
	var onFailureCalled atomic.Bool

	executorFn = func() failsafe.Executor[R] {
		if setupFn != nil {
			setupFn()
		}
		result := executor
		if contextFn != nil {
			if ctx := contextFn(); ctx != nil {
				result = result.WithContext(ctx)
			}
		}
		return result.OnDone(func(e failsafe.ExecutionDoneEvent[R]) {
			doneEvent.Store(&e)
		}).OnSuccess(func(e failsafe.ExecutionDoneEvent[R]) {
			onSuccessCalled.Store(true)
		}).OnFailure(func(e failsafe.ExecutionDoneEvent[R]) {
			onFailureCalled.Store(true)
		})
	}

	assertFn = func(expectedAttempts int, expectedExecutions int, expectedResult R, result R, expectedErr error, err error, expectedSuccess bool, expectedFailure bool, errorAs bool, thens ...func()) {
		for _, then := range thens {
			if then != nil {
				then()
			}
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
			if errorAs {
				assert.ErrorAs(t, err, expectedErr, "expected error did not match")
			} else {
				assert.ErrorIs(t, err, expectedErr, "expected error did not match")
			}
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
