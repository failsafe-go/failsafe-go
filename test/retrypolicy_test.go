package test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// Tests a simple execution that retries.
func TestShouldRetryOnFailure(t *testing.T) {
	// Given
	rp := retrypolicy.NewWithDefaults[bool]()

	// When / Then
	testutil.Test[bool](t).
		With(rp).
		Get(testutil.GetFn(false, testutil.ErrConnecting)).
		AssertFailure(3, 3, testutil.ErrConnecting)
}

func TestShouldReturnRetriesExceededError(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.NewBuilder[bool](), stats).Build()

	// When / Then
	testutil.Test[bool](t).
		With(rp).
		Reset(stats).
		Get(testutil.GetFn(false, testutil.ErrConnecting)).
		AssertFailureAs(3, 3, &retrypolicy.ExceededError{}, func() {
			assert.Equal(t, 2, stats.Retries())
			assert.Equal(t, 1, stats.RetriesExceeded())
		})
}

func TestShouldReturnExceededErrorWrappingResults(t *testing.T) {
	// Given
	rp1 := retrypolicy.NewWithDefaults[any]()
	underlyingErr := errors.New("test")

	// When / Then for last error
	err := failsafe.Run(func() error {
		return underlyingErr
	}, rp1)
	var reErr retrypolicy.ExceededError
	assert.True(t, errors.As(err, &reErr))
	assert.Equal(t, underlyingErr, reErr.LastError)
	assert.Nil(t, reErr.LastResult)

	// Given
	rp2 := retrypolicy.NewBuilder[bool]().HandleResult(false).Build()

	// When / Then for last result
	_, err = failsafe.Get[bool](func() (bool, error) {
		return false, nil
	}, rp2)
	assert.True(t, errors.As(err, &reErr))
	assert.Nil(t, reErr.LastError)
	assert.Equal(t, false, reErr.LastResult)
}

// Tests a simple execution that does not retry.
func TestShouldNotRetryOnSuccess(t *testing.T) {
	// Given
	rp := retrypolicy.NewWithDefaults[bool]()

	// When / Then
	testutil.Test[bool](t).
		With(rp).
		Get(testutil.GetFn(false, nil)).
		AssertSuccess(1, 1, false)
}

// Asserts that a non-handled error does not trigger retries.
func TestShouldNotRetryOnNonRetriableFailure(t *testing.T) {
	// Given
	rp := retrypolicy.NewBuilder[int]().
		WithMaxRetries(-1).
		HandleResult(500).
		Build()

	// When / Then
	testutil.Test[int](t).
		With(rp).
		Get(func(exec failsafe.Execution[int]) (int, error) {
			if exec.Attempts() <= 2 {
				return 500, nil
			}
			return 0, nil
		}).
		AssertSuccess(3, 3, 0)
}

// Asserts that an execution is failed when the max duration is exceeded.
func TestShouldFailWhenMaxDurationExceeded(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.NewBuilder[bool]().
		HandleResult(false).
		WithMaxDuration(100*time.Millisecond), stats).
		Build()

	// When / Then
	testutil.Test[bool](t).
		With(rp).
		Get(func(exec failsafe.Execution[bool]) (bool, error) {
			if exec.Attempts() == 2 {
				time.Sleep(120 * time.Millisecond)
			}
			return false, errors.New("test")
		}).
		AssertFailureAs(2, 2, &retrypolicy.ExceededError{})
}

// Asserts that the last failure is returned
func TestShouldReturnLastFailure(t *testing.T) {
	// Given
	rp := retrypolicy.NewBuilder[any]().
		WithMaxRetries(3).
		ReturnLastFailure().
		Build()
	err := errors.New("test")

	// When / Then
	testutil.Test[any](t).
		With(rp).
		Run(testutil.RunFn(err)).
		AssertFailure(4, 4, err)
}

// Asserts that a RetryPolicy configured with unlimited attempts, behaves as expected.
func TestUnlimitedAttempts(t *testing.T) {
	// Given
	rp := retrypolicy.NewBuilder[bool]().WithMaxAttempts(-1).Build()
	stub, reset := testutil.ErrorNTimesThenReturn(testutil.ErrInvalidState, 5, true)

	// When / Then
	testutil.Test[bool](t).
		With(rp).
		Setup(reset).
		Get(stub).
		AssertSuccess(6, 6, true)
}

// Asserts that backoff delays are as expected.
func TestBackoffDelay(t *testing.T) {
	var delays []time.Duration
	rp := retrypolicy.NewBuilder[any]().
		WithBackoff(time.Millisecond, 10*time.Millisecond).
		WithMaxRetries(6).
		OnRetryScheduled(func(e failsafe.ExecutionScheduledEvent[any]) {
			delays = append(delays, e.Delay)
		}).Build()

	failsafe.Run(func() error {
		return testutil.ErrInvalidState
	}, rp)

	expected := []time.Duration{time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond, 8 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	assert.ElementsMatch(t, expected, delays)
}
