package failsafe_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"failsafe"
	"failsafe/retrypolicy"
)

var testErr = errors.New("test")

func TestRun(t *testing.T) {
	rp := retrypolicy.Builder[any]().
		WithDelay(1 * time.Second).
		Handle(testErr).
		OnRetry(func(e failsafe.ExecutionAttemptedEvent[any]) {
			fmt.Printf("retrying %v, %v\n", e.LastResult, e.LastErr)
		}).
		Build()

	err := failsafe.With[any](rp).RunWithExecution(func(exec failsafe.Execution[any]) error {
		return testErr
	})

	fmt.Printf("er: %v\n", err)
}

// might not be a good idea to assume people will want/need a variant that doesn't return results
func TestGet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()

	rp := retrypolicy.Builder[string]().
		HandleAllIf(func(result string, err error) bool {
			return result == "foo"
		}).
		WithDelay(1 * time.Second).
		WithMaxRetries(5).
		OnRetry(func(e failsafe.ExecutionAttemptedEvent[string]) {
			fmt.Printf("retrying %v, %v\n", e.LastResult, e.LastErr)
		}).
		Build()

	result, err := failsafe.With[string](rp).WithContext(ctx).Get(func() (string, error) {
		return "asdf", errors.New("test")
	})

	fmt.Printf("er: %v, %v\n", result, err)
}
