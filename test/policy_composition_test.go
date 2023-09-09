package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
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
	assert.Equal(t, uint(1), cb.SuccessCount())
	assert.Equal(t, uint(2), cb.FailureCount())
}

// RetryPolicy -> CircuitBreaker
//
// Tests RetryPolicy with a CircuitBreaker that is open.
func TestRetryPolicyCircuitBreakerOpen(t *testing.T) {
	rp := policytesting.WithRetryLogs(retrypolicy.Builder[any]()).Build()
	cb := policytesting.WithBreakerLogs(circuitbreaker.Builder[any]()).Build()

	testutil.TestRunFailure(t, failsafe.With[any](rp, cb),
		func(execution failsafe.Execution[any]) error {
			return errors.New("test")
		}, 3, 1, circuitbreaker.ErrCircuitBreakerOpen)
}

// CircuitBreaker -> RetryPolicy
func TestCircuitBreakerRetryPolicy(t *testing.T) {
	rp := retrypolicy.WithDefaults[any]()
	cb := circuitbreaker.Builder[any]().WithFailureThreshold(3).Build()

	testutil.TestRunFailure(t, failsafe.With[any](cb).Compose(rp),
		func(execution failsafe.Execution[any]) error {
			return testutil.InvalidStateError{}
		}, 3, 3, testutil.InvalidStateError{})
	assert.Equal(t, uint(0), cb.SuccessCount())
	assert.Equal(t, uint(1), cb.FailureCount())
	assert.True(t, cb.IsClosed())
}

// Fallback -> RetryPolicy
func TestFallbackRetryPolicy(t *testing.T) {
	// Given
	fb := fallback.WithResult[bool](true)
	rp := retrypolicy.WithDefaults[bool]()

	// When / Then
	testutil.TestGetSuccess[bool](t, failsafe.With[bool](fb, rp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		3, 3, true)

	// Given
	fb = fallback.WithFn[bool](func(exec failsafe.Execution[bool]) (bool, error) {
		assert.False(t, exec.LastResult())
		assert.ErrorIs(t, testutil.InvalidStateError{}, exec.LastError())
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
	rp := retrypolicy.WithDefaults[string]()
	fb := fallback.WithResult[string]("test")

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
	fb := fallback.WithFn(func(exec failsafe.Execution[bool]) (bool, error) {
		assert.False(t, exec.LastResult())
		assert.ErrorIs(t, testutil.InvalidStateError{}, exec.LastError())
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
	fb := fallback.WithFn(func(exec failsafe.Execution[bool]) (bool, error) {
		assert.False(t, exec.LastResult())
		assert.ErrorIs(t, circuitbreaker.ErrCircuitBreakerOpen, exec.LastError())
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
	rpStats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.Builder[any](), rpStats).WithMaxAttempts(7).Build()
	rl := ratelimiter.BurstyBuilder[any](3, 1*time.Second).Build()

	testutil.TestGetFailure(t, failsafe.With[any](rp, rl),
		testutil.GetWithExecutionFn[any](nil, testutil.InvalidStateError{}),
		7, 3, ratelimiter.ErrRateLimitExceeded)
	assert.Equal(t, 7, rpStats.ExecutionCount)
	assert.Equal(t, 6, rpStats.RetryCount)
}

// Fallback -> RetryPolicy -> CircuitBreaker
func TestFallbackRetryPolicyCircuitBreaker(t *testing.T) {
	rp := retrypolicy.WithDefaults[string]()
	cb := circuitbreaker.Builder[string]().WithFailureThreshold(5).Build()
	fb := fallback.WithResult[string]("test")

	testutil.TestGetSuccess(t, failsafe.With[string](fb).Compose(rp).Compose(cb),
		testutil.GetWithExecutionFn[string]("", testutil.InvalidStateError{}),
		3, 3, "test")
	assert.Equal(t, uint(0), cb.SuccessCount())
	assert.Equal(t, uint(3), cb.FailureCount())
	assert.True(t, cb.IsClosed())
}
