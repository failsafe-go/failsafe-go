package testutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
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
	_, ok := err.(*CompositeError)
	return ok
}

func NewCompositeError(cause error) *CompositeError {
	return &CompositeError{
		Cause: cause,
	}
}

var ErrInvalidArgument = errors.New("invalid argument")
var ErrInvalidState = errors.New("invalid state")
var ErrConnecting = errors.New("connection error")

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

func MockResponse(statusCode int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, body)
	}))
}

func MockFlakyServer(failTimes int, responseCode int, retryAfterDelay time.Duration, finalResponse string) *httptest.Server {
	failures := atomic.Int32{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if failures.Add(1) <= int32(failTimes) {
			if retryAfterDelay > 0 {
				w.Header().Add("Retry-After", strconv.Itoa(int(retryAfterDelay.Seconds())))
			}
			fmt.Println("Replying with", responseCode)
			w.WriteHeader(responseCode)
		} else {
			fmt.Fprintf(w, finalResponse)
		}
	}))
}

func MockDelayedResponse(statusCode int, body string, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			w.WriteHeader(statusCode)
			fmt.Fprintf(w, body)
		case <-request.Context().Done():
			timer.Stop()
		}
	}))
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
