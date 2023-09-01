package failsafe

// ExecutionHandler returns an ExecutionResult for an ExecutionInternal.
//
// Part of the Failsafe-go SPI.
type ExecutionHandler[R any] func(*ExecutionInternal[R]) *ExecutionResult[R]

// PolicyExecutor handles execution and execution results according to a policy. May contain pre-execution and post-execution behaviors.
// Each PolicyExecutor makes its own determination about whether an execution result is a success or failure.
//
// Part of the Failsafe-go SPI.
type PolicyExecutor[R any] interface {
	// PreExecute is called before execution to return an alternative result or error, such as if execution is not allowed or needed.
	PreExecute() *ExecutionResult[R]

	// Apply performs an execution by calling PreExecute and returning any result, else calling the innerFn PostExecute.
	Apply(innerFn ExecutionHandler[R]) ExecutionHandler[R]

	// PostExecute performs synchronous post-execution handling for an execution result.
	PostExecute(exec *ExecutionInternal[R], result *ExecutionResult[R]) *ExecutionResult[R]

	// IsFailure returns whether the result is a failure according to the corresponding policy.
	IsFailure(result *ExecutionResult[R]) bool

	// OnSuccess performs post-execution handling for a result that is considered a success according to IsFailure.
	OnSuccess(result *ExecutionResult[R])

	// OnFailure performs post-execution handling for a result that is considered a failure according to IsFailure, possibly creating a new
	// result, else returning the original result.
	OnFailure(exec *Execution[R], result *ExecutionResult[R]) *ExecutionResult[R]
}

// BasePolicyExecutor provides base implementation of PolicyExecutor.
//
// Part of the Failsafe-go SPI.
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
