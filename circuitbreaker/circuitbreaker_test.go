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

func BenchmarkTimedCircuitBreaker(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Builder[any]().
			WithDelay(time.Minute).
			WithFailureThresholdPeriod(10, time.Minute).
			Build()
	}
}
