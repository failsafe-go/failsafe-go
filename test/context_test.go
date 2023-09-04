package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"failsafe"
	"failsafe/circuitbreaker"
	"failsafe/fallback"
	"failsafe/internal/testutil"
	"failsafe/internal/util"
	"failsafe/retrypolicy"
)

// Asserts context deadlines are handled as expected.
func TestContextDeadline(t *testing.T) {
	// Given
	fb := fallback.WithResult[any](true)
	rp := retrypolicy.WithDefaults[any]()
	cb := circuitbreaker.WithDefaults[any]()
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(100*time.Millisecond))

	// When
	err := failsafe.With[any](fb, rp, cb).WithContext(ctx).RunWithExecution(func(exec failsafe.Execution[any]) error {
		if err := util.WaitWithContext(exec.Context, time.Second); err != nil {
			return err
		}
		return testutil.InvalidArgumentError{}
	})

	// Then
	assert.Error(t, context.DeadlineExceeded, err)
}

func TestExecutionWithContext(t *testing.T) {
	// Given
	fb := fallback.WithResult[any](true)
	rp := retrypolicy.WithDefaults[any]()
	cb := circuitbreaker.WithDefaults[any]()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	done := false
	canceled := false

	// When
	err := failsafe.With[any](fb, rp, cb).WithContext(ctx).RunWithExecution(func(exec failsafe.Execution[any]) error {
		if err := util.WaitWithContext(exec.Context, time.Second); err != nil {
			fmt.Println(exec.Context)
			done = exec.IsDone()
			canceled = exec.IsCanceled()
			return err
		}
		return testutil.InvalidArgumentError{}
	})

	// Then
	assert.True(t, done)
	assert.True(t, canceled)
	assert.Error(t, context.Canceled, err)
}
