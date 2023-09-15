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

// timeoutExecutor is a failsafe.PolicyExecutor that handles failures according to a Timeout.
type timeoutExecutor[R any] struct {
	*policy.BasePolicyExecutor[R]
	*timeout[R]
}

var _ policy.PolicyExecutor[any] = &timeoutExecutor[any]{}

func (e *timeoutExecutor[R]) Apply(innerFn func(failsafe.Execution[R]) *common.ExecutionResult[R]) func(failsafe.Execution[R]) *common.ExecutionResult[R] {
	// This func sets up a race between a timeout and the innerFn returning
	return func(exec failsafe.Execution[R]) *common.ExecutionResult[R] {
		execInternal := exec.(policy.ExecutionInternal[R])
		var result atomic.Pointer[common.ExecutionResult[R]]

		timer := time.AfterFunc(e.config.timeoutDelay, func() {
			timeoutResult := internal.FailureResult[R](ErrTimeoutExceeded)
			if result.CompareAndSwap(nil, timeoutResult) {
				execInternal.Cancel(e.PolicyIndex, timeoutResult)
				if e.config.onTimeoutExceeded != nil {
					e.config.onTimeoutExceeded(failsafe.ExecutionCompletedEvent[R]{
						ExecutionStats: exec,
						Error:          ErrTimeoutExceeded,
					})
				}
			}
		})

		// Store result and cancel timeout context if needed
		if result.CompareAndSwap(nil, innerFn(exec)) {
			timer.Stop()
		}
		return e.PostExecute(execInternal, result.Load())
	}
}

func (e *timeoutExecutor[R]) IsFailure(_ R, err error) bool {
	return err != nil && errors.Is(err, ErrTimeoutExceeded)
}
