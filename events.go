package failsafe

import (
	"time"

	"github.com/failsafe-go/failsafe-go/common"
)

// ExecutionEvent indicates an execution was attempted.
type ExecutionEvent[R any] struct {
	ExecutionAttempt[R]
}

// ExecutionScheduledEvent indicates an execution was scheduled.
type ExecutionScheduledEvent[R any] struct {
	ExecutionAttempt[R]
	// The delay before the next execution attempt.
	Delay time.Duration
}

// ExecutionCompletedEvent indicates an execution was completed.
type ExecutionCompletedEvent[R any] struct {
	ExecutionStats
	// The execution result, else the zero value for R
	Result R
	// The execution error, else nil
	Error error
}

func newExecutionCompletedEvent[R any](er *common.ExecutionResult[R], stats ExecutionStats) ExecutionCompletedEvent[R] {
	return ExecutionCompletedEvent[R]{
		Result:         er.Result,
		Error:          er.Error,
		ExecutionStats: stats,
	}
}
