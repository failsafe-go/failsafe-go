package budget

import (
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// executor is a policy.Executor that constrains executions according to a Budget.
type executor[R any] struct {
	*policy.BaseExecutor[R]
	*budget[R]
}

var _ policy.Executor[any] = &executor[any]{}

func (e *executor[R]) PreExecute(exec policy.ExecutionInternal[R]) *common.PolicyResult[R] {
	var err error

	if exec.IsRetry() {
		err = e.AcquireRetryPermit()
	} else if exec.IsHedge() {
		err = e.AcquireHedgePermit()
	}
	if err != nil {
		if e.onBudgetExceeded != nil {
			e.onBudgetExceeded(failsafe.ExecutionEvent[R]{
				ExecutionAttempt: exec,
			})
		}
		return internal.FailureResult[R](ErrExceeded)
	}

	return nil
}

func (e *executor[R]) PostExecute(exec policy.ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R] {
	if exec.IsRetry() {
		e.ReleaseRetryPermit()
	} else if exec.IsHedge() {
		e.ReleaseHedgePermit()
	}
	return result
}
