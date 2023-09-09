package spi

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
)

// PolicyExecutor handles execution and execution results according to a policy. May contain pre-execution and
// post-execution behaviors. Each PolicyExecutor makes its own determination about whether an execution result is a
// success or failure.
//
// Part of the Failsafe-go SPI.
type PolicyExecutor[R any] interface {
	// PreExecute is called before execution to return an alternative result or error, such as if execution is not allowed or
	// needed.
	PreExecute(exec ExecutionInternal[R]) *common.ExecutionResult[R]

	// Apply performs an execution by calling PreExecute and returning any result, else calling the innerFn PostExecute.
	//
	// If a PolicyExecutor delays or blocks during execution, it must check that the execution was not canceled in the
	// meantime, else return the ExecutionInternal.Result if it was.
	Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R]

	// PostExecute performs synchronous post-execution handling for an execution result.
	PostExecute(exec ExecutionInternal[R], result *common.ExecutionResult[R]) *common.ExecutionResult[R]

	// IsFailure returns whether the result is a failure according to the corresponding policy.
	IsFailure(result *common.ExecutionResult[R]) bool

	// OnSuccess performs post-execution handling for a result that is considered a success according to IsFailure.
	OnSuccess(exec ExecutionInternal[R], result *common.ExecutionResult[R])

	// OnFailure performs post-execution handling for a result that is considered a failure according to IsFailure, possibly
	// creating a new result, else returning the original result.
	OnFailure(exec ExecutionInternal[R], result *common.ExecutionResult[R]) *common.ExecutionResult[R]
}

// BasePolicyExecutor provides base implementation of PolicyExecutor.
type BasePolicyExecutor[R any] struct {
	PolicyExecutor[R]
	*BaseFailurePolicy[R]
	// Index of the policy relative to other policies in a composition, starting at 0 with the innermost policy.
	PolicyIndex int
}

var _ PolicyExecutor[any] = &BasePolicyExecutor[any]{}

func (bpe *BasePolicyExecutor[R]) PreExecute(_ ExecutionInternal[R]) *common.ExecutionResult[R] {
	return nil
}

func (bpe *BasePolicyExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R] {
	return func(exec failsafe.Execution[R]) *common.ExecutionResult[R] {
		execInternal := exec.(ExecutionInternal[R])
		result := bpe.PolicyExecutor.PreExecute(execInternal)
		if result != nil {
			return result
		}

		result = innerFn(exec)
		return bpe.PolicyExecutor.PostExecute(execInternal, result)
	}
}

func (bpe *BasePolicyExecutor[R]) PostExecute(exec ExecutionInternal[R], er *common.ExecutionResult[R]) *common.ExecutionResult[R] {
	if bpe.PolicyExecutor.IsFailure(er) {
		er = bpe.PolicyExecutor.OnFailure(exec, er.WithFailure())
	} else {
		er = er.WithComplete(true, true)
		bpe.PolicyExecutor.OnSuccess(exec, er)
	}
	return er
}

func (bpe *BasePolicyExecutor[R]) IsFailure(result *common.ExecutionResult[R]) bool {
	if bpe.BaseFailurePolicy != nil {
		return bpe.BaseFailurePolicy.IsFailure(result.Result, result.Error)
	}
	return result.Error != nil
}

func (bpe *BasePolicyExecutor[R]) OnSuccess(exec ExecutionInternal[R], result *common.ExecutionResult[R]) {
	if bpe.BaseFailurePolicy != nil && bpe.onSuccess != nil {
		bpe.onSuccess(failsafe.ExecutionEvent[R]{
			ExecutionAttempt: exec.ExecutionForResult(result),
		})
	}
}

func (bpe *BasePolicyExecutor[R]) OnFailure(exec ExecutionInternal[R], result *common.ExecutionResult[R]) *common.ExecutionResult[R] {
	if bpe.BaseFailurePolicy != nil && bpe.onFailure != nil {
		bpe.onFailure(failsafe.ExecutionEvent[R]{
			ExecutionAttempt: exec.ExecutionForResult(result),
		})
	}
	return result
}
