package issues

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/budget"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
)

// See https://github.com/failsafe-go/failsafe-go/issues/141
// Asserts that when a hedge budget is exceeded, only the hedge that exceeds the budget is suppressed, and attempts that
// are already inflight, including the initial attempt, are still allowed to produce a result.
func TestIssue141(t *testing.T) {
	// Given
	hedgeBudget := budget.NewBuilder().WithMinConcurrency(1).Build()
	hp := hedgepolicy.NewBuilderWithDelay[string](20 * time.Millisecond).
		WithMaxHedges(2).
		WithBudget(hedgeBudget).
		Build()

	// When
	result, err := failsafe.With(hp).GetWithExecution(func(exec failsafe.Execution[string]) (string, error) {
		time.Sleep(200 * time.Millisecond)
		return "success", nil
	})

	// Then
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}
