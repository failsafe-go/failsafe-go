package common

// ExecutionResult represents the internal result of an execution attempt for zero or more policies, before or after the
// policy has handled the result. If a policy is done handling a result or is no longer able to handle a result, such as
// when retries are exceeded, the ExecutionResult should be marked as complete.
type ExecutionResult[R any] struct {
	Result R
	Error  error
	// Complete indicates whether an execution is complete or if retries may be needed.
	Complete   bool
	Success    bool
	SuccessAll bool
}

// WithComplete returns a new Result for the complete and success values.
func (er *ExecutionResult[R]) WithComplete(complete bool, success bool) *ExecutionResult[R] {
	c := *er
	c.Complete = complete
	c.Success = success
	c.SuccessAll = success && c.SuccessAll
	return &c
}

// WithFailure returns a new Result that is marked as not successful.
func (er *ExecutionResult[R]) WithFailure() *ExecutionResult[R] {
	c := *er
	c.Success = false
	c.SuccessAll = false
	return &c
}
