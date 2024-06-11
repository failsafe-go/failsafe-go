package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
)

func TestBulkheadPermitAcquiredAfterWait(t *testing.T) {
	// Given
	bh := bulkhead.Builder[string](2).WithMaxWaitTime(time.Second).Build()
	setup := func() {
		bh.TryAcquirePermit()
		bh.TryAcquirePermit() // bulkhead should be full
		go func() {
			time.Sleep(200 * time.Millisecond)
			bh.ReleasePermit() // bulkhead should not be full
		}()
	}

	// When / Then
	testutil.Test[string](t).
		With(bh).
		Setup(setup).
		Get(testutil.GetFn("test", nil)).
		AssertSuccess(1, 1, "test")
}

func TestBulkheadNotFull(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	bh := policytesting.WithBulkheadStatsAndLogs(bulkhead.Builder[any](2), stats, true).Build()

	// When / Then
	testutil.Test[any](t).
		With(bh).
		Reset(stats).
		Get(testutil.GetFn[any]("test", nil)).
		AssertSuccess(1, 1, "test")
}

func TestBulkheadFull(t *testing.T) {
	// Given
	stats := &policytesting.Stats{}
	bh := policytesting.WithBulkheadStatsAndLogs(bulkhead.Builder[any](2), stats, true).Build()
	assert.True(t, bh.TryAcquirePermit())
	assert.True(t, bh.TryAcquirePermit()) // bulkhead should be full

	// When / Then
	testutil.Test[any](t).
		With(bh).
		Reset(stats).
		Run(testutil.RunFn(nil)).
		AssertFailure(1, 0, bulkhead.ErrFull, func() {
			assert.Equal(t, 1, stats.Fulls())
		})
}

// Asserts that an exceeded maxWaitTime causes ErrFull.
func TestBulkheadMaxWaitTimeExceeded(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](2).WithMaxWaitTime(20 * time.Millisecond).Build()
	bh.TryAcquirePermit()
	bh.TryAcquirePermit() // bulkhead should be full

	// When / Then
	testutil.Test[any](t).
		With(bh).
		Run(testutil.RunFn(nil)).
		AssertFailure(1, 0, bulkhead.ErrFull)
}

// Asserts that a short maxWaitTime still allows a permit to be claimed.
func TestBulkheadWithShortMaxWaitTime(t *testing.T) {
	// Given
	bh := bulkhead.Builder[any](1).WithMaxWaitTime(1 * time.Nanosecond).Build()

	// When / Then
	testutil.Test[any](t).
		With(bh).
		Run(testutil.RunFn(nil)).
		AssertSuccess(1, 1, nil)
}
