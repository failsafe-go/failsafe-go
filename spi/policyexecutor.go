package spi

import (
	"github.com/failsafe-go/failsafe-go"
)

// BasePolicyExecutor provides base implementation of PolicyExecutor.
type BasePolicyExecutor[R any] struct {
	failsafe.PolicyExecutor[R]
	*BaseFailurePolicy[R]
	// Index of the policy relative to other policies in a composition, starting at 0 with the innermost policy.
	PolicyIndex int
}

var _ failsafe.PolicyExecutor[any] = &BasePolicyExecutor[any]{}

func (bpe *BasePolicyExecutor[R]) PreExecute(_ *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
	return nil
}

func (bpe *BasePolicyExecutor[R]) Apply(innerFn failsafe.ExecutionHandler[R]) failsafe.ExecutionHandler[R] {
	return func(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
		result := bpe.PolicyExecutor.PreExecute(exec)
		if result != nil {
			return result
		}

		result = innerFn(exec)
		return bpe.PolicyExecutor.PostExecute(exec, result)
	}
}

func (bpe *BasePolicyExecutor[R]) PostExecute(exec *failsafe.ExecutionInternal[R], er *failsafe.ExecutionResult[R]) *failsafe.ExecutionResult[R] {
	if bpe.PolicyExecutor.IsFailure(er) {
		er = bpe.PolicyExecutor.OnFailure(exec, er.WithFailure())
	} else {
		er = er.WithComplete(true, true)
		bpe.PolicyExecutor.OnSuccess(exec, er)
	}
	return er
}

func (bpe *BasePolicyExecutor[R]) IsFailure(result *failsafe.ExecutionResult[R]) bool {
	if bpe.BaseFailurePolicy != nil {
		return bpe.BaseFailurePolicy.IsFailure(result.Result, result.Error)
	}
	return result.Error != nil
}

func (bpe *BasePolicyExecutor[R]) OnSuccess(exec *failsafe.ExecutionInternal[R], result *failsafe.ExecutionResult[R]) {
	if bpe.BaseFailurePolicy != nil && bpe.onSuccess != nil {
		bpe.onSuccess(failsafe.ExecutionAttemptedEvent[R]{
			Execution: exec.ExecutionForResult(result),
		})
	}
}

func (bpe *BasePolicyExecutor[R]) OnFailure(exec *failsafe.ExecutionInternal[R], result *failsafe.ExecutionResult[R]) *failsafe.ExecutionResult[R] {
	if bpe.BaseFailurePolicy != nil && bpe.onFailure != nil {
		bpe.onFailure(failsafe.ExecutionAttemptedEvent[R]{
			Execution: exec.ExecutionForResult(result),
		})
	}
	return result
}

func (bpe *BasePolicyExecutor[R]) GetPolicyIndex() int {
	return bpe.PolicyIndex
}
