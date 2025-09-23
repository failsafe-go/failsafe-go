package timeout

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// executor is a policy.Executor that handles failures according to a Timeout.
type executor[R any] struct {
	policy.BaseExecutor[R]
	*timeout[R]
}

var _ policy.Executor[any] = &executor[any]{}

func (e *executor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	// This func sets up a race between a timeout and the innerFn returning
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])

		// Create child context
		execInternal = execInternal.CopyForCancellable().(policy.ExecutionInternal[R])
		var result atomic.Pointer[common.PolicyResult[R]]
		timer := time.AfterFunc(e.timeLimit, func() {
			timeoutResult := internal.FailureResult[R](ErrExceeded)
			if result.CompareAndSwap(nil, timeoutResult) {
				if e.onTimeoutExceeded != nil {
					e.onTimeoutExceeded(failsafe.ExecutionDoneEvent[R]{
						ExecutionInfo: execInternal,
						Error:         ErrExceeded,
					})
				}

				// Sets the timeoutResult, overwriting any previously set result for the execution. This is correct, because while an
				// execution may have completed, inner policies such as fallbacks may still be processing that result, in which case
				// it's still important to interrupt them with a timeout.
				execInternal.Cancel(timeoutResult)
			}
		})

		// Store result and ctxCancel timeout context if needed
		if result.CompareAndSwap(nil, innerFn(execInternal)) {
			timer.Stop()
		}
		return e.PostExecute(execInternal, result.Load())
	}
}

func (e *executor[R]) IsFailure(_ R, err error) bool {
	return err != nil && errors.Is(err, ErrExceeded)
}
