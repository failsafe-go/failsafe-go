package adaptivethrottler

import (
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// executor is a policy.Executor that handles failures according to a AdaptiveThrottler.
type executor[R any] struct {
	*policy.BaseExecutor[R]
	*adaptiveThrottler[R]
}

var _ policy.Executor[any] = &executor[any]{}

func (e *executor[R]) PreExecute(_ policy.ExecutionInternal[R]) *common.PolicyResult[R] {
	if !e.TryAcquirePermit() {
		return internal.FailureResult[R](ErrExceeded)
	}
	return nil
}

func (e *executor[R]) OnSuccess(exec policy.ExecutionInternal[R], result *common.PolicyResult[R]) {
	e.BaseExecutor.OnSuccess(exec, result)
	e.RecordSuccess()
}

func (e *executor[R]) OnFailure(exec policy.ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R] {
	e.BaseExecutor.OnFailure(exec, result)
	e.RecordFailure()
	return result
}
