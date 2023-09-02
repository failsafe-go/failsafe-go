package internal

import (
	"failsafe"
)

func NewExecutionForResult[R any](result *failsafe.ExecutionResult[R], exec *failsafe.Execution[R]) failsafe.Execution[R] {
	return failsafe.Execution[R]{
		LastResult:       result.Result,
		LastErr:          result.Err,
		Context:          exec.Context,
		ExecutionStats:   exec.ExecutionStats,
		AttemptStartTime: exec.AttemptStartTime,
	}
}
