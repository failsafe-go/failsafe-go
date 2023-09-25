package testutil

import (
	"context"
	"errors"
	"time"

	"github.com/failsafe-go/failsafe-go"
)

type CompositeError struct {
	Cause error
}

func (ce *CompositeError) Error() string {
	return "CompositeError"
}

func (ce *CompositeError) Unwrap() error {
	return ce.Cause
}

func (ce *CompositeError) Is(err error) bool {
	e, ok := err.(*CompositeError)
	if !ok {
		return false
	}
	return errors.Is(e, ce.Cause)
}

var ErrInvalidArgument = errors.New("invalid argument")
var ErrInvalidState = errors.New("invalid state")
var ErrConnecting = errors.New("connection error")
var ErrTimeout = errors.New("timeout error")

var NoopFn = func() error {
	return nil
}

var GetFalseFn = func() (bool, error) {
	return false, nil
}

var GetTrueFn = func() (bool, error) {
	return true, nil
}

func RunFn(err error) func() error {
	return func() error {
		return err
	}
}

func GetFn[R any](result R, err error) func() (R, error) {
	return func() (R, error) {
		return result, err
	}
}

func GetWithExecutionFn[R any](result R, err error) func(exec failsafe.Execution[R]) (R, error) {
	return func(exec failsafe.Execution[R]) (R, error) {
		return result, err
	}
}

// ErrorNTimesThenReturn returns a stub function that returns the err errorTimes and then returns the results.
// Can be used with failsafe.GetWithExecution.
func ErrorNTimesThenReturn[R any](err error, errorTimes int, results ...R) (fn func(exec failsafe.Execution[R]) (R, error), resetFn func()) {
	errorCounter := 0
	resultIndex := 0
	return func(exec failsafe.Execution[R]) (R, error) {
			if errorCounter < errorTimes {
				errorCounter++
				return *(new(R)), err
			} else if resultIndex < len(results) {
				result := results[resultIndex]
				resultIndex++
				return result, nil
			}
			return *(new(R)), nil
		}, func() {
			errorCounter = 0
			resultIndex = 0
		}
}

// ErrorNTimesThenPanic returns a stub function that returns the err errorTimes and then panics with the panicValue.
// Can be used with failsafe.GetWithExecution.
func ErrorNTimesThenPanic[R any](err error, errorTimes int, panicValue any) func(exec failsafe.Execution[R]) (R, error) {
	errorCounter := 0
	return func(exec failsafe.Execution[R]) (R, error) {
		if errorCounter < errorTimes {
			errorCounter++
			return *(new(R)), err
		}
		panic(panicValue)
	}
}

// ErrorNTimesThenError returns a stub function that returns the err errorTimes and then returns the finalError.
// Can be used with failsafe.GetWithExecution.
func ErrorNTimesThenError[R any](err error, errorTimes int, finalError error) func(exec failsafe.Execution[R]) (R, error) {
	errorCounter := 0
	return func(exec failsafe.Execution[R]) (R, error) {
		if errorCounter < errorTimes {
			errorCounter++
			return *(new(R)), err
		}
		return *(new(R)), finalError
	}
}

type TestExecution[R any] struct {
	TheLastResult R
}

func (s TestExecution[R]) Attempts() int {
	panic("unimplemented stub")
}

func (s TestExecution[R]) Executions() int {
	panic("unimplemented stub")
}

func (s TestExecution[R]) StartTime() time.Time {
	panic("unimplemented stub")
}

func (s TestExecution[R]) IsFirstAttempt() bool {
	panic("unimplemented stub")
}

func (s TestExecution[R]) IsRetry() bool {
	panic("unimplemented stub")
}

func (s TestExecution[R]) ElapsedTime() time.Duration {
	panic("unimplemented stub")
}

func (s TestExecution[R]) LastResult() R {
	return s.TheLastResult
}

func (s TestExecution[R]) LastError() error {
	panic("unimplemented stub")
}

func (s TestExecution[R]) AttemptStartTime() time.Time {
	panic("unimplemented stub")
}

func (s TestExecution[R]) ElapsedAttemptTime() time.Duration {
	panic("unimplemented stub")
}

func (s TestExecution[R]) Context() context.Context {
	return nil
}

func (s TestExecution[R]) IsCanceled() bool {
	panic("unimplemented stub")
}

func (s TestExecution[R]) Canceled() <-chan any {
	panic("unimplemented stub")
}
