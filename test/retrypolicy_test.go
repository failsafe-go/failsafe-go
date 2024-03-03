package test

import (
	"context"
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
	rp := retrypolicy.WithDefaults[bool]()

	// When / Then
	testutil.TestGetFailure(t, nil, failsafe.NewExecutor[bool](rp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrConnecting
		},
		3, 3, testutil.ErrConnecting)
}

func TestShouldReturnRetriesExceededError(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.Builder[bool](), stats).Build()

	// When / Then
	testutil.TestGetFailure(t, policytesting.SetupFn(stats), failsafe.NewExecutor[bool](rp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			return false, testutil.ErrConnecting
		},
		3, 3, &retrypolicy.ExceededError{}, func() {
			assert.Equal(t, 2, stats.Retries())
			assert.Equal(t, 1, stats.RetriesExceeded())
		})
}

func TestShouldReturnExceededErrorWrappingResults(t *testing.T) {
	// Given
	rp1 := retrypolicy.WithDefaults[any]()
	underlyingErr := errors.New("test")

	// When / Then for last error
	err := failsafe.Run(func() error {
		return underlyingErr
	}, rp1)
	var reErr *retrypolicy.ExceededError
	assert.True(t, errors.As(err, &reErr))
	assert.Equal(t, underlyingErr, reErr.LastError())
	assert.Nil(t, reErr.LastResult())

	// Given
	rp2 := retrypolicy.Builder[bool]().HandleResult(false).Build()

	// When / Then for last result
	_, err = failsafe.Get[bool](func() (bool, error) {
		return false, nil
	}, rp2)
	assert.True(t, errors.As(err, &reErr))
	assert.Nil(t, reErr.LastError())
	assert.Equal(t, false, reErr.LastResult())
}

// Tests a simple execution that does not retry.
func TestShouldNotRetryOnSuccess(t *testing.T) {
	// Given
	rp := retrypolicy.WithDefaults[bool]()

	// When / Then
	testutil.TestGetSuccess(t, nil, failsafe.NewExecutor[bool](rp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			return false, nil
		},
		1, 1, false)
}

// Asserts that a non-handled error does not trigger retries.
func TestShouldNotRetryOnNonRetriableFailure(t *testing.T) {
	// Given
	rp := retrypolicy.Builder[int]().
		WithMaxRetries(-1).
		HandleResult(500).
		Build()

	// When / Then
	testutil.TestGetSuccess(t, nil, failsafe.NewExecutor[int](rp),
		func(exec failsafe.Execution[int]) (int, error) {
			if exec.Attempts() <= 2 {
				return 500, nil
			}
			return 0, nil
		},
		3, 3, 0)
}

// Asserts that an execution is failed when the max duration is exceeded.
func TestShouldFailWhenMaxDurationExceeded(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	rp := policytesting.WithRetryStats(retrypolicy.Builder[bool]().
		HandleResult(false).
		WithMaxDuration(100*time.Millisecond), stats).
		Build()

	// When / Then
	testutil.TestGetFailure(t, nil, failsafe.NewExecutor[bool](rp),
		func(exec failsafe.Execution[bool]) (bool, error) {
			if exec.Attempts() == 2 {
				time.Sleep(120 * time.Millisecond)
			}
			return false, errors.New("test")
		},
		2, 2, &retrypolicy.ExceededError{})
}

// Asserts that the last failure is returned
func TestShouldReturnLastFailure(t *testing.T) {
	// Given
	rp := retrypolicy.Builder[any]().
		WithMaxRetries(3).
		ReturnLastFailure().
		Build()
	err := errors.New("test")

	// When / Then
	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](rp),
		func(exec failsafe.Execution[any]) error {
			return err
		},
		4, 4, err)
}

// Asserts that a RetryPolicy configured with unlimited attempts, behaves as expected.
func TestUnlimitedAttempts(t *testing.T) {
	// Given
	rp := retrypolicy.Builder[bool]().WithMaxAttempts(-1).Build()
	stub, reset := testutil.ErrorNTimesThenReturn(testutil.ErrInvalidState, 5, true)
	setup := func() context.Context {
		reset()
		return nil
	}

	// When / Then
	testutil.TestGetSuccess[bool](t, setup, failsafe.NewExecutor[bool](rp),
		stub,
		6, 6, true)
}
