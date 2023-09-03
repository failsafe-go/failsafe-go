package circuitbreaker

import (
	"testing"

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
	assert.Equal(t, uint(3), breaker.GetFailureCount())
	assert.Equal(t, uint(43), breaker.GetFailureRate())
	assert.Equal(t, uint(4), breaker.GetSuccessCount())
	assert.Equal(t, uint(57), breaker.GetSuccessRate())

	// When
	for i := 0; i < 15; i++ {
		if i%4 == 0 {
			breaker.RecordFailure()
		} else {
			breaker.RecordSuccess()
		}
	}

	// Then
	assert.Equal(t, uint(2), breaker.GetFailureCount())
	assert.Equal(t, uint(20), breaker.GetFailureRate())
	assert.Equal(t, uint(8), breaker.GetSuccessCount())
	assert.Equal(t, uint(80), breaker.GetSuccessRate())

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
	assert.Equal(t, uint(5), breaker.GetFailureCount())
	assert.Equal(t, uint(33), breaker.GetFailureRate())
	assert.Equal(t, uint(10), breaker.GetSuccessCount())
	assert.Equal(t, uint(67), breaker.GetSuccessRate())
}
