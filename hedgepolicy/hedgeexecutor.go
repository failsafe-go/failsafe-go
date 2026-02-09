package hedgepolicy

import (
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/budget"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// executor is a policy.Executor that handles failures according to a HedgePolicy.
type executor[R any] struct {
	policy.BaseExecutor[R]
	*hedgePolicy[R]
}

var _ policy.Executor[any] = &executor[any]{}

func (e *executor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		type execResult struct {
			result *common.PolicyResult[R]
			index  int
		}
		parentExecution := exec.(policy.ExecutionInternal[R])
		executions := make([]policy.ExecutionInternal[R], e.maxHedges+1)

		// Guard against a race between execution results
		resultCount := atomic.Int32{}
		resultSent := atomic.Bool{}
		resultChan := make(chan *execResult, 1) // Only one result is sent

		for execIdx := 0; ; execIdx++ {
			// Prepare execution
			if execIdx == 0 {
				executions[execIdx] = parentExecution.CopyForCancellable().(policy.ExecutionInternal[R])
			} else {
				executions[execIdx] = parentExecution.CopyForHedge().(policy.ExecutionInternal[R])

				// Check the hedge budget, if any
				if e.budget != nil && !e.budget.TryAcquireHedgePermit() {
					e.budget.OnBudgetExceeded(budget.HedgeExecution, exec)
					return internal.FailureResult[R](budget.ErrExceeded)
				}

				if e.onHedge != nil {
					e.onHedge(failsafe.ExecutionEvent[R]{ExecutionAttempt: executions[execIdx].CopyWithResult(nil)})
				}
			}

			// Perform execution
			go func(hedgeExec policy.ExecutionInternal[R], execIdx int) {
				startTime := time.Now()
				result := innerFn(hedgeExec)
				if execIdx > 0 && e.budget != nil {
					e.budget.ReleaseHedgePermit()
				}
				isFinalResult := int(resultCount.Add(1)) == e.maxHedges+1
				isCancellable := e.IsAbortable(result.Result, result.Error)

				// Record successful execution duration for quantile-based delay
				if isCancellable && e.quantile != nil {
					e.mu.Lock()
					e.quantile.Add(float64(time.Since(startTime)))
					e.mu.Unlock()
				}

				if (isFinalResult || isCancellable) && resultSent.CompareAndSwap(false, true) {
					resultChan <- &execResult{result, execIdx}
				}
			}(executions[execIdx], execIdx)

			// Wait for result or hedge delay
			var result *execResult
			delay := e.delayFunc(exec)
			if execIdx < e.maxHedges && delay >= 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case result = <-resultChan:
					timer.Stop()
				}
			} else {
				result = <-resultChan
			}

			// Return if parent execution is canceled
			if canceled, cancelResult := parentExecution.IsCanceledWithResult(); canceled {
				return cancelResult
			}

			// Return result and cancel all attempts to cleanup their context references
			if result != nil {
				for i, execution := range executions {
					if execution != nil {
						if i == result.index {
							execution.Cancel(nil)
						} else {
							execution.Cancel(result.result)
						}
					}
				}
				return result.result
			}
		}
	}
}
