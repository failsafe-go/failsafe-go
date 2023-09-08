package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

/*
RetryPolicy -> RetryPolicy

Tests a scenario with nested retry policies where the inner policy is exceeded and skipped.
*/
func TestNestedRetryPoliciesWhereInnerIsExceeded(t *testing.T) {
	// Given
	outerRetryStats := &policytesting.Stats{}
	innerRetryStats := &policytesting.Stats{}
	outerRetryPolicy := policytesting.WithRetryStats(retrypolicy.Builder[bool]().WithMaxRetries(10), outerRetryStats).Build()
	innerRetryPolicy := policytesting.WithRetryStats(retrypolicy.Builder[bool]().WithMaxRetries(1), innerRetryStats).Build()

	// When / Then
	testutil.TestGetSuccess(t, failsafe.With[bool](outerRetryPolicy, innerRetryPolicy),
		testutil.ErrorNTimesThenReturn[bool](testutil.ConnectionError{}, 5, true),
		6, 6, true)
	assert.Equal(t, 4, outerRetryStats.FailedAttemptCount)
	assert.Equal(t, 0, outerRetryStats.FailureCount)
	assert.Equal(t, 2, innerRetryStats.FailedAttemptCount)
	assert.Equal(t, 1, innerRetryStats.FailureCount)
}

/*
Fallback -> RetryPolicy -> RetryPolicy
*/
func TestFallbackRetryPolicyRetryPolicy(t *testing.T) {
	// Given
	retryPolicy1Stats := &policytesting.Stats{}
	retryPolicy2Stats := &policytesting.Stats{}
	retryPolicy1 := policytesting.WithRetryStats(retrypolicy.Builder[any]().HandleErrors(testutil.InvalidStateError{}).WithMaxRetries(2), retryPolicy1Stats).Build()
	retryPolicy2 := policytesting.WithRetryStats(retrypolicy.Builder[any]().HandleErrors(testutil.InvalidArgumentError{}).WithMaxRetries(3), retryPolicy2Stats).Build()
	fb := fallback.WithResult[any](true)
	fn := func(exec failsafe.Execution[any]) (any, error) {
		if exec.Attempts%2 == 1 {
			return false, testutil.InvalidStateError{}
		}
		return false, testutil.InvalidArgumentError{}
	}

	// When / Then
	testutil.TestGetSuccess(t, failsafe.With[any](fb, retryPolicy2, retryPolicy1), fn,
		5, 5, true)
	// Expected RetryPolicy failure sequence:
	//    rp1 InvalidStateError - failure, retry
	//    rp1 InvalidArgumentError - success
	//    rp2 InvalidArgumentError - failure, retry
	//    rp1 InvalidStateError - failure, retry, retries exhausted
	//    rp1 InvalidArgumentError - success
	//    rp2 InvalidArgumentError - failure, retry
	//    rp1 InvalidStateError - failure, retries exceeded
	//    rp2 InvalidStateError - success
	assert.Equal(t, 3, retryPolicy1Stats.FailedAttemptCount)
	assert.Equal(t, 1, retryPolicy1Stats.FailureCount)
	assert.Equal(t, 2, retryPolicy2Stats.FailedAttemptCount)
	assert.Equal(t, 0, retryPolicy2Stats.FailureCount)

	// When / Then
	retryPolicy1Stats.Reset()
	retryPolicy2Stats.Reset()
	testutil.TestGetSuccess(t, failsafe.With[any](fb, retryPolicy1, retryPolicy2), fn,
		5, 5, true)
	// Expected RetryPolicy failure sequence:
	//    rp2 InvalidStateError - success
	//    rp1 InvalidStateError - failure, retry
	//    rp2 InvalidArgumentError - failure, retry
	//    rp2 InvalidStateError - success
	//    rp1 InvalidStateError - failure, retry, retries exhausted
	//    rp2 InvalidArgumentError - failure, retry
	//    rp2 InvalidStateError - success
	//    rp1 InvalidStateError - retries exceeded
	assert.Equal(t, 3, retryPolicy1Stats.FailedAttemptCount)
	assert.Equal(t, 1, retryPolicy1Stats.FailureCount)
	assert.Equal(t, 2, retryPolicy2Stats.FailedAttemptCount)
	assert.Equal(t, 0, retryPolicy2Stats.FailureCount)
}
