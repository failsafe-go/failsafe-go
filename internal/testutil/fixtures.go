package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/failsafe-go/failsafe-go"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrInvalidState    = errors.New("invalid state")
	ErrConnecting      = errors.New("connection error")

	NoopFn = func() error { return nil }

	GetFalseFn = func() (bool, error) { return false, nil }

	GetTrueFn = func() (bool, error) { return true, nil }
)

type CustomError struct{ Msg string }

func (e CustomError) Error() string { return e.Msg }

type CompositeError struct {
	Cause error
}

func (e CompositeError) Error() string { return "CompositeError" }

func (e CompositeError) Unwrap() error { return e.Cause }

type MultiError []error

func (e MultiError) Error() string { return "MultiError" }

func (e MultiError) Unwrap() []error { return e }

func RunFn(err error) func(failsafe.Execution[any]) error {
	return func(exec failsafe.Execution[any]) error {
		return err
	}
}

func GetFn[R any](result R, err error) func(failsafe.Execution[R]) (R, error) {
	return func(exec failsafe.Execution[R]) (R, error) {
		return result, err
	}
}

// ErrorNTimesThenReturn returns a stub function that returns the err errorTimes and then returns the results.
// Can be used with failsafe.GetWithExecution.
func ErrorNTimesThenReturn[R any](err error, errorTimes int, results ...R) (fn func(failsafe.Execution[R]) (R, error), resetFn func()) {
	errorCounter := 0
	resultIndex := 0
	return func(exec failsafe.Execution[R]) (R, error) {
			if errorCounter < errorTimes {
				errorCounter++
				return *new(R), err
			} else if resultIndex < len(results) {
				result := results[resultIndex]
				resultIndex++
				return result, nil
			}
			return *new(R), nil
		}, func() {
			errorCounter = 0
			resultIndex = 0
		}
}

// ErrorNTimesThenPanic returns a stub function that returns the err errorTimes and then panics with the panicValue.
// Can be used with failsafe.GetWithExecution.
func ErrorNTimesThenPanic[R any](err error, errorTimes int, panicValue any) func(failsafe.Execution[R]) (R, error) {
	errorCounter := 0
	return func(exec failsafe.Execution[R]) (R, error) {
		if errorCounter < errorTimes {
			errorCounter++
			return *new(R), err
		}
		panic(panicValue)
	}
}

// ErrorNTimesThenError returns a stub function that returns the err errorTimes and then returns the finalError.
// Can be used with failsafe.GetWithExecution.
func ErrorNTimesThenError[R any](err error, errorTimes int, finalError error) func(failsafe.Execution[R]) (R, error) {
	errorCounter := 0
	return func(exec failsafe.Execution[R]) (R, error) {
		if errorCounter < errorTimes {
			errorCounter++
			return *new(R), err
		}
		return *new(R), finalError
	}
}

func SlowNTimesThenReturn[R any](t *testing.T, slowTimes int, sleepTime time.Duration, delayedResult R, fastResult R) func(failsafe.Execution[R]) (R, error) {
	return func(exec failsafe.Execution[R]) (R, error) {
		if exec.Attempts() <= slowTimes {
			time.Sleep(sleepTime)
			return delayedResult, nil
		}
		WaitAndAssertCanceled(t, time.Second, exec)
		return fastResult, nil
	}
}

type TestExecution[R any] struct {
	TheLastResult R
	TheAttempts   int
	TheRetries    int
	TheHedges     int
}

func (e TestExecution[R]) Attempts() int {
	return e.TheAttempts
}

func (e TestExecution[R]) Executions() int {
	panic("unimplemented stub")
}

func (e TestExecution[R]) StartTime() time.Time {
	panic("unimplemented stub")
}

func (e TestExecution[R]) Retries() int {
	return e.TheRetries
}

func (e TestExecution[R]) Hedges() int {
	return e.TheHedges
}

func (e TestExecution[R]) IsFirstAttempt() bool {
	panic("unimplemented stub")
}

func (e TestExecution[R]) IsRetry() bool {
	panic("unimplemented stub")
}

func (e TestExecution[R]) ElapsedTime() time.Duration {
	panic("unimplemented stub")
}

func (e TestExecution[R]) IsHedge() bool {
	panic("unimplemented stub")
}

func (e TestExecution[R]) LastResult() R {
	return e.TheLastResult
}

func (e TestExecution[R]) LastError() error {
	panic("unimplemented stub")
}

func (e TestExecution[R]) AttemptStartTime() time.Time {
	panic("unimplemented stub")
}

func (e TestExecution[R]) ElapsedAttemptTime() time.Duration {
	panic("unimplemented stub")
}

func (e TestExecution[R]) Context() context.Context {
	return nil
}

func (e TestExecution[R]) IsCanceled() bool {
	panic("unimplemented stub")
}

func (e TestExecution[R]) Canceled() <-chan struct{} {
	panic("unimplemented stub")
}
