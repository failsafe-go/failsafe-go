package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
	"failsafe/circuitbreaker"
	"failsafe/fallback"
	cbtesting "failsafe/internal/circuitbreaker_testutil"
	rptesting "failsafe/internal/retrypolicy_testutil"
	"failsafe/internal/testutil"
	"failsafe/ratelimiter"
	"failsafe/retrypolicy"
)

// RetryPolicy -> CircuitBreaker
func TestRetryPolicyCircuitBreaker(t *testing.T) {
	rp := retrypolicy.Builder[bool]().WithMaxRetries(-1).Build()
	cb := circuitbreaker.Builder[bool]().
		WithFailureThreshold(3).
		WithDelay(10 * time.Minute).
		Build()

	testutil.TestGetSuccess(t, failsafe.With[bool](rp, cb),
		testutil.ErrorNTimesThenReturn[bool](testutil.ConnectionError{}, 2, true),
		3, 3, true)
	assert.Equal(t, uint(1), cb.GetSuccessCount())
	assert.Equal(t, uint(2), cb.GetFailureCount())
}

// RetryPolicy -> CircuitBreaker
//
// Tests RetryPolicy with a CircuitBreaker that is open.
func TestRetryPolicyCircuitBreakerOpen(t *testing.T) {
	rp := rptesting.WithRetryLogs(retrypolicy.Builder[any]()).Build()
	cb := cbtesting.WithBreakerLogs(circuitbreaker.Builder[any]()).Build()

	testutil.TestRunFailure(t, failsafe.With[any](rp, cb),
		func(execution failsafe.Execution[any]) error {
			return errors.New("test")
		}, 3, 1, circuitbreaker.ErrCircuitBreakerOpen)
}

// CircuitBreaker -> RetryPolicy
func TestCircuitBreakerRetryPolicy(t *testing.T) {
	rp := retrypolicy.OfDefaults[any]()
	cb := circuitbreaker.Builder[any]().WithFailureThreshold(3).Build()

	testutil.TestRunFailure(t, failsafe.With[any](cb).Compose(rp),
		func(execution failsafe.Execution[any]) error {
			return testutil.InvalidStateError{}
		}, 3, 3, testutil.InvalidStateError{})
	assert.Equal(t, uint(0), cb.GetSuccessCount())
	assert.Equal(t, uint(1), cb.GetFailureCount())
	assert.True(t, cb.IsClosed())
}

// Fallback -> RetryPolicy
func TestFallbackRetryPolicy(t *testing.T) {
	// Given
	fb := fallback.OfResult[bool](true)
	rp := retrypolicy.OfDefaults[bool]()

	// When / Then
	testutil.TestGetSuccess[bool](t, failsafe.With[bool](fb, rp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		3, 3, true)

	// Given
	fb = fallback.OfFn[bool](func(e failsafe.ExecutionAttemptedEvent[bool]) (bool, error) {
		assert.False(t, e.LastResult)
		assert.ErrorIs(t, testutil.InvalidStateError{}, e.LastErr)
		return true, nil
	})

	// When / Then
	testutil.TestGetSuccess[bool](t, failsafe.With[bool](fb, rp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidStateError{}
		},
		3, 3, true)
}

// RetryPolicy -> Fallback
func TestRetryPolicyFallback(t *testing.T) {
	// Given
	rp := retrypolicy.OfDefaults[string]()
	fb := fallback.OfResult[string]("test")

	// When / Then
	testutil.TestGetSuccess[string](t, failsafe.With[string](rp).Compose(fb),
		func(execution failsafe.Execution[string]) (string, error) {
			return "", testutil.InvalidStateError{}
		},
		1, 1, "test")
}

// Fallback -> CircuitBreaker
//
// Tests fallback with a circuit breaker that is closed.
func TestFallbackCircuitBreaker(t *testing.T) {
	// Given
	fb := fallback.OfFn(func(e failsafe.ExecutionAttemptedEvent[bool]) (bool, error) {
		assert.False(t, e.LastResult)
		assert.ErrorIs(t, testutil.InvalidStateError{}, e.LastErr)
		return true, nil
	})
	cb := circuitbreaker.Builder[bool]().WithSuccessThreshold(3).Build()

	// When / Then
	testutil.TestGetSuccess(t, failsafe.With[bool](fb, cb),
		testutil.GetWithExecutionFn[bool](false, testutil.InvalidStateError{}),
		1, 1, true)
}

// Fallback -> CircuitBreaker
//
// Tests fallback with a circuit breaker that is open.
func TestFallbackCircuitBreakerOpen(t *testing.T) {
	// Given
	fb := fallback.OfFn(func(e failsafe.ExecutionAttemptedEvent[bool]) (bool, error) {
		assert.False(t, e.LastResult)
		assert.ErrorIs(t, circuitbreaker.ErrCircuitBreakerOpen, e.LastErr)
		return false, nil
	})
	cb := circuitbreaker.Builder[bool]().WithSuccessThreshold(3).Build()

	// When / Then
	cb.Open()
	testutil.TestGetSuccess(t, failsafe.With[bool](fb, cb),
		testutil.GetWithExecutionFn[bool](true, nil),
		1, 0, false)
}

// RetryPolicy -> RateLimiter
func TestRetryPolicyRateLimiter(t *testing.T) {
	rpStats := &testutil.Stats{}
	rp := rptesting.WithRetryStats(retrypolicy.Builder[any](), rpStats).WithMaxAttempts(7).Build()
	rl := ratelimiter.BurstyBuilder[any](3, 1*time.Second).Build()

	testutil.TestGetFailure(t, failsafe.With[any](rp, rl),
		testutil.GetWithExecutionFn[any](nil, testutil.InvalidStateError{}),
		7, 3, ratelimiter.ErrRateLimitExceeded)
	assert.Equal(t, 7, rpStats.FailedAttemptCount)
	assert.Equal(t, 6, rpStats.RetryCount)
}

// Fallback -> RetryPolicy -> CircuitBreaker
func TestFallbackRetryPolicyCircuitBreaker(t *testing.T) {
	rp := retrypolicy.OfDefaults[string]()
	cb := circuitbreaker.Builder[string]().WithFailureThreshold(5).Build()
	fb := fallback.OfResult[string]("test")

	testutil.TestGetSuccess(t, failsafe.With[string](fb).Compose(rp).Compose(cb),
		testutil.GetWithExecutionFn[string]("", testutil.InvalidStateError{}),
		3, 3, "test")
	assert.Equal(t, uint(0), cb.GetSuccessCount())
	assert.Equal(t, uint(3), cb.GetFailureCount())
	assert.True(t, cb.IsClosed())
}
