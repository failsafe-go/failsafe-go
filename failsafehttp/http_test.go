package failsafehttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func TestSuccess(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](RetryPolicyBuilder().Build())

	// When / Then
	testRequestSuccess(t, server.URL, executor,
		1, 1, 200, "foo")
}

func TestError(t *testing.T) {
	executor := failsafe.NewExecutor[*http.Response](retrypolicy.Builder[*http.Response]().ReturnLastFailure().Build())

	// When / Then
	testRequestFailureError(t, "http://localhost:55555", executor,
		3, 3, syscall.ECONNREFUSED)
}

func TestRetryPolicyWith429(t *testing.T) {
	server := testutil.MockResponse(429, "foo")
	defer server.Close()
	rp := RetryPolicyBuilder().
		ReturnLastFailure().
		Build()
	executor := failsafe.NewExecutor[*http.Response](rp)

	// When / Then
	testRequestFailureResult(t, server.URL, executor,
		3, 3, 429, "foo")
}

func TestRetryPolicyWith429ThenSuccess(t *testing.T) {
	// Given
	server := testutil.MockFlakyServer(2, 429, 0, "foo")
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](RetryPolicyBuilder().Build())

	// When / Then
	testRequestSuccess(t, server.URL, executor,
		3, 3, 200, "foo")
}

func TestRetryPolicyWithRedirects(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](RetryPolicyBuilder().Build())

	// When / Then
	expectedErr := &url.Error{
		Op:  "Get",
		URL: "/",
		Err: errors.New("stopped after 10 redirects"),
	}
	// expected attempts and executions are only 1 since redirects are followed by the HTTP client
	testRequestSuccessError(t, server.URL, executor,
		1, 1, expectedErr)
}

func TestRetryPolicyWithUnsupportedProtocolScheme(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](RetryPolicyBuilder().Build())

	// When / Then
	expectedErr := &url.Error{
		Op:  "Get",
		URL: "rstp://localhost",
		Err: errors.New("unsupported protocol scheme \"rstp\""),
	}
	testRequestSuccessError(t, "rstp://localhost", executor,
		1, 1, expectedErr)
}

func TestRetryPolicyFallback(t *testing.T) {
	server := testutil.MockResponse(400, "bad")
	defer server.Close()
	is400 := func(response *http.Response, err error) bool {
		return response.StatusCode == 400
	}
	rp := retrypolicy.Builder[*http.Response]().HandleIf(is400).ReturnLastFailure().Build()
	fb := fallback.BuilderWithFunc[*http.Response](func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		response := &http.Response{}
		response.StatusCode = 200
		response.Body = io.NopCloser(bytes.NewBufferString("fallback"))
		return response, nil
	}).HandleIf(is400).Build()
	executor := failsafe.NewExecutor[*http.Response](fb, rp)

	// When / Then
	testRequestSuccess(t, server.URL, executor,
		3, 3, 200, "fallback")
}

// Asserts that an open circuit breaker prevents executions from occurring, even with outer retries.
func TestCircuitBreaker(t *testing.T) {
	cb := circuitbreaker.WithDefaults[*http.Response]()
	rp := retrypolicy.WithDefaults[*http.Response]()
	executor := failsafe.NewExecutor[*http.Response](rp, cb)
	cb.Open()

	// When / Then
	testRequestFailureError(t, "", executor,
		3, 0, circuitbreaker.ErrOpen)
}

func TestTimeout(t *testing.T) {
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](timeout.With[*http.Response](100 * time.Millisecond))

	// When / Then
	testRequestFailureError(t, server.URL, executor,
		1, 1, timeout.ErrExceeded)
}

// Tests that a failsafe roundtripper's requests are canceled when an external context is canceled.
func TestCancelWithContext(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	rp := retrypolicy.WithDefaults[*http.Response]()
	ctx := testutil.SetupWithContextSleep(50 * time.Millisecond)()
	executor := failsafe.NewExecutor[*http.Response](rp).WithContext(ctx)

	// When / Then
	testRequestFailureError(t, server.URL, executor,
		1, 1, context.Canceled)
}

// Tests that a failsafe roundtripper's requests are canceled when an external context is canceled.
func TestCancelWithTimeout(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	to := timeout.With[*http.Response](100 * time.Millisecond)
	executor := failsafe.NewExecutor[*http.Response](to)

	// When / Then
	testRequestFailureError(t, server.URL, executor,
		1, 1, timeout.ErrExceeded)
}

func testRequestSuccess(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, then ...func()) {
	testRequest(t, url, executor, expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, true, then...)
}

func testRequestFailureResult(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, then ...func()) {
	testRequest(t, url, executor, expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, false, then...)
}

func testRequestSuccessError(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testRequest(t, url, executor, expectedAttempts, expectedExecutions, -1, nil, expectedError, true, then...)
}

func testRequestFailureError(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testRequest(t, url, executor, expectedAttempts, expectedExecutions, -1, nil, expectedError, false, then...)
}

func testRequest(t *testing.T, path string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, expectedError error, expectedSuccess bool, then ...func()) {
	var expectedErrPtr *error
	expectedErrPtr = &expectedError
	executorFn, assertResult := testutil.PrepareTest(t, nil, executor, expectedAttempts, expectedExecutions, nil, expectedErrPtr, expectedSuccess, then...)

	// Execute request
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	resp, err := NewRequest(executorFn(), req, http.DefaultClient).Do()
	http.DefaultClient.Get("http://failsafe-go.dev")

	// Read body
	var body string
	if resp != nil {
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			body = string(bodyBytes)
		}
	}

	// Assert result
	if expectedResult != nil {
		assert.Equal(t, expectedResult, body)
	}

	// Unwrap and assert URL errors
	urlErr1, ok1 := err.(*url.Error)
	urlErr2, ok2 := expectedError.(*url.Error)
	if ok1 && ok2 {
		assert.Equal(t, urlErr1.Err.Error(), urlErr2.Err.Error(), "expected error did not match")
		// Clear error vars so that assertResult doesn't assert them
		err = nil
		*expectedErrPtr = nil
	}

	// Assert status
	if resp != nil && expectedStatus != -1 {
		assert.Equal(t, expectedStatus, resp.StatusCode)
	}

	// Assert remaining error and events
	assertResult(nil, err)
}
