package failsafe

import (
	"time"
)

// ExecutionResult represents the internal result of an execution attempt for zero or more policies, before or after the policy has handled
// the result. If a policy is done handling a result or is no longer able to handle a result, such as when retries are exceeded, the
// ExecutionResult should be marked as complete.
//
// Part of the Failsafe-go SPI.
type ExecutionResult[R any] struct {
	Result     R
	Err        error
	Complete   bool
	Success    bool
	SuccessAll bool
}

// WithComplete returns a new ExecutionResult for the complete and success values.
func (er *ExecutionResult[R]) WithComplete(complete bool, success bool) *ExecutionResult[R] {
	c := *er
	c.Complete = complete
	c.Success = success
	c.SuccessAll = success && c.SuccessAll
	return &c
}

// WithFailure returns a new ExecutionResult that is marked as not successful.
func (er *ExecutionResult[R]) WithFailure() *ExecutionResult[R] {
	c := *er
	c.Success = false
	c.SuccessAll = false
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

// ExecutionHandler returns an ExecutionResult for an ExecutionInternal.
//
// Part of the Failsafe-go SPI.
type ExecutionHandler[R any] func(*ExecutionInternal[R]) *ExecutionResult[R]

// PolicyExecutor handles execution and execution results according to a policy. May contain pre-execution and post-execution behaviors.
// Each PolicyExecutor makes its own determination about whether an execution result is a success or failure.
//
// Part of the Failsafe-go SPI.
type PolicyExecutor[R any] interface {
	// PreExecute is called before execution to return an alternative result or error, such as if execution is not allowed or needed.
	PreExecute(exec *ExecutionInternal[R]) *ExecutionResult[R]

	// Apply performs an execution by calling PreExecute and returning any result, else calling the innerFn PostExecute.
	Apply(innerFn ExecutionHandler[R]) ExecutionHandler[R]

	// PostExecute performs synchronous post-execution handling for an execution result.
	PostExecute(exec *ExecutionInternal[R], result *ExecutionResult[R]) *ExecutionResult[R]

	// IsFailure returns whether the result is a failure according to the corresponding policy.
	IsFailure(result *ExecutionResult[R]) bool

	// OnSuccess performs post-execution handling for a result that is considered a success according to IsFailure.
	OnSuccess(result *ExecutionResult[R])

	// OnFailure performs post-execution handling for a result that is considered a failure according to IsFailure, possibly creating a new
	// result, else returning the original result.
	OnFailure(exec *Execution[R], result *ExecutionResult[R]) *ExecutionResult[R]
}
