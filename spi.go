package failsafe

import (
	"context"
	"time"
)

// ExecutionResult represents the internal result of an execution attempt for zero or more policies, before or after the policy has handled
// the result. If a policy is done handling a result or is no longer able to handle a result, such as when retries are exceeded, the
// ExecutionResult should be marked as complete.
//
// Part of the Failsafe-go SPI.
type ExecutionResult[R any] struct {
	Result R
	Error  error
	// Complete indicates whether an execution is complete or if retries may be needed.
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
	result *ExecutionResult[R]
	context.Context
}

func (e *ExecutionInternal[R]) ExecutionForResult(result *ExecutionResult[R]) Execution[R] {
	c := e.Execution
	c.LastResult = result.Result
	c.LastError = result.Error
	return c
}

// InitializeAttempt prepares a new execution attempt. Returns false if the attempt could not be initialized since it was canceled by an
// external Context or a timeout.Timeout composed outside the policyExecutor.
func (e *ExecutionInternal[R]) InitializeAttempt(policyIndex int) bool {
	// Lock to guard against a race with a Timeout canceling the execution
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.isCanceled(policyIndex) {
		return false
	}
	e.Attempts++
	e.AttemptStartTime = time.Now()
	if e.isCanceled(-1) {
		*e.canceledIndex = -1
		e.canceled = make(chan any)
	}
	return true
}

// Record records the result of an execution attempt, if a result was not already recorded, and returns the recorded execution.
func (e *ExecutionInternal[R]) Record(result *ExecutionResult[R]) *ExecutionResult[R] {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return e.record(result)
}

// Requires locking externally.
func (e *ExecutionInternal[R]) record(result *ExecutionResult[R]) *ExecutionResult[R] {
	if !e.isCanceled(-1) {
		e.result = result
		e.LastResult = result.Result
		e.LastError = result.Error
	}
	return e.result
}

// Cancel marks the execution as having been cancelled by the policyExecutor, which will also cancel pending executions of any inner
// policies of the policyExecutor, and also records the result. Outer policies of the policyExecutor will be unaffected.
func (e *ExecutionInternal[R]) Cancel(policyIndex int, result *ExecutionResult[R]) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if e.isCanceled(-1) {
		return
	}
	e.result = result
	e.LastResult = result.Result
	e.LastError = result.Error
	*e.canceledIndex = policyIndex
	if e.canceled != nil {
		close(e.canceled)
	}
}

// IsCanceled returns whether the execution has been canceled by an external Context or a policy composed outside the policyExecutor.
func (e *ExecutionInternal[R]) IsCanceled(policyIndex int) bool {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return e.isCanceled(policyIndex)
}

// Returns whether the execution has been canceled by a policy composed outside the policyExecutor.
// Requires locking externally.
func (e *ExecutionInternal[R]) isCanceled(policyIndex int) bool {
	return *e.canceledIndex > policyIndex
}

// GetResult returns the last recorded result. This should be called to fetch the last recorded result from the execution if the execution
// was canceled.
func (e *ExecutionInternal[R]) GetResult() *ExecutionResult[R] {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return e.result
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
	//
	// If a PolicyExecutor delays or blocks during execution, it must check that the execution was not canceled in the meantime, else
	// return the ExecutionInternal.GetResult if it was.
	Apply(innerFn ExecutionHandler[R]) ExecutionHandler[R]

	// PostExecute performs synchronous post-execution handling for an execution result.
	PostExecute(exec *ExecutionInternal[R], result *ExecutionResult[R]) *ExecutionResult[R]

	// IsFailure returns whether the result is a failure according to the corresponding policy.
	IsFailure(result *ExecutionResult[R]) bool

	// OnSuccess performs post-execution handling for a result that is considered a success according to IsFailure.
	OnSuccess(exec *ExecutionInternal[R], result *ExecutionResult[R])

	// OnFailure performs post-execution handling for a result that is considered a failure according to IsFailure, possibly creating a new
	// result, else returning the original result.
	OnFailure(exec *ExecutionInternal[R], result *ExecutionResult[R]) *ExecutionResult[R]
}
