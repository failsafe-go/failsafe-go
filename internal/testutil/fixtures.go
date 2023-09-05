package testutil

import "github.com/failsafe-go/failsafe-go"

type InvalidArgumentError struct {
	Cause error
}

func (ce InvalidArgumentError) Error() string {
	return "InvalidArgumentError"
}

func (ce InvalidArgumentError) Unwrap() error {
	return ce.Cause
}

type InvalidStateError struct {
	Cause error
}

func (ce InvalidStateError) Error() string {
	return "InvalidStateError"
}

func (ce InvalidStateError) Unwrap() error {
	return ce.Cause
}

type ConnectionError struct {
	Cause error
}

func (ce ConnectionError) Error() string {
	return "ConnectionError"
}
func (ce ConnectionError) Unwrap() error {
	return ce.Cause
}

type TimeoutError struct {
	Cause error
}

func (ce TimeoutError) Error() string {
	return "TimeoutError"
}

func (ce TimeoutError) Unwrap() error {
	return ce.Cause
}

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
func ErrorNTimesThenReturn[R any](err error, errorTimes int, results ...R) func(exec failsafe.Execution[R]) (R, error) {
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
