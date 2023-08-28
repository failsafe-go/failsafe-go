package failsafe

import (
	"context"
	"time"
)

type ExecutionStats[R any] struct {
	Attempts   int
	Executions int
	StartTime  time.Time
}

func (s *ExecutionStats[R]) IsFirstAttempt() bool {
	return s.Attempts == 1
}

func (s *ExecutionStats[R]) IsRetry() bool {
	return s.Attempts > 1
}

func (s *ExecutionStats[R]) GetElapsedTime() time.Duration {
	return time.Since(s.StartTime)
}

type Execution[R any] struct {
	Context context.Context
	ExecutionStats[R]

	LastResult       R
	LastErr          error
	AttemptStartTime time.Time
}

func (e *Execution[_]) GetElapsedAttemptTime() time.Duration {
	return time.Since(e.AttemptStartTime)
}

func (e *Execution[R]) copy() *Execution[R] {
	execCopy := *e
	return &execCopy
}

type ExecutionResult[R any] struct {
	Result   R
	Err      error
	Complete bool
	Success  bool
}

// WithComplete marks the ExecutionResult as Complete.
func (er *ExecutionResult[R]) WithComplete(complete bool, success bool) *ExecutionResult[R] {
	c := *er
	c.Complete = complete
	c.Success = success
	return &c
}

// WithFailure marks the ExecutionResult as not complete or successful.
func (er *ExecutionResult[R]) WithFailure() *ExecutionResult[R] {
	c := *er
	c.Complete = false
	c.Success = false
	return &c
}

type ExecutionInternal[R any] struct {
	Execution[R]
}

//func (e *ExecutionInternal[R]) Copy() *ExecutionInternal[R] {
//	execCopy := *e
//	execCopy.attemptRecorded = false
//	return &execCopy
//}

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

type ExecutionAttemptedEvent[R any] struct {
	Execution[R]
}

type ExecutionScheduledEvent[R any] struct {
	Execution[R]
	Delay time.Duration
}

// GetDelay returns the Delay before the next execution event.
func (e *ExecutionScheduledEvent[R]) GetDelay() time.Duration {
	return e.Delay
}

type ExecutionCompletedEvent[R any] struct {
	Result R
	Err    error
	ExecutionStats[R]
}
