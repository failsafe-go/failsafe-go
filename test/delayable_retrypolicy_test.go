package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestPanicInRetryPolicyDelayFunction(t *testing.T) {
	breaker := retrypolicy.Builder[any]().WithDelayFn(func(exec *failsafe.Execution[any]) time.Duration {
		panic("test")
	}).Build()

	assert.Panics(t, func() {
		failsafe.With[any](breaker).Run(testutil.RunFn(errors.New("test")))
	})
}

func TestShouldDelayRetryPolicy(t *testing.T) {
	delays := 0
	retryPolicy := retrypolicy.Builder[bool]().
		HandleResult(false).
		WithDelayFn(func(exec *failsafe.Execution[bool]) time.Duration {
			delays++ // side-effect for test purposes
			return 1
		}).
		Build()

	executor := failsafe.With[bool](retryPolicy)
	executor.Get(testutil.GetFn[bool](true, nil))
	executor.Get(testutil.GetFn[bool](false, nil))
	assert.Equal(t, 2, delays)
}
