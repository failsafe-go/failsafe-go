package internal

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
)

func NewExecutionCompletedEventForExec[R any](exec failsafe.Execution[R]) failsafe.ExecutionCompletedEvent[R] {
	return failsafe.ExecutionCompletedEvent[R]{
		Result:         exec.LastResult(),
		Error:          exec.LastError(),
		ExecutionStats: exec,
	}
}

func FailureResult[R any](err error) *common.ExecutionResult[R] {
	return &common.ExecutionResult[R]{
		Error:    err,
		Complete: true,
	}
}
