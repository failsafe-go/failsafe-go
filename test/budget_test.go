package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go/budget"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestBudget(t *testing.T) {
	// This test sets 1 execution and 1 retry inflight, then causes an execution to fail, which triggers a successful retry.
	t.Run("when retries not exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		rp := retrypolicy.NewBuilder[bool]().AbortOnErrors(budget.ErrExceeded).Build()
		bb := budget.NewBuilder[bool]().
			ForRetries().
			WithMaxRate(.5).
			WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build()
		testutil.GetBudgetExecutions(b).Add(1)
		assert.True(t, b.TryAcquireRetryPermit()) // budget is almost full

		// When / Then
		stub, reset := testutil.ErrorNTimesThenReturn(testutil.ErrInvalidState, 1, true)
		testutil.Test[bool](t).
			With(rp, b).
			Before(reset).
			Get(stub).
			AssertSuccess(2, 2, true)
	})

	// This test sets 1 execution and 2 retries inflight, then causes another retry to be attempted which is rejected.
	t.Run("when retries exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		rp := retrypolicy.NewBuilder[any]().AbortOnErrors(budget.ErrExceeded).Build()
		bb := budget.NewBuilder[any]().
			ForRetries().
			WithMaxRate(.5).
			WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build()
		testutil.GetBudgetExecutions(b).Add(1)
		assert.True(t, b.TryAcquireRetryPermit())
		assert.True(t, b.TryAcquireRetryPermit()) // budget should be full

		// When / Then
		testutil.Test[any](t).
			With(rp, b).
			Reset(stats).
			Run(testutil.RunFn(testutil.ErrConnecting)).
			AssertFailure(2, 1, budget.ErrExceeded, func() {
				assert.Equal(t, 1, stats.BudgetExceededs())
			})
	})

	// This test sets 1 execution and 1 hedge inflight, then causes an execution to hang, which triggers a successful hedge.
	t.Run("when hedges not exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[bool](10*time.Millisecond), stats).Build()
		bb := budget.NewBuilder[bool]().
			ForHedges().
			WithMaxRate(.5).
			WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build()
		testutil.GetBudgetExecutions(b).Add(1)
		assert.True(t, b.TryAcquireHedgePermit()) // budget is almost full

		// When / Then
		testutil.Test[bool](t).
			With(hp, b).
			Reset(stats).
			Get(testutil.SlowNTimesThenReturn(t, 1, 100*time.Millisecond, true, false)).
			AssertSuccess(2, -1, true, func() {
				assert.Equal(t, 1, stats.Hedges())
			})
	})

	// This test sets 1 execution and 2 hedges inflight, then causes another hedge to be attempted, which is rejected.
	t.Run("when hedges exceeded", func(t *testing.T) {
		// Given
		stats := &policytesting.Stats{}
		hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[bool](10*time.Millisecond), stats).Build()
		bb := budget.NewBuilder[bool]().
			ForHedges().
			WithMaxRate(.5).
			WithMinConcurrency(1)
		b := policytesting.WithBudgetStatsAndLogs(bb, stats, true).Build()
		testutil.GetBudgetExecutions(b).Add(1)
		assert.True(t, b.TryAcquireRetryPermit())
		assert.True(t, b.TryAcquireRetryPermit()) // budget should be full

		// When / Then
		testutil.Test[bool](t).
			With(hp, b).
			Reset(stats).
			Get(testutil.SlowNTimesThenReturn(t, 1, 100*time.Millisecond, true, false)).
			AssertFailure(2, 0, budget.ErrExceeded, func() {
				assert.Equal(t, 1, stats.BudgetExceededs())
			})
	})
}
