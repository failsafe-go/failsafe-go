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
	clock := testutil.NewTestClock(0)
	breaker := NewBuilder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 100 * time.Millisecond
	}).Build().(*circuitBreaker[any])
	breaker.clock = clock
	breaker.open(testutil.TestExecution[any]{})
	assert.True(t, breaker.IsOpen())
	assert.False(t, breaker.TryAcquirePermit())

	// When
	clock.SetTime(110)

	// Then
	assert.True(t, breaker.TryAcquirePermit())
	assert.True(t, breaker.IsHalfOpen())
}

func TestRemainingDelay(t *testing.T) {
	clock := testutil.NewTestClock(0)
	breaker := NewBuilder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 1 * time.Second
	}).Build().(*circuitBreaker[any])
	breaker.clock = clock
	breaker.open(testutil.TestExecution[any]{})

	// When / Then
	remainingDelay := breaker.RemainingDelay()
	assert.True(t, remainingDelay > 0)
	assert.True(t, remainingDelay.Milliseconds() < 1001)

	clock.SetTime(110)
	remainingDelay = breaker.RemainingDelay()
	assert.True(t, remainingDelay > 0)
	assert.True(t, remainingDelay.Milliseconds() < 900)
}

// Asserts that a configured jitter keeps the OpenState delay within [delay-jitter, delay+jitter]. The delay just after
// opening equals RemainingDelay since no time has elapsed on the test clock.
func TestJitteredDelay(t *testing.T) {
	clock := testutil.NewTestClock(0)
	for i := 0; i < 100; i++ {
		breaker := NewBuilder[any]().
			WithDelay(100 * time.Millisecond).
			WithJitter(20 * time.Millisecond).
			Build().(*circuitBreaker[any])
		breaker.clock = clock
		breaker.open(testutil.TestExecution[any]{})

		delay := breaker.RemainingDelay()
		assert.GreaterOrEqual(t, delay, 80*time.Millisecond)
		assert.LessOrEqual(t, delay, 120*time.Millisecond)
	}
}

// Asserts that a configured jitter factor keeps the OpenState delay within [delay*(1-factor), delay*(1+factor)].
func TestJitterFactorDelay(t *testing.T) {
	clock := testutil.NewTestClock(0)
	for i := 0; i < 100; i++ {
		breaker := NewBuilder[any]().
			WithDelay(100 * time.Millisecond).
			WithJitterFactor(.25).
			Build().(*circuitBreaker[any])
		breaker.clock = clock
		breaker.open(testutil.TestExecution[any]{})

		delay := breaker.RemainingDelay()
		assert.GreaterOrEqual(t, delay, 75*time.Millisecond)
		assert.LessOrEqual(t, delay, 125*time.Millisecond)
	}
}

// Asserts that a configured random delay keeps the OpenState delay within [delayMin, delayMax].
func TestRandomDelay(t *testing.T) {
	clock := testutil.NewTestClock(0)
	for i := 0; i < 100; i++ {
		breaker := NewBuilder[any]().
			WithRandomDelay(50*time.Millisecond, 150*time.Millisecond).
			Build().(*circuitBreaker[any])
		breaker.clock = clock
		breaker.open(testutil.TestExecution[any]{})

		delay := breaker.RemainingDelay()
		assert.GreaterOrEqual(t, delay, 50*time.Millisecond)
		assert.LessOrEqual(t, delay, 150*time.Millisecond)
	}
}

func TestNoRemainingDelay(t *testing.T) {
	clock := testutil.NewTestClock(0)
	breaker := NewBuilder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 10 * time.Millisecond
	}).Build().(*circuitBreaker[any])
	breaker.clock = clock
	assert.Equal(t, time.Duration(0), breaker.RemainingDelay())

	// When
	breaker.open(testutil.TestExecution[any]{})
	assert.True(t, breaker.RemainingDelay() > 0)
	clock.SetTime(50)

	// Then
	assert.Equal(t, time.Duration(0), breaker.RemainingDelay())
}
