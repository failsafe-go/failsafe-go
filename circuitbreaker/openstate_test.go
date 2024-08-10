package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

var _ circuitState[any] = &openState[any]{}

func TestTryAcquirePermit(t *testing.T) {
	breaker := NewBuilder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 100 * time.Millisecond
	}).Build().(*circuitBreaker[any])
	breaker.open(testutil.TestExecution[any]{})
	assert.True(t, breaker.IsOpen())
	assert.False(t, breaker.TryAcquirePermit())

	// When
	time.Sleep(110 * time.Millisecond)

	// Then
	assert.True(t, breaker.TryAcquirePermit())
	assert.True(t, breaker.IsHalfOpen())
}

func TestRemainingDelay(t *testing.T) {
	breaker := NewBuilder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 1 * time.Second
	}).Build().(*circuitBreaker[any])
	breaker.open(testutil.TestExecution[any]{})

	// When / Then
	remainingDelay := breaker.RemainingDelay()
	assert.True(t, remainingDelay > 0)
	assert.True(t, remainingDelay.Milliseconds() < 1001)

	time.Sleep(110 * time.Millisecond)
	remainingDelay = breaker.RemainingDelay()
	assert.True(t, remainingDelay > 0)
	assert.True(t, remainingDelay.Milliseconds() < 900)
}

func TestNoRemainingDelay(t *testing.T) {
	breaker := NewBuilder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 10 * time.Millisecond
	}).Build().(*circuitBreaker[any])
	assert.Equal(t, time.Duration(0), breaker.RemainingDelay())

	// When
	breaker.open(testutil.TestExecution[any]{})
	assert.True(t, breaker.RemainingDelay() > 0)
	time.Sleep(50 * time.Millisecond)

	// Then
	assert.Equal(t, time.Duration(0), breaker.RemainingDelay())
}
