package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
	rptesting "failsafe/internal/retrypolicy_testutil"
	"failsafe/internal/testutil"
	"failsafe/retrypolicy"
)

// Tests a simple execution that retries.
func TestShouldRetryOnFailure(t *testing.T) {
	// Given
	rp := retrypolicy.OfDefaults[bool]()

	// When / Then
	testutil.TestGetFailure(t, failsafe.With[bool](rp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ConnectionError{}
		},
		3, 3, testutil.ConnectionError{})
}

// Tests a simple execution that does not retry.
func TestShouldNotRetryOnSuccess(t *testing.T) {
	// Given
	rp := retrypolicy.OfDefaults[bool]()

	// When / Then
	testutil.TestGetSuccess(t, failsafe.With[bool](rp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			return false, nil
		},
		1, 1, false)
}

// Asserts that a non-handled error does not trigger retries.
func TestShouldNotRetryOnNonRetriableFailure(t *testing.T) {
	// Given
	rp := retrypolicy.Builder[any]().
		WithMaxRetries(-1).
		HandleErrors(testutil.ConnectionError{}).
		Build()

	// When / Then
	testutil.TestRunFailure(t, failsafe.With[any](rp),
		func(exec failsafe.Execution[any]) error {
			if exec.Attempts <= 2 {
				return testutil.ConnectionError{}
			}
			return testutil.TimeoutError{}
		},
		3, 3, testutil.TimeoutError{})
}

// Asserts that an execution is failed when the max duration is exceeded.
func TestShouldCompleteWhenMaxDurationExceeded(t *testing.T) {
	// Given
	stats := &testutil.Stats{}
	rp := rptesting.WithRetryStats(retrypolicy.Builder[bool]().
		HandleResult(false).
		WithMaxDuration(100*time.Millisecond), stats).
		Build()

	// When / Then
	testutil.TestGetSuccess(t, failsafe.With[bool](rp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			time.Sleep(120 * time.Millisecond)
			return false, nil
		},
		1, 1, false)
}

// Asserts that the ExecutionScheduledEvent.getDelay is as expected.
func TestScheduledRetryDelay(t *testing.T) {
	// Given
	delay := 10 * time.Millisecond
	rp := retrypolicy.Builder[any]().
		WithDelay(delay).
		OnRetryScheduled(func(e failsafe.ExecutionScheduledEvent[any]) {
			assert.Equal(t, delay, e.GetDelay())
		}).
		Build()

	// When / Then
	failsafe.With[any](rp).Run(func() error {
		return testutil.ConnectionError{}
	})
}
