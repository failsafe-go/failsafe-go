package hedgepolicy

import (
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/budget"
	"github.com/failsafe-go/failsafe-go/common"
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
			final  bool
		}
		parentExecution := exec.(policy.ExecutionInternal[R])
		executions := make([]policy.ExecutionInternal[R], e.maxHedges+1)
		resultChan := make(chan *execResult, e.maxHedges+1)
		started, completed := 0, 0
		var lastResult *execResult

		if e.budget != nil {
			e.budget.RecordExecution()
			defer e.budget.ReleaseExecution()
		}

		// Waits for a result that hedging should cause hedging to be canceled, up to the timer, else returns nil. A nil timer waits
		// for the inflight executions to complete, then returns the last result received, which is never nil since at
		// least one execution is always started.
		awaitResult := func(timer <-chan time.Time) *execResult {
			for timer != nil || completed < started {
				select {
				case <-timer:
					return nil
				case result := <-resultChan:
					completed++
					lastResult = result
					if result.final {
						return result
					}
				}
			}
			return lastResult
		}

		for execIdx := 0; ; execIdx++ {
			// Prepare execution
			allowed := true
			if execIdx == 0 {
				executions[execIdx] = parentExecution.CopyForCancellable().(policy.ExecutionInternal[R])
			} else if e.budget != nil && !e.budget.TryAcquirePermit() {
				// Stop hedging when the hedge budget is exceeded
				e.budget.OnBudgetExceeded(budget.HedgeExecution, exec)
				allowed = false
			} else {
				executions[execIdx] = parentExecution.CopyForHedge().(policy.ExecutionInternal[R])

				if e.onHedge != nil {
					e.onHedge(failsafe.ExecutionEvent[R]{ExecutionAttempt: executions[execIdx].CopyWithResult(nil)})
				}
			}

			// Perform execution
			if allowed {
				started++
				go func(hedgeExec policy.ExecutionInternal[R], execIdx int) {
					startTime := time.Now()
					result := innerFn(hedgeExec)
					if execIdx > 0 && e.budget != nil {
						e.budget.ReleasePermit()
					}

					isDone := e.IsAbortable(result.Result, result.Error)

					// Record successful execution duration for quantile-based delay
					if isDone && e.quantile != nil {
						e.mu.Lock()
						e.quantile.Add(float64(time.Since(startTime)))
						e.mu.Unlock()
					}

					resultChan <- &execResult{result, execIdx, isDone}
				}(executions[execIdx], execIdx)
			}

			// Wait for result or hedge delay
			var result *execResult
			delay := e.delayFunc(exec)
			if allowed && execIdx < e.maxHedges && delay >= 0 {
				timer := time.NewTimer(delay)
				result = awaitResult(timer.C)
				timer.Stop()
			} else {
				result = awaitResult(nil)
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
