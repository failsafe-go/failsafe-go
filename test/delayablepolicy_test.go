package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestPanicInRetryPolicyDelayFunction(t *testing.T) {
	rp := retrypolicy.Builder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		panic("test")
	}).Build()

	assert.Panics(t, func() {
		failsafe.RunWithExecution(testutil.RunFn(errors.New("test")), rp)
	})
}

func TestShouldDelayRetryPolicy(t *testing.T) {
	delays := 0
	retryPolicy := retrypolicy.Builder[bool]().
		HandleResult(false).
		WithDelayFunc(func(exec failsafe.ExecutionAttempt[bool]) time.Duration {
			delays++ // side-effect for test purposes
			return 1
		}).
		Build()

	executor := failsafe.NewExecutor[bool](retryPolicy)
	executor.GetWithExecution(testutil.GetFn[bool](true, nil))
	executor.GetWithExecution(testutil.GetFn[bool](false, nil))
	assert.Equal(t, 2, delays)
}

func TestPanicInCircuitBreakerDelayFunction(t *testing.T) {
	breaker := circuitbreaker.Builder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		panic("test")
	}).Build()

	assert.Panics(t, func() {
		failsafe.RunWithExecution(testutil.RunFn(errors.New("test")), breaker)
	})
}

func TestShouldDelayCircuitBreaker(t *testing.T) {
	delays := 0
	breaker := circuitbreaker.Builder[int]().
		HandleIf(func(i int, _ error) bool {
			return i > 0
		}).
		WithDelayFunc(func(exec failsafe.ExecutionAttempt[int]) time.Duration {
			delays++ // side-effect for test purposes
			return 1
		}).
		Build()

	executor := failsafe.NewExecutor[int](breaker)
	executor.GetWithExecution(testutil.GetFn[int](0, nil))
	executor.GetWithExecution(testutil.GetFn[int](1, nil))
	assert.Equal(t, 1, delays)
}
