package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/budget"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestBudget(t *testing.T) {
	// Tests acquire/release using internal.Budget
	t.Run("should acquire and release permit", func(t *testing.T) {
		b := budget.NewBuilder().
			WithMaxRate(.5).
			WithMinConcurrency(1).
			Build().(internal.Budget)
		assert.Equal(t, 0.0, b.Rate())

		testutil.GetBudgetExecutions(b).Add(2)
		testutil.GetInflight(b).Add(1)

		assert.True(t, b.TryAcquirePermit())
		assert.False(t, b.TryAcquirePermit())

		b.ReleasePermit()
		assert.True(t, b.TryAcquirePermit())
		assert.False(t, b.TryAcquirePermit())
	})

	// This test injects a state where rate > maxRate but inflight < minConcurrency, so the floor allows a permit.
	t.Run("when minConcurrency floor allows permit despite rate exceeded", func(t *testing.T) {
		b := budget.NewBuilder().
			WithMaxRate(.5).
			WithMinConcurrency(2).
			Build().(internal.Budget)
		testutil.GetBudgetExecutions(b).Add(1)
		testutil.GetInflight(b).Add(1)

		assert.True(t, b.TryAcquirePermit())
		assert.False(t, b.TryAcquirePermit())
	})

	// This test simulates 1 other primary + 1 other retry inflight, then runs an execution that
	// fails and triggers a successful retry.
	t.Run("when retries not exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		bb := budget.NewBuilder().WithMaxRate(.5).WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build().(internal.Budget)
		rp := retrypolicy.NewBuilder[bool]().WithBudget(b).Build()
		b.RecordExecution()
		b.TryAcquirePermit()

		// When / Then
		stub, reset := testutil.ErrorNTimesThenReturn(testutil.ErrInvalidState, 1, true)
		testutil.Test[bool](t).
			With(rp).
			Before(reset).
			Get(stub).
			AssertSuccess(2, 2, true)
	})

	// This test injects 2 budgeted slots with no associated primaries to simulate a burst of
	// concurrent retries from other requests. The executor's own primary permit then tips the
	// combined rate over the limit, causing the retry to be rejected.
	t.Run("when retries exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		bb := budget.NewBuilder().WithMaxRate(.5).WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build().(internal.Budget)
		rp := retrypolicy.NewBuilder[any]().WithBudget(b).Build()
		testutil.GetBudgetExecutions(b).Add(2)
		testutil.GetInflight(b).Add(2)

		// When / Then
		testutil.Test[any](t).
			With(rp).
			Reset(stats).
			Run(testutil.RunFn(testutil.ErrConnecting)).
			AssertFailure(2, 1, budget.ErrExceeded, func() {
				assert.Equal(t, 1, stats.BudgetExceededs())
			})
	})

	// This test simulates 1 other primary + 1 other hedge inflight, then runs an execution that
	// hangs and triggers a successful hedge.
	t.Run("when hedges not exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		bb := budget.NewBuilder().WithMaxRate(.5).WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build().(internal.Budget)
		hpb := hedgepolicy.NewBuilderWithDelay[bool](10 * time.Millisecond).WithBudget(b)
		hp := policytesting.WithHedgeStatsAndLogs(hpb, stats).Build()
		b.RecordExecution()
		b.TryAcquirePermit()

		// When / Then
		testutil.Test[bool](t).
			With(hp).
			Reset(stats).
			Get(testutil.SlowNTimesThenReturn(t, 1, 100*time.Millisecond, true, false)).
			AssertSuccess(2, -1, true, func() {
				assert.Equal(t, 1, stats.Hedges())
			})
	})

	// This test injects 2 budgeted slots with no associated primaries to simulate a burst of
	// concurrent hedges from other requests. The executor's own primary permit then tips the
	// combined rate over the limit, causing the hedge to be rejected.
	t.Run("when hedges exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		bb := budget.NewBuilder().WithMaxRate(.5).WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build().(internal.Budget)
		hpb := hedgepolicy.NewBuilderWithDelay[bool](10 * time.Millisecond).WithBudget(b)
		hp := policytesting.WithHedgeStatsAndLogs(hpb, stats).Build()
		testutil.GetBudgetExecutions(b).Add(2)
		testutil.GetInflight(b).Add(2)

		// When / Then
		testutil.Test[bool](t).
			With(hp).
			Reset(stats).
			Get(testutil.SlowNTimesThenReturn(t, 1, 100*time.Millisecond, true, false)).
			AssertFailure(2, 0, budget.ErrExceeded, func() {
				assert.Equal(t, 1, stats.BudgetExceededs())
			})
	})
}
