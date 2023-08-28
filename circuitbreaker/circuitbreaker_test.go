package circuitbreaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldDefaultDelay(t *testing.T) {
	breaker := OfDefaults[any]()
	breaker.RecordFailure()
	assert.True(t, breaker.IsOpen())
}

func TestGetSuccessAndFailureStats(t *testing.T) {
	// Given
	breaker := Builder().
		WithFailureThreshold(NewCountBasedThreshold(5, 10)).
		WithSuccessThreshold(NewCountBasedThreshold(15, 20)).
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
	assert.Equal(t, 3, breaker.GetFailureCount())
	assert.Equal(t, 43, breaker.GetFailureRate())
	assert.Equal(t, 4, breaker.GetSuccessCount())
	assert.Equal(t, 57, breaker.GetSuccessRate())

	// When
	for i := 0; i < 15; i++ {
		if i%4 == 0 {
			breaker.RecordSuccess()
		} else {
			breaker.RecordFailure()
		}
	}

	// Then
	assert.Equal(t, 2, breaker.GetFailureCount())
	assert.Equal(t, 20, breaker.GetFailureRate())
	assert.Equal(t, 8, breaker.GetSuccessCount())
	assert.Equal(t, 80, breaker.GetSuccessRate())

	// When
	breaker.HalfOpen()
	for i := 0; i < 15; i++ {
		if i%3 == 0 {
			breaker.RecordSuccess()
		} else {
			breaker.RecordFailure()
		}
	}

	// Then
	assert.Equal(t, 5, breaker.GetFailureCount())
	assert.Equal(t, 33, breaker.GetFailureRate())
	assert.Equal(t, 10, breaker.GetSuccessCount())
	assert.Equal(t, 67, breaker.GetSuccessRate())
}
