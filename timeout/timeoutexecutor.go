package timeout

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

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
	// This func sets up a race between a timeout context, the execution's context, and the innerFn returning.
	return func(exec *failsafe.ExecutionInternal[R]) *failsafe.ExecutionResult[R] {
		var result atomic.Pointer[failsafe.ExecutionResult[R]]
		timeoutCtx, timeoutCancelFn := context.WithTimeout(context.Background(), e.config.timeoutDelay)

		go func() {
			select {
			case <-timeoutCtx.Done():
				if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
					// Timeout exceeded
					fmt.Println("Timeout fired") // TODO remove
					timeoutResult := internal.FailureResult[R](ErrTimeoutExceeded)
					if result.CompareAndSwap(nil, timeoutResult) {
						exec.Cancel(e, timeoutResult)
					}
				}

			case <-exec.Context.Done():
				// Execution context completed
				if result.CompareAndSwap(nil, internal.FailureResult[R](exec.Context.Err())) {
					timeoutCancelFn()
				}
			}
		}()

		// Store result and cancel timeout context if needed
		if result.CompareAndSwap(nil, innerFn(exec)) {
			fmt.Println("Execution done! canceling timeout") // TODO remove
			timeoutCancelFn()
		}
		return e.PostExecute(exec, result.Load())
	}
}

func (e *timeoutExecutor[R]) IsFailure(result *failsafe.ExecutionResult[R]) bool {
	return result.Err != nil && errors.Is(result.Err, ErrTimeoutExceeded)
}