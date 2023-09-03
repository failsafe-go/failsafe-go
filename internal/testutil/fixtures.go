package testutil

import (
	"failsafe"
)

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

func ErrorNTimesThenReturn[R any](err error, errorTimes int, result R) func(exec failsafe.Execution[R]) (R, error) {
	counter := 0
	return func(exec failsafe.Execution[R]) (R, error) {
		if counter < errorTimes {
			counter++
			defaultResult := *(new(R))
			return defaultResult, err
		}
		return result, nil
	}
}
