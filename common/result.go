package common

// PolicyResult represents an execution result for a policy. If a policy is done handling a result or is no longer able
// to handle a result, such as when retries are exceeded, the PolicyResult should be marked as complete.
type PolicyResult[R any] struct {
	Result R
	Error  error
	// Complete indicates whether an execution is complete or if retries may be needed.
	Complete bool
	// Success indicates that a failure did not occur, or the policy was successful in handling the failure/
	Success bool
	// SuccessAll indicates whether the policy and all inner policies were successful.
	SuccessAll bool
}

// WithComplete returns a new Result for the complete and success values.
func (er *PolicyResult[R]) WithComplete(complete bool, success bool) *PolicyResult[R] {
	c := *er
	c.Complete = complete
	c.Success = success
	c.SuccessAll = success && c.SuccessAll
	return &c
}

// WithFailure returns a new Result that is marked as not successful.
func (er *PolicyResult[R]) WithFailure() *PolicyResult[R] {
	c := *er
	c.Success = false
	c.SuccessAll = false
	return &c
}
