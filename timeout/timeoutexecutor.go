package timeout

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/spi"
)

// timeoutExecutor is a failsafe.PolicyExecutor that handles failures according to a Timeout.
type timeoutExecutor[R any] struct {
	*spi.BasePolicyExecutor[R]
	*timeout[R]
}

var _ failsafe.PolicyExecutor[any] = &timeoutExecutor[any]{}

func (e *timeoutExecutor[R]) Apply(innerFn failsafe.ExecutionHandler[R]) failsafe.ExecutionHandler[R] {
	// This func sets up a race between a timeout and the innerFn returning
	return func(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
		var result atomic.Pointer[failsafe.ExecutionResult[R]]

		timer := time.AfterFunc(e.config.timeoutDelay, func() {
			timeoutResult := internal.FailureResult[R](ErrTimeoutExceeded)
			if result.CompareAndSwap(nil, timeoutResult) {
				exec.Cancel(e.PolicyIndex, timeoutResult)
				if e.config.onTimeoutExceeded != nil {
					e.config.onTimeoutExceeded(failsafe.ExecutionCompletedEvent[R]{
						ExecutionStats: exec.ExecutionStats,
						Error:          ErrTimeoutExceeded,
					})
				}
			}
		})

		// Store result and cancel timeout context if needed
		if result.CompareAndSwap(nil, innerFn(exec)) {
			timer.Stop()
		}
		return e.PostExecute(exec, result.Load())
	}
}

func (e *timeoutExecutor[R]) IsFailure(result *failsafe.ExecutionResult[R]) bool {
	return result.Error != nil && errors.Is(result.Error, ErrTimeoutExceeded)
}
