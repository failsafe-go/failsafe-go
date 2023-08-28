package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"failsafe"
	"failsafe/fallback"
	"failsafe/internal/testutil"
	"failsafe/retrypolicy"
)

// Fallback -> RetryPolicy
func TestFallbackRetryPolicy(t *testing.T) {
	// Given
	fb := fallback.WithResult[bool](true)
	rp := retrypolicy.OfDefaults[bool]()

	// When / Then
	testutil.TestGetSuccess[bool](t, failsafe.WithResult[bool](fb, rp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidArgumentError{}
		},
		3, 3, true)

	// Given
	fb = fallback.WithGetFn[bool](func(e failsafe.ExecutionAttemptedEvent[bool]) (bool, error) {
		assert.False(t, e.LastResult)
		assert.ErrorIs(t, testutil.InvalidStateError{}, e.LastErr)
		return true, nil
	})

	// When / Then
	testutil.TestGetSuccess[bool](t, failsafe.WithResult[bool](fb, rp),
		func(execution failsafe.Execution[bool]) (bool, error) {
			return false, testutil.InvalidStateError{}
		},
		3, 3, true)
}
