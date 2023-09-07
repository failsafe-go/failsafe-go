package failsafe

import (
	"sync"
	"time"
)

// ExecutionStats contains stats for an execution.
type ExecutionStats struct {
	// The number of execution attempts, including attempts that are currently in progress and attempts that were blocked before being
	// executed, such as by a CircuitBreaker or RateLimiter.
	Attempts int
	// The number of completed executions. Executions that are blocked, such as when a CircuitBreaker is open, are not counted.
	Executions int
	// The time that the initial execution attempt started at.
	StartTime time.Time
}

// IsFirstAttempt returns true when Attempts is 1, meaning this is the first execution attempt.
func (s *ExecutionStats) IsFirstAttempt() bool {
	return s.Attempts == 1
}

// IsRetry returns true when Attempts is > 1, meaning the execution is being retried.
func (s *ExecutionStats) IsRetry() bool {
	return s.Attempts > 1
}

// GetElapsedTime returns the elapsed time since initial execution attempt began.
func (s *ExecutionStats) GetElapsedTime() time.Duration {
	return time.Since(s.StartTime)
}

// Execution contains information about an execution.
type Execution[R any] struct {
	ExecutionStats
	// The time that the most recent execution attempt started at.
	AttemptStartTime time.Time
	mtx              *sync.Mutex

	// Guarded by mtx

	// The last error that occurred, else the zero value for R.
	LastResult R
	// The last error that occurred, else nil.
	LastErr       error
	canceled      chan any
	canceledIndex *int
}

// GetElapsedAttemptTime returns the elapsed time since the last execution attempt began.
func (e *Execution[_]) GetElapsedAttemptTime() time.Duration {
	return time.Since(e.AttemptStartTime)
}

// IsCanceled returns whether the execution has been canceled by an external Context or a timeout.Timeout.
func (e *Execution[_]) IsCanceled() bool {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return *e.canceledIndex > -1
}

// Canceled returns a channel that is done when the execution is canceled, either by an external Context or a timeout.Timeout.
func (e *Execution[_]) Canceled() <-chan any {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return e.canceled
}

// ExecutionAttemptedEvent indicates an execution was attempted.
type ExecutionAttemptedEvent[R any] struct {
	Execution[R]
}

// ExecutionScheduledEvent indicates an execution was scheduled.
type ExecutionScheduledEvent[R any] struct {
	Execution[R]
	// The delay before the next execution attempt.
	Delay time.Duration
}

// GetDelay returns the Delay before the next execution event.
func (e *ExecutionScheduledEvent[R]) GetDelay() time.Duration {
	return e.Delay
}

// ExecutionCompletedEvent indicates an execution was completed.
type ExecutionCompletedEvent[R any] struct {
	ExecutionStats
	// The execution result, else the zero value for R
	Result R
	// The execution error, else nil
	Err error
}

func newExecutionCompletedEvent[R any](er *ExecutionResult[R], stats *ExecutionStats) ExecutionCompletedEvent[R] {
	return ExecutionCompletedEvent[R]{
		Result:         er.Result,
		Err:            er.Err,
		ExecutionStats: *stats,
	}
}
