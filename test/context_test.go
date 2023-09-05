package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// Asserts context timeouts are handled as expected.
func TestContextTimeout(t *testing.T) {
	// Given
	fb := fallback.WithResult[any](true)
	rp := retrypolicy.WithDefaults[any]()
	cb := circuitbreaker.WithDefaults[any]()
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Millisecond)

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

	// When
	err := failsafe.With[any](fb, rp, cb).WithContext(ctx).RunWithExecution(func(exec failsafe.Execution[any]) error {
		if err := util.WaitWithContext(exec.Context, time.Second); err != nil {
			fmt.Println(exec.Context)
			done = exec.IsCanceled()
			return err
		}
		return testutil.InvalidArgumentError{}
	})

	// Then
	assert.True(t, done)
	assert.Error(t, context.Canceled, err)
}
