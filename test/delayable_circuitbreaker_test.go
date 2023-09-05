package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestPanicInCircuitBreakerDelayFunction(t *testing.T) {
	breaker := circuitbreaker.Builder[any]().WithDelayFn(func(exec *failsafe.Execution[any]) time.Duration {
		panic("test")
	}).Build()

	assert.Panics(t, func() {
		failsafe.With[any](breaker).Run(testutil.RunFn(errors.New("test")))
	})
}

func TestShouldDelayCircuitBreaker(t *testing.T) {
	delays := 0
	breaker := circuitbreaker.Builder[int]().
		HandleIf(func(i int, _ error) bool {
			return i > 0
		}).
		WithDelayFn(func(exec *failsafe.Execution[int]) time.Duration {
			delays++ // side-effect for test purposes
			return 1
		}).
		Build()

	executor := failsafe.With[int](breaker)
	executor.Get(testutil.GetFn[int](0, nil))
	executor.Get(testutil.GetFn[int](1, nil))
	assert.Equal(t, 1, delays)
}
