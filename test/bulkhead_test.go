package test

import (
	"context"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestBulkheadPermitAcquiredAfterWait(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(time.Second).Build()
	setup := func() context.Context {
		bh.TryAcquirePermit()
		bh.TryAcquirePermit() // bulkhead should be full
		go func() {
			time.Sleep(200 * time.Millisecond)
			bh.ReleasePermit() // bulkhead should not be full
		}()
		return nil
	}

	// When / Then
	testutil.TestGetSuccess(t,
		setup, failsafe.NewExecutor[any](bh),
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
	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](bh), func(execution failsafe.Execution[any]) error {
		return nil
	}, 1, 0, bulkhead.ErrFull)
}

// Asserts that an exceeded maxWaitTime causes ErrFull.
func TestBulkheadMaxWaitTimeExceeded(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(20 * time.Millisecond).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.TestRunFailure(t, nil, failsafe.NewExecutor[any](bh), func(execution failsafe.Execution[any]) error {
		return nil
	}, 1, 0, bulkhead.ErrFull)
}
