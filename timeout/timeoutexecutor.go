package timeout

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/common"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/policy"
)

// timeoutExecutor is a failsafe.Executor that handles failures according to a Timeout.
type timeoutExecutor[R any] struct {
	*policy.BaseExecutor[R]
	*timeout[R]
}

var _ policy.Executor[any] = &timeoutExecutor[any]{}

func (e *timeoutExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.PolicyResult[R]) func(failsafe.Execution[R]) *common.PolicyResult[R] {
	// This func sets up a race between a timeout and the innerFn returning
	return func(exec failsafe.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])

		// Create child context if needed
		ctx := exec.Context()
		var ctxCancel func()
		if ctx != nil {
			ctx, ctxCancel = context.WithCancel(ctx)
			execInternal = execInternal.CopyWithContext(ctx).(policy.ExecutionInternal[R])
		}

		var result atomic.Pointer[common.PolicyResult[R]]
		timer := time.AfterFunc(e.config.timeLimit, func() {
			timeoutResult := internal.FailureResult[R](ErrExceeded)
			if result.CompareAndSwap(nil, timeoutResult) {
				if ctxCancel != nil {
					ctxCancel()
				}

				// Sets the timeoutResult, overwriting any previously set result for the execution. This is correct because while a
				// result may have been recorded, inner policies such as fallbacks may still be processing that result, in which case
				// it's still important to interrupt them with a timeout.
				execInternal.Cancel(e.PolicyIndex, timeoutResult)
				if e.config.onTimeoutExceeded != nil {
					e.config.onTimeoutExceeded(failsafe.ExecutionDoneEvent[R]{
						ExecutionStats: execInternal,
						Error:          ErrExceeded,
					})
				}
			}
		})

		// Store result and ctxCancel timeout context if needed
		if result.CompareAndSwap(nil, innerFn(execInternal)) {
			timer.Stop()
			if ctxCancel != nil {
				ctxCancel()
			}
		}
		return e.PostExecute(execInternal, result.Load())
	}
}

func (e *timeoutExecutor[R]) IsFailure(_ R, err error) bool {
	return err != nil && errors.Is(err, ErrExceeded)
}
