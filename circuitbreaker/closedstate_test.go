package circuitbreaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Asserts that the circuit is opened after a single failure.
func TestClosedStateFailureWithDefaultConfig(t *testing.T) {
	// Given
	breaker := OfDefaults[any]().(*circuitBreaker[any])
	breaker.Close()
	assert.True(t, breaker.IsClosed())

	// When
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after the failure threshold is met.
func TestClosedStateFailureWithFailureThreshold(t *testing.T) {
	// Given
	breaker := Builder[any]().WithFailureThreshold(NewCountBasedThreshold(3, 3)).Build()
	breaker.Close()

	// When
	breaker.RecordFailure()
	breaker.RecordSuccess()
	breaker.RecordFailure()
	breaker.RecordFailure()
	assert.True(t, breaker.IsClosed())
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after the failure ratio is met.
func TestClosedStateFailureWithFailureRatio(t *testing.T) {
	// Given
	breaker := Builder[any]().WithFailureThreshold(NewCountBasedThreshold(2, 3)).Build()
	breaker.Close()

	// When
	breaker.RecordFailure()
	breaker.RecordSuccess()
	assert.True(t, breaker.IsClosed())
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is still closed after a single success.
func TestClosedStateSuccessWithDefaultConfig(t *testing.T) {
	// Given
	breaker := OfDefaults[any]()
	breaker.Close()
	assert.True(t, breaker.IsClosed())

	// When
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

// Asserts that the circuit stays closed after the failure ratio fails to be met.
func TestClosedStateSuccessWithFailureRatio(t *testing.T) {
	// Given
	breaker := Builder[any]().WithFailureThreshold(NewCountBasedThreshold(3, 4)).Build()
	breaker.Close()
	assert.True(t, breaker.IsClosed())

	// When / Then
	for i := 0; i < 20; i++ {
		breaker.RecordSuccess()
		breaker.RecordFailure()
		assert.True(t, breaker.IsClosed())
	}
}

// Asserts that the circuit stays closed after the failure ratio fails to be met.
func TestClosedStateSuccessWithFailureThreshold(t *testing.T) {
	// Given
	breaker := Builder[any]().WithFailureThreshold(NewCountBasedThreshold(2, 2)).Build()
	breaker.Close()
	assert.True(t, breaker.IsClosed())

	// When / Then
	for i := 0; i < 20; i++ {
		breaker.RecordSuccess()
		breaker.RecordFailure()
		assert.True(t, breaker.IsClosed())
	}
}
