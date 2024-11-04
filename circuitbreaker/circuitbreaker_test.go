package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var _ CircuitBreaker[any] = &circuitBreaker[any]{}

func TestShouldDefaultDelay(t *testing.T) {
	breaker := WithDefaults[any]()
	breaker.RecordFailure()
	assert.True(t, breaker.IsOpen())
}

func TestGetSuccessAndFailureStats(t *testing.T) {
	// Given
	breaker := Builder[any]().
		WithFailureThresholdRatio(5, 10).
		WithSuccessThresholdRatio(15, 20).
		Build()

	// When
	for i := 0; i < 7; i++ {
		if i%2 == 0 {
			breaker.RecordSuccess()
		} else {
			breaker.RecordFailure()
		}
	}

	// Then
	assert.Equal(t, uint(3), breaker.Metrics().Failures())
	assert.Equal(t, uint(43), breaker.Metrics().FailureRate())
	assert.Equal(t, uint(4), breaker.Metrics().Successes())
	assert.Equal(t, uint(57), breaker.Metrics().SuccessRate())

	// When
	for i := 0; i < 15; i++ {
		if i%4 == 0 {
			breaker.RecordFailure()
		} else {
			breaker.RecordSuccess()
		}
	}

	// Then
	assert.Equal(t, uint(2), breaker.Metrics().Failures())
	assert.Equal(t, uint(20), breaker.Metrics().FailureRate())
	assert.Equal(t, uint(8), breaker.Metrics().Successes())
	assert.Equal(t, uint(80), breaker.Metrics().SuccessRate())

	// When
	breaker.HalfOpen()
	for i := 0; i < 15; i++ {
		if i%3 == 0 {
			breaker.RecordFailure()
		} else {
			breaker.RecordSuccess()
		}
	}

	// Then
	assert.Equal(t, uint(5), breaker.Metrics().Failures())
	assert.Equal(t, uint(33), breaker.Metrics().FailureRate())
	assert.Equal(t, uint(10), breaker.Metrics().Successes())
	assert.Equal(t, uint(67), breaker.Metrics().SuccessRate())
}

func TestIgnoringExecutions(t *testing.T) {
	// Given
	breaker := Builder[any]().
		WithExecutionIgnorePeriod(10*time.Millisecond).
		WithFailureThresholdRatio(5, 10).
		Build()

	// execution ignore period starts when first execution is recorded, not when circuit breaker is created
	// executions within the ignore period are not recorded

	// When
	time.Sleep(15 * time.Millisecond)
	breaker.RecordSuccess()
	breaker.RecordFailure()

	// Then
	assert.Equal(t, uint(0), breaker.Metrics().Failures())
	assert.Equal(t, uint(0), breaker.Metrics().FailureRate())
	assert.Equal(t, uint(0), breaker.Metrics().Successes())
	assert.Equal(t, uint(0), breaker.Metrics().SuccessRate())

	// once execution ignore period expires, all executions are recorded

	// When
	time.Sleep(10 * time.Millisecond) // execution ignore period expires
	breaker.RecordSuccess()
	breaker.RecordFailure()

	// Then
	assert.Equal(t, uint(1), breaker.Metrics().Failures())
	assert.Equal(t, uint(50), breaker.Metrics().FailureRate())
	assert.Equal(t, uint(1), breaker.Metrics().Successes())
	assert.Equal(t, uint(50), breaker.Metrics().SuccessRate())

	// re-closing the breaker doesn't reset the execution ignore period

	// When
	breaker.Open()  // open the breaker
	breaker.Close() // and close it again
	breaker.RecordFailure()
	breaker.RecordFailure()
	breaker.RecordSuccess()
	breaker.RecordSuccess()
	breaker.RecordSuccess()

	// Then
	assert.Equal(t, uint(2), breaker.Metrics().Failures())
	assert.Equal(t, uint(40), breaker.Metrics().FailureRate())
	assert.Equal(t, uint(3), breaker.Metrics().Successes())
	assert.Equal(t, uint(60), breaker.Metrics().SuccessRate())

	// resetting the breaker resets the execution ignore period (which again starts when first execution is recorded)

	// When
	breaker.Reset()
	time.Sleep(15 * time.Millisecond)
	breaker.RecordSuccess()
	breaker.RecordFailure()

	// Then
	assert.Equal(t, uint(0), breaker.Metrics().Failures())
	assert.Equal(t, uint(0), breaker.Metrics().FailureRate())
	assert.Equal(t, uint(0), breaker.Metrics().Successes())
	assert.Equal(t, uint(0), breaker.Metrics().SuccessRate())
}

func BenchmarkTimedCircuitBreaker(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Builder[any]().
			WithDelay(time.Minute).
			WithFailureThresholdPeriod(10, time.Minute).
			Build()
	}
}
