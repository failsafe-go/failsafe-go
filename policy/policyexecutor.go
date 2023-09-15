package policy

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
)

// Executor handles execution and execution results according to a policy. May contain pre-execution and
// post-execution behaviors. Each Executor makes its own determination about whether an execution result is a
// success or failure.
type Executor[R any] interface {
	// PreExecute is called before execution to return an alternative result or error, such as if execution is not allowed or
	// needed.
	PreExecute(exec ExecutionInternal[R]) *common.ExecutionResult[R]

	// Apply performs an execution by calling PreExecute and returning any result, else calling the innerFn PostExecute.
	//
	// If a Executor delays or blocks during execution, it must check that the execution was not canceled in the
	// meantime, else return the ExecutionInternal.Result if it was.
	Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R]

	// PostExecute performs synchronous post-execution handling for an execution result.
	PostExecute(exec ExecutionInternal[R], result *common.ExecutionResult[R]) *common.ExecutionResult[R]

	// IsFailure returns whether the result is a failure according to the corresponding policy.
	IsFailure(result R, err error) bool

	// OnSuccess performs post-execution handling for a result that is considered a success according to IsFailure.
	OnSuccess(exec ExecutionInternal[R], result *common.ExecutionResult[R])

	// OnFailure performs post-execution handling for a result that is considered a failure according to IsFailure, possibly
	// creating a new result, else returning the original result.
	OnFailure(exec ExecutionInternal[R], result *common.ExecutionResult[R]) *common.ExecutionResult[R]
}

// BaseExecutor provides base implementation of Executor.
type BaseExecutor[R any] struct {
	Executor[R]
	*BaseFailurePolicy[R]
	// Index of the policy relative to other policies in a composition, starting at 0 with the innermost policy.
	PolicyIndex int
}

var _ Executor[any] = &BaseExecutor[any]{}

func (e *BaseExecutor[R]) PreExecute(_ ExecutionInternal[R]) *common.ExecutionResult[R] {
	return nil
}

func (e *BaseExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R] {
	return func(exec failsafe.Execution[R]) *common.ExecutionResult[R] {
		execInternal := exec.(ExecutionInternal[R])
		result := e.Executor.PreExecute(execInternal)
		if result != nil {
			return result
		}

		result = innerFn(exec)
		return e.Executor.PostExecute(execInternal, result)
	}
}

func (e *BaseExecutor[R]) PostExecute(exec ExecutionInternal[R], er *common.ExecutionResult[R]) *common.ExecutionResult[R] {
	if e.Executor.IsFailure(er.Result, er.Error) {
		er = e.Executor.OnFailure(exec, er.WithFailure())
	} else {
		er = er.WithComplete(true, true)
		e.Executor.OnSuccess(exec, er)
	}
	return er
}

func (e *BaseExecutor[R]) IsFailure(result R, err error) bool {
	if e.BaseFailurePolicy != nil {
		return e.BaseFailurePolicy.IsFailure(result, err)
	}
	return err != nil
}

func (e *BaseExecutor[R]) OnSuccess(exec ExecutionInternal[R], result *common.ExecutionResult[R]) {
	if e.BaseFailurePolicy != nil && e.onSuccess != nil {
		e.onSuccess(failsafe.ExecutionEvent[R]{
			ExecutionAttempt: exec.ExecutionForResult(result),
		})
	}
}

func (e *BaseExecutor[R]) OnFailure(exec ExecutionInternal[R], result *common.ExecutionResult[R]) *common.ExecutionResult[R] {
	if e.BaseFailurePolicy != nil && e.onFailure != nil {
		e.onFailure(failsafe.ExecutionEvent[R]{
			ExecutionAttempt: exec.ExecutionForResult(result),
		})
	}
	return result
}
