package test

import (
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestBulkheadPermitAcquiredAfterWait(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(time.Second).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full
	go func() {
		time.Sleep(200 * time.Millisecond)
		bh.ReleasePermit() // bulkhead should not be full
	}()

	// When / Then
	testutil.TestGetSuccess(t, failsafe.NewExecutor[any](bh),
		func(execution failsafe.Execution[any]) (any, error) {
			return "test", nil
		}, 1, 1, "test")
}

func TestBulkheadFull(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.TestRunFailure(t, failsafe.NewExecutor[any](bh), func(execution failsafe.Execution[any]) error {
		return nil
	}, 1, 0, bulkhead.ErrBulkheadFull)
}

// Asserts that an exceeded maxWaitTime causes ErrBulkheadFull.
func TestBulkheadMaxWaitTimeExceeded(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(20 * time.Millisecond).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.TestRunFailure(t, failsafe.NewExecutor[any](bh), func(execution failsafe.Execution[any]) error {
		return nil
	}, 1, 0, bulkhead.ErrBulkheadFull)
}
