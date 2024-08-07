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
		execInternal := exec.(policy.ExecutionInternal[R])

		// Create a cancellable parent execution for all attempts
		parentExecution := execInternal.CopyForCancellable().(policy.ExecutionInternal[R])
		execInternal = parentExecution

		// Guard against a race between execution results
		done := atomic.Bool{}
		resultCount := atomic.Int32{}
		resultChan := make(chan *common.PolicyResult[R], 1) // Only the first result is sent

		for attempts := 1; ; attempts++ {
			go func(hedgeExec policy.ExecutionInternal[R]) {
				result := innerFn(hedgeExec)
				isFinalResult := int(resultCount.Add(1)) == e.maxHedges+1
				isCancellable := e.IsAbortable(result.Result, result.Error)

				if (isFinalResult || isCancellable) && done.CompareAndSwap(false, true) {
					// Cancel any outstanding attempts without recording a result
					if cancelResult := parentExecution.Cancel(nil); cancelResult != nil {
						result = cancelResult
					}
					resultChan <- result
				}
			}(execInternal)

			if attempts-1 < e.maxHedges {
				// Wait for hedge delay or result
				timer := time.NewTimer(e.delayFunc(exec))
				select {
				case <-timer.C:
				case result := <-resultChan:
					timer.Stop()
					return result
				}
			} else {
				// All hedges have been started, wait for a result
				select {
				case result := <-resultChan:
					return result
				}
			}

			if canceled, cancelResult := execInternal.IsCanceledWithResult(); canceled {
				return cancelResult
			}

			// Prepare for hedge execution
			execInternal = parentExecution.CopyForHedge().(policy.ExecutionInternal[R])

			// Call hedge listener
			if e.onHedge != nil {
				e.onHedge(failsafe.ExecutionEvent[R]{ExecutionAttempt: execInternal.CopyWithResult(nil)})
			}
		}
	}
}
