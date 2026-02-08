package issues

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

// See https://github.com/failsafe-go/failsafe-go/issues/134
func TestIssue134(t *testing.T) {
	// Given
	to := timeout.New[any](time.Second)
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	var childCtx context.Context

	// When
	result, err := failsafe.With(to).
		WithContext(parentCtx).
		GetWithExecution(func(exec failsafe.Execution[any]) (any, error) {
			childCtx = exec.Context()
			return "success", nil
		})

	// Then
	assert.Nil(t, err)
	assert.Equal(t, "success", result)
	assert.NotNil(t, childCtx.Err(), "child context should be canceled after successful completion")
}

// TestIssue134Hedge tests that hedge policy properly cancels child contexts after completion.
func TestIssue134Hedge(t *testing.T) {
	// Given
	hp := hedgepolicy.NewBuilderWithDelay[any](10 * time.Millisecond).Build()
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	var childCtx context.Context

	// When
	result, err := failsafe.With(hp).
		WithContext(parentCtx).
		GetWithExecution(func(exec failsafe.Execution[any]) (any, error) {
			childCtx = exec.Context()
			return "success", nil
		})

	// Then
	assert.Nil(t, err)
	assert.Equal(t, "success", result)
	assert.NotNil(t, childCtx.Err(), "child context should be canceled after successful completion")
}
