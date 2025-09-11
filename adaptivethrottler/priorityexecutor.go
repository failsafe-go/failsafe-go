package adaptivethrottler

import (
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// priorityExecutor is a policy.Executor that handles failures according to a PriorityThrottler.
type priorityExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*priorityThrottler[R]
}

var _ policy.Executor[any] = &priorityExecutor[any]{}

func (e *priorityExecutor[R]) PreExecute(exec policy.ExecutionInternal[R]) *common.PolicyResult[R] {
	if err := e.AcquirePermit(exec.Context()); err != nil {
		return internal.FailureResult[R](err)
	}
	return nil
}

func (e *priorityExecutor[R]) OnSuccess(exec policy.ExecutionInternal[R], result *common.PolicyResult[R]) {
	e.BaseExecutor.OnSuccess(exec, result)
	e.RecordSuccess()
}

func (e *priorityExecutor[R]) OnFailure(exec policy.ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R] {
	e.BaseExecutor.OnFailure(exec, result)
	e.RecordFailure()
	return result
}
