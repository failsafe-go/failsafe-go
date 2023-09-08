package internal

import "github.com/failsafe-go/failsafe-go"

func NewExecutionCompletedEvent[R any](er *failsafe.ExecutionResult[R], stats *failsafe.ExecutionStats) failsafe.ExecutionCompletedEvent[R] {
	return failsafe.ExecutionCompletedEvent[R]{
		Result:         er.Result,
		Err:            er.Err,
		ExecutionStats: *stats,
	}
}

func NewExecutionCompletedEventForExec[R any](exec *failsafe.Execution[R]) failsafe.ExecutionCompletedEvent[R] {
	return failsafe.ExecutionCompletedEvent[R]{
		Result:         exec.LastResult,
		Err:            exec.LastErr,
		ExecutionStats: exec.ExecutionStats,
	}
}

func FailureResult[R any](err error) *failsafe.ExecutionResult[R] {
	return &failsafe.ExecutionResult[R]{
		Err:      err,
		Complete: true,
	}
}
