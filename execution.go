package failsafe

import (
	"context"
	"time"
)

// ExecutionStats contains stats for an execution.
type ExecutionStats[R any] struct {
	Attempts   int
	Executions int
	StartTime  time.Time
}

// IsFirstAttempt returns true when Attempts is 1 meaning this is the first execution attempt.
func (s *ExecutionStats[R]) IsFirstAttempt() bool {
	return s.Attempts == 1
}

// IsRetry returns true when Attempts is > 1 meaning the execution is being retried.
func (s *ExecutionStats[R]) IsRetry() bool {
	return s.Attempts > 1
}

// GetElapsedTime returns the elapsed time since initial execution began.
func (s *ExecutionStats[R]) GetElapsedTime() time.Duration {
	return time.Since(s.StartTime)
}

// Execution contains contextual information about an execution.
type Execution[R any] struct {
	Context context.Context
	ExecutionStats[R]

	LastResult       R
	LastErr          error
	AttemptStartTime time.Time
}

// GetElapsedAttemptTime returns the elapsed time since the last execution attempt began.
func (e *Execution[_]) GetElapsedAttemptTime() time.Duration {
	return time.Since(e.AttemptStartTime)
}

// ExecutionResult represents the internal result of an execution attempt for zero or more policies, before or after the policy has handled
// the result. If a policy is done handling a result or is no longer able to handle a result, such as when retries are exceeded, the
// ExecutionResult should be marked as complete.
//
// Part of the Failsafe-go SPI.
type ExecutionResult[R any] struct {
	Result   R
	Err      error
	Complete bool
	Success  bool
}

// WithComplete returns a new ExecutionResult that is marked as Complete.
func (er *ExecutionResult[R]) WithComplete(complete bool, success bool) *ExecutionResult[R] {
	c := *er
	c.Complete = complete
	c.Success = success
	return &c
}

// WithFailure returns a new ExecutionResult that is marked as not successful.
func (er *ExecutionResult[R]) WithFailure() *ExecutionResult[R] {
	c := *er
	c.Complete = false
	c.Success = false
	return &c
}

// ExecutionInternal contains internal execution APIs.
//
// Part of the Failsafe-go SPI.
type ExecutionInternal[R any] struct {
	Execution[R]
}

// InitializeAttempt marks the beginning of an execution attempt.
func (e *ExecutionInternal[R]) InitializeAttempt() {
	e.Attempts++
	e.AttemptStartTime = time.Now()
}

// recordAttempt records the result of an execution attempt.
func (e *ExecutionInternal[R]) recordAttempt(result *ExecutionResult[R]) {
	e.Executions++
	e.LastResult = result.Result
	e.LastErr = result.Err
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
	ExecutionStats[R]
}
