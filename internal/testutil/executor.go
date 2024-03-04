package testutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
)

type Given func() context.Context
type WhenRun[R any] func(execution failsafe.Execution[R]) error
type WhenGet[R any] func(execution failsafe.Execution[R]) (R, error)

func TestRunSuccess[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, then ...func()) {
	testRun(t, given, executor, when, expectedAttempts, expectedExecutions, nil, then...)
}

func TestRunFailure[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testRun(t, given, executor, when, expectedAttempts, expectedExecutions, expectedError, then...)
}

func TestGetSuccess[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, then ...func()) {
	testGet(t, given, executor, when, expectedAttempts, expectedExecutions, expectedResult, nil, then...)
}

func TestGetFailure[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testGet(t, given, executor, when, expectedAttempts, expectedExecutions, *(new(R)), expectedError, then...)
}

func testRun[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenRun[R], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	defaultR := *(new(R))
	executorFn, assertResult := prepareTest(t, given, executor, expectedAttempts, expectedExecutions, defaultR, expectedError, expectedError == nil, then...)

	// Run sync
	fmt.Println("Testing sync")
	assertResult(defaultR, executorFn().RunWithExecution(when))

	// Run async
	fmt.Println("\nTesting async")
	assertResult(executorFn().RunWithExecutionAsync(when).Get())
}

func testGet[R any](t *testing.T, given Given, executor failsafe.Executor[R], when WhenGet[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error, then ...func()) {
	executorFn, assertResult := prepareTest(t, given, executor, expectedAttempts, expectedExecutions, expectedResult, expectedError, expectedError == nil, then...)

	// Run sync
	fmt.Println("Testing sync")
	assertResult(executorFn().GetWithExecution(when))

	// Run async
	fmt.Println("\nTesting async")
	assertResult(executorFn().GetWithExecutionAsync(when).Get())
}

func prepareTest[R any](t *testing.T, given Given, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult R, expectedError error, expectedSuccess bool, then ...func()) (executorFn func() failsafe.Executor[R], assertResult func(R, error)) {
	var doneEvent *failsafe.ExecutionDoneEvent[R]
	onSuccessCalled := false
	onFailureCalled := false
	executor = executor.OnDone(func(e failsafe.ExecutionDoneEvent[R]) {
		doneEvent = &e
	}).OnSuccess(func(e failsafe.ExecutionDoneEvent[R]) {
		onSuccessCalled = true
	}).OnFailure(func(e failsafe.ExecutionDoneEvent[R]) {
		onFailureCalled = true
	})
	executorFn = func() failsafe.Executor[R] {
		if given != nil {
			executor = executor.WithContext(given())
		}
		return executor
	}
	assertResult = func(result R, err error) {
		if len(then) > 0 {
			then[0]()
		}
		if expectedAttempts != -1 {
			assert.Equal(t, expectedAttempts, doneEvent.Attempts(), "expected attempts did not match")
		}
		if expectedExecutions != -1 {
			assert.Equal(t, expectedExecutions, doneEvent.Executions(), "expected executions did not match")
		}
		// Assert non-http response results
		_, isHttpResponse := any(result).(*http.Response)
		if any(expectedResult) == nil && !isHttpResponse {
			assert.Equal(t, expectedResult, result, "expected result did not match")
		}
		assert.ErrorIs(t, err, expectedError, "expected error did not match")
		if expectedSuccess {
			assert.True(t, onSuccessCalled, "onSuccess should have been called")
			assert.False(t, onFailureCalled, "onFailure should not have been called")
		} else {
			assert.False(t, onSuccessCalled, "onSuccess should not have been called")
			assert.True(t, onFailureCalled, "onFailure should have been called")
		}
	}
	return
}

func TestRequestSuccess(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, then ...func()) {
	testRequest(t, url, executor, expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, true, then...)
}

func TestRequestFailureResult(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, then ...func()) {
	testRequest(t, url, executor, expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, false, then...)
}

func TestRequestFailureError(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testRequest(t, url, executor, expectedAttempts, expectedExecutions, -1, nil, expectedError, false, then...)
}

func testRequest(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, expectedError error, expectedSuccess bool, then ...func()) {
	executorFn, assertResult := prepareTest(t, nil, executor, expectedAttempts, expectedExecutions, nil, expectedError, expectedSuccess, then...)

	// Build and send HTTP request
	client := http.Client{Transport: &roundTripper{executor: executorFn()}}
	resp, err := client.Get(url)

	var body string
	if resp != nil {
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		body = string(bodyBytes)
	}

	// Assert results
	assertResult(resp, err)
	if resp != nil {
		assert.Equal(t, expectedStatus, resp.StatusCode)
	}
	if expectedResult != nil {
		assert.Equal(t, expectedResult, body)
	}
}

type roundTripper struct {
	executor failsafe.Executor[*http.Response]
}

func (f *roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f.executor.GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		return http.DefaultTransport.RoundTrip(request.WithContext(exec.Context()))
	})
}
