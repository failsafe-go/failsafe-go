package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
)

var _ circuitState[any] = &halfOpenState[any]{}

// Asserts that  the circuit is opened after a single failure.
func TestHalfOpenStateFailureWithDefaultConfig(t *testing.T) {
	// Given
	breaker := NewWithDefaults[any]()
	breaker.HalfOpen()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())

	// When
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after the failure threshold is met.
func TestHalfOpenFailureWithFailureThreshold(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithFailureThreshold(3).Build()
	breaker.HalfOpen()

	// When
	for i := 0; i < 3; i++ {
		assert.False(t, breaker.IsOpen())
		assert.False(t, breaker.IsClosed())
		breaker.RecordFailure()
	}

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after the failure ratio is met.
func TestHalfOpenFailureWithFailureRatio(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithFailureThresholdRatio(2, 3).Build()
	breaker.HalfOpen()
	assert.False(t, breaker.IsOpen())

	// When
	breaker.RecordFailure()
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after a single failure. The failure threshold is ignored.
func TestHalfOpenFailureWithSuccessAndFailureThresholds(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithSuccessThreshold(3).
		WithFailureThreshold(2).
		Build()
	breaker.HalfOpen()

	// When
	breaker.RecordSuccess()
	breaker.RecordSuccess()

	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after the success ratio fails to be met. The failure ratio is ignored.
func TestHalfOpenFailureWithSuccessAndFailureRatios(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithSuccessThresholdRatio(3, 4).
		WithFailureThresholdRatio(3, 5).Build()
	breaker.HalfOpen()

	// When
	breaker.RecordSuccess()
	breaker.RecordFailure()
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after the success ratio fails to be met.
func TestHalfOpenFailureWithSuccessRatio(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithSuccessThresholdRatio(2, 3).Build()
	breaker.HalfOpen()
	assert.False(t, breaker.IsOpen())

	// When
	breaker.RecordFailure()
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after the success ratio fails to be met. The failure threshold is ignored.
func TestHalfOpenFailureWithSuccessRatioAndFailureThreshold(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithSuccessThresholdRatio(2, 4).
		WithFailureThreshold(1).
		Build()
	breaker.HalfOpen()

	// When
	breaker.RecordSuccess()
	breaker.RecordFailure()
	breaker.RecordFailure()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after a single failure.
func TestHalfOpenFailureWithSuccessThreshold(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithSuccessThreshold(3).Build()
	breaker.HalfOpen()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())

	// When
	breaker.RecordSuccess()
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is opened after a single failure.
func TestHalfOpenFailureWithSuccessThresholdAndFailureRatio(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithSuccessThreshold(3).
		WithFailureThresholdRatio(3, 5).
		Build()
	breaker.HalfOpen()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())

	// When
	breaker.RecordFailure()

	// Then
	assert.True(t, breaker.IsOpen())
}

// Asserts that the circuit is closed after a single success.
func TestHalfOpenSuccessWithDefaultConfig(t *testing.T) {
	// Given
	breaker := NewWithDefaults[any]()
	breaker.HalfOpen()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())

	// When
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after the failure ratio fails to be met.
 */
func TestHalfOpenSuccessWithFailureRatio(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithFailureThresholdRatio(2, 3).Build()
	breaker.HalfOpen()

	// When
	breaker.RecordFailure()
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after a single success.
 */
func TestHalfOpenSuccessWithFailureThreshold(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithFailureThreshold(3).Build()
	breaker.HalfOpen()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())

	// When
	breaker.RecordFailure()
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after the success ratio is met. The failure ratio is ignored.
 */
func TestHalfOpenSuccessWithSuccessAndFailureRatios(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithFailureThresholdRatio(3, 5).
		WithSuccessThresholdRatio(3, 4).Build()
	breaker.HalfOpen()

	// When
	breaker.RecordSuccess()
	breaker.RecordFailure()
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after the success threshold is met. The failure threshold is ignored.
 */
func TestHalfOpenSuccessWithSuccessAndFailureThresholds(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithFailureThreshold(2).
		WithSuccessThreshold(3).
		Build()
	breaker.HalfOpen()

	// When
	breaker.RecordSuccess()
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after the success ratio is met.
 */
func TestHalfOpenSuccessWithSuccessRatio(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithSuccessThresholdRatio(2, 3).Build()
	breaker.HalfOpen()

	// When
	breaker.RecordFailure()
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after the success ratio is met. The failure threshold is ignored.
 */
func TestHalfOpenSuccessWithSuccessRatioAndFailureThreshold(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithFailureThreshold(2).
		WithSuccessThresholdRatio(3, 4).
		Build()
	breaker.HalfOpen()

	// When
	breaker.RecordSuccess()
	breaker.RecordSuccess()
	breaker.RecordFailure()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after the success threshold is met.
 */
func TestHalfOpenSuccessWithSuccessThreshold(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().WithSuccessThreshold(3).Build()
	breaker.HalfOpen()

	// When
	for i := 0; i < 3; i++ {
		assert.False(t, breaker.IsOpen())
		assert.False(t, breaker.IsClosed())
		breaker.RecordSuccess()
	}

	// Then
	assert.True(t, breaker.IsClosed())
}

/**
 * Asserts that the circuit is closed after the success threshold is met. The failure ratio is ignored.
 */
func TestHalfOpenSuccessWithSuccessThresholdAndFailureRatio(t *testing.T) {
	// Given
	breaker := NewBuilder[any]().
		WithFailureThresholdRatio(3, 5).
		WithSuccessThreshold(2).
		Build()
	breaker.HalfOpen()

	// When success threshold exceeded
	breaker.RecordSuccess()
	assert.False(t, breaker.IsOpen())
	assert.False(t, breaker.IsClosed())
	breaker.RecordSuccess()

	// Then
	assert.True(t, breaker.IsClosed())
}

func TestRemainingDelayInHalfOpenState(t *testing.T) {
	breaker := NewBuilder[any]().WithDelayFunc(func(exec failsafe.ExecutionAttempt[any]) time.Duration {
		return 1 * time.Second
	}).Build().(*circuitBreaker[any])

	// When / Then
	breaker.halfOpen()
	assert.Equal(t, time.Duration(0), breaker.RemainingDelay())
}
