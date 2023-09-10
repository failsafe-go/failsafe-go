package internal

import (
	"github.com/failsafe-go/failsafe-go/common"
)

func FailureResult[R any](err error) *common.ExecutionResult[R] {
	return &common.ExecutionResult[R]{
		Error:    err,
		Complete: true,
	}
}
