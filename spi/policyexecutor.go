package spi

import (
	"failsafe"
)

// BasePolicyExecutor provides base implementation of PolicyExecutor.
type BasePolicyExecutor[R any] struct {
	failsafe.PolicyExecutor[R]
	*BaseListenablePolicy[R]
	*BaseFailurePolicy[R]
}

var _ failsafe.PolicyExecutor[any] = &BasePolicyExecutor[any]{}

func (bpe *BasePolicyExecutor[R]) PreExecute() *failsafe.ExecutionResult[R] {
	return nil
}

func (bpe *BasePolicyExecutor[R]) Apply(innerFn failsafe.ExecutionHandler[R]) failsafe.ExecutionHandler[R] {
	return func(execInternal *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
		result := bpe.PolicyExecutor.PreExecute()
		if result != nil {
			return result
		}

		return bpe.PostExecute(execInternal, innerFn(execInternal))
	}
}

func (bpe *BasePolicyExecutor[R]) PostExecute(execInternal *failsafe.ExecutionInternal[R], er *failsafe.ExecutionResult[R]) *failsafe.ExecutionResult[R] {
	if bpe.IsFailure(er) {
		er = bpe.PolicyExecutor.OnFailure(&execInternal.Execution, er.WithFailure())
		if er.Complete && bpe.BaseListenablePolicy.failureListener != nil {
			bpe.BaseListenablePolicy.failureListener(failsafe.ExecutionCompletedEvent[R]{
				Result:         er.Result,
				Err:            er.Err,
				ExecutionStats: execInternal.ExecutionStats,
			})
		}
	} else {
		er = er.WithComplete(true, true)
		bpe.PolicyExecutor.OnSuccess(er)
		if er.Complete && bpe.BaseListenablePolicy.successListener != nil {
			bpe.BaseListenablePolicy.successListener(failsafe.ExecutionCompletedEvent[R]{
				Result:         er.Result,
				Err:            er.Err,
				ExecutionStats: execInternal.ExecutionStats,
			})
		}
	}
	return er
}

func (bpe *BasePolicyExecutor[R]) IsFailure(result *failsafe.ExecutionResult[R]) bool {
	if bpe.BaseFailurePolicy != nil {
		return bpe.BaseFailurePolicy.IsFailure(result.Result, result.Err)
	}
	return result.Err != nil
}

func (bpe *BasePolicyExecutor[R]) OnSuccess(_ *failsafe.ExecutionResult[R]) {
}

func (bpe *BasePolicyExecutor[R]) OnFailure(_ *failsafe.Execution[R], result *failsafe.ExecutionResult[R]) *failsafe.ExecutionResult[R] {
	return result
}
