package failsafe

import (
	"context"
	"time"
)

// ExecutionStats contains stats for an execution.
type ExecutionStats struct {
	Attempts   int
	Executions int
	StartTime  time.Time
}

// IsFirstAttempt returns true when Attempts is 1 meaning this is the first execution attempt.
func (s *ExecutionStats) IsFirstAttempt() bool {
	return s.Attempts == 1
}

// IsRetry returns true when Attempts is > 1 meaning the execution is being retried.
func (s *ExecutionStats) IsRetry() bool {
	return s.Attempts > 1
}

// GetElapsedTime returns the elapsed time since initial execution began.
func (s *ExecutionStats) GetElapsedTime() time.Duration {
	return time.Since(s.StartTime)
}

// Execution contains contextual information about an execution.
type Execution[R any] struct {
	Context context.Context
	ExecutionStats

	LastResult       R
	LastErr          error
	AttemptStartTime time.Time
}

// GetElapsedAttemptTime returns the elapsed time since the last execution attempt began.
func (e *Execution[_]) GetElapsedAttemptTime() time.Duration {
	return time.Since(e.AttemptStartTime)
}

// ExecutionAttemptedEvent indicates an execution was attempted.
type ExecutionAttemptedEvent[R any] struct {
	Execution[R]
}

// ExecutionScheduledEvent indicates an execution was scheduled.
type ExecutionScheduledEvent[R any] struct {
	Execution[R]
	Delay time.Duration
}

// GetDelay returns the Delay before the next execution event.
func (e *ExecutionScheduledEvent[R]) GetDelay() time.Duration {
	return e.Delay
}

// ExecutionCompletedEvent indicates an execution was completed.
type ExecutionCompletedEvent[R any] struct {
	Result R
	Err    error
	ExecutionStats
}
