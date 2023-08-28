package failsafe

// ExecutionHandler returns a er for an execution.
type ExecutionHandler[R any] func(*ExecutionInternal[R]) *ExecutionResult[R]

// PolicyExecutor performs pre and post execution handling according to a policy.
type PolicyExecutor[R any] interface {
	PreExecute() *ExecutionResult[R]
	Apply(innerFn ExecutionHandler[R]) ExecutionHandler[R]
	PostExecute(exec *ExecutionInternal[R], result *ExecutionResult[R]) *ExecutionResult[R]
	IsFailure(result *ExecutionResult[R]) bool
	OnSuccess(result *ExecutionResult[R])
	OnFailure(exec *Execution[R], result *ExecutionResult[R]) *ExecutionResult[R]
}

// BasePolicyExecutor provides bese behavior for policy execution.
type BasePolicyExecutor[R any] struct {
	PolicyExecutor[R]
	*BaseListenablePolicy[R]
	*BaseFailurePolicy[R]
}

var _ PolicyExecutor[any] = &BasePolicyExecutor[any]{}

func (bpe *BasePolicyExecutor[R]) PreExecute() *ExecutionResult[R] {
	return nil
}

func (bpe *BasePolicyExecutor[R]) Apply(innerFn ExecutionHandler[R]) ExecutionHandler[R] {
	return func(execInternal *ExecutionInternal[R]) *ExecutionResult[R] {
		result := bpe.PolicyExecutor.PreExecute()
		if result != nil {
			return result
		}

		return bpe.PostExecute(execInternal, innerFn(execInternal))
	}
}

func (bpe *BasePolicyExecutor[R]) PostExecute(execInternal *ExecutionInternal[R], er *ExecutionResult[R]) *ExecutionResult[R] {
	if bpe.IsFailure(er) {
		er = bpe.PolicyExecutor.OnFailure(&execInternal.Execution, er.WithFailure())
		if er.Complete && bpe.BaseListenablePolicy.failureListener != nil {
			bpe.BaseListenablePolicy.failureListener(ExecutionCompletedEvent[R]{
				Result:         er.Result,
				Err:            er.Err,
				ExecutionStats: execInternal.ExecutionStats,
			})
		}
	} else {
		bpe.PolicyExecutor.OnSuccess(er)
		er = er.WithComplete(true, true)
		if er.Complete && bpe.BaseListenablePolicy.successListener != nil {
			bpe.BaseListenablePolicy.successListener(ExecutionCompletedEvent[R]{
				Result:         er.Result,
				Err:            er.Err,
				ExecutionStats: execInternal.ExecutionStats,
			})
		}
	}
	return er
}

func (bpe *BasePolicyExecutor[R]) IsFailure(result *ExecutionResult[R]) bool {
	if bpe.BaseFailurePolicy != nil {
		return bpe.BaseFailurePolicy.IsFailure(result.Result, result.Err)
	}
	return result.Err != nil
}

func (bpe *BasePolicyExecutor[R]) OnSuccess(_ *ExecutionResult[R]) {
}

func (bpe *BasePolicyExecutor[R]) OnFailure(_ *Execution[R], result *ExecutionResult[R]) *ExecutionResult[R] {
	return result
}
