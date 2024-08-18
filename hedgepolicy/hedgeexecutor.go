package hedgepolicy

import (
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/policy"
)

// executor is a policy.Executor that handles failures according to a HedgePolicy.
type executor[R any] struct {
	*policy.BaseExecutor[R]
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
				if e.onHedge != nil {
					e.onHedge(failsafe.ExecutionEvent[R]{ExecutionAttempt: executions[execIdx].CopyWithResult(nil)})
				}
			}

			// Perform execution
			go func(hedgeExec policy.ExecutionInternal[R], execIdx int) {
				result := innerFn(hedgeExec)
				isFinalResult := int(resultCount.Add(1)) == e.maxHedges+1
				isCancellable := e.IsAbortable(result.Result, result.Error)
				if (isFinalResult || isCancellable) && resultSent.CompareAndSwap(false, true) {
					resultChan <- &execResult{result, execIdx}
				}
			}(executions[execIdx], execIdx)

			// Wait for result or hedge delay
			var result *execResult
			if execIdx < e.maxHedges {
				timer := time.NewTimer(e.delayFunc(exec))
				select {
				case <-timer.C:
				case result = <-resultChan:
					timer.Stop()
				}
			} else {
				select {
				case result = <-resultChan:
				}
			}

			// Return if parent execution is canceled
			if canceled, cancelResult := parentExecution.IsCanceledWithResult(); canceled {
				return cancelResult
			}

			// Return result and cancel any outstanding attempts
			if result != nil {
				for i, execution := range executions {
					if i != result.index && execution != nil {
						execution.Cancel(nil)
					}
				}
				return result.result
			}
		}
	}
}
