package failsafehttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	testHttpSuccess(t, server.URL, executor,
		1, 1, 200, "foo")
}

func TestError(t *testing.T) {
	executor := failsafe.NewExecutor[*http.Response](retrypolicy.Builder[*http.Response]().ReturnLastFailure().Build())

	// When / Then
	testHttpFailureError(t, nil, "http://localhost:55555", executor,
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
	testHttpFailureResult(t, server.URL, executor,
		3, 3, 429, "foo")
}

func TestRetryPolicyWith429ThenSuccess(t *testing.T) {
	// Given
	server, reset := testutil.MockFlakyServer(2, 429, 0, "foo")
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](RetryPolicyBuilder().Build())

	// When / Then
	testHttpSuccess(t, server.URL, executor,
		3, 3, 200, "foo", reset)
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
	testHttpSuccessError(t, server.URL, executor,
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
	testHttpSuccessError(t, "rstp://localhost", executor,
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
	testHttpSuccess(t, server.URL, executor,
		3, 3, 200, "fallback")
}

// Asserts that an open circuit breaker prevents executions from occurring, even with outer retries.
func TestCircuitBreaker(t *testing.T) {
	cb := circuitbreaker.WithDefaults[*http.Response]()
	rp := retrypolicy.WithDefaults[*http.Response]()
	executor := failsafe.NewExecutor[*http.Response](rp, cb)
	cb.Open()

	// When / Then
	testHttpFailureError(t, nil, "", executor,
		3, 0, circuitbreaker.ErrOpen)
}

func TestCancelWithContext(t *testing.T) {
	slowCtxFn := testutil.SetupWithContextSleep(time.Second)
	fastCtxFn := testutil.SetupWithContextSleep(50 * time.Millisecond)

	tests := []struct {
		name         string
		requestCtxFn func() context.Context
		executorCtx  context.Context
	}{
		{
			"with executor context",
			nil,
			fastCtxFn(),
		},
		{
			"with request context",
			fastCtxFn,
			nil,
		},
		{
			"with executor context and canceling request context",
			fastCtxFn,
			slowCtxFn(),
		},
		{
			"with canceling executor context and request context",
			slowCtxFn,
			fastCtxFn(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			server := testutil.MockDelayedResponse(200, "bad", time.Second)
			defer server.Close()
			rp := retrypolicy.Builder[*http.Response]().AbortOnErrors(context.Canceled).Build()
			executor := failsafe.NewExecutor[*http.Response](rp)
			if tt.executorCtx != nil {
				executor = executor.WithContext(tt.executorCtx)
			}

			// When / Then
			start := time.Now()
			testHttpFailureError(t, tt.requestCtxFn, server.URL, executor,
				1, 1, context.Canceled)
			assert.True(t, start.Add(time.Second).After(time.Now()), "cancellation should immediately exit execution")
		})
	}
}

// Tests that an execution is canceled when a Timeout occurs.
func TestCancelWithTimeout(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](timeout.With[*http.Response](100 * time.Millisecond))

	// When / Then
	start := time.Now()
	testHttpFailureError(t, nil, server.URL, executor,
		1, 1, timeout.ErrExceeded)
	assert.True(t, start.Add(time.Second).After(time.Now()), "timeout should immediately exit execution")
}

func testHttpSuccess(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, then ...func()) {
	testHttp(t, nil, url, executor, expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, true, then...)
}

func testHttpFailureResult(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, then ...func()) {
	testHttp(t, nil, url, executor, expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, false, then...)
}

func testHttpSuccessError(t *testing.T, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testHttp(t, nil, url, executor, expectedAttempts, expectedExecutions, -1, nil, expectedError, true, then...)
}

func testHttpFailureError(t *testing.T, requestCtxFn func() context.Context, url string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testHttp(t, requestCtxFn, url, executor, expectedAttempts, expectedExecutions, -1, nil, expectedError, false, then...)
}

// testHttp tests http behavior using a RoundTripper and a failsafehttp.Request.
func testHttp(t *testing.T, requestCtxFn func() context.Context, path string, executor failsafe.Executor[*http.Response], expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult any, expectedError error, expectedSuccess bool, thens ...func()) {
	executorFn, assertResult := testutil.PrepareTest(t, nil, executor)
	assertHttpResult := func(resp *http.Response, err error) {
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
		expectedErrCopy := expectedError
		urlErr1, ok1 := err.(*url.Error)
		urlErr2, ok2 := expectedError.(*url.Error)
		if ok1 && ok2 {
			assert.Equal(t, urlErr1.Err.Error(), urlErr2.Err.Error(), "expected error did not match")
			// Clear error vars so that assertResult doesn't assert them again
			expectedErrCopy = nil
			err = nil
		}

		// Assert status
		if resp != nil && expectedStatus != -1 {
			assert.Equal(t, expectedStatus, resp.StatusCode)
		}

		// Assert remaining error and events
		var then func()
		if len(thens) > 0 {
			then = thens[0]
		}
		assertResult(expectedAttempts, expectedExecutions, nil, nil, expectedErrCopy, err, expectedSuccess, !expectedSuccess, then)
	}

	ctx := context.Background()
	if requestCtxFn != nil {
		ctx = requestCtxFn()
	}

	// Test with roundtripper
	fmt.Println("Testing RoundTripper")
	assertHttpResult(testRoundTripper(ctx, path, executorFn))

	// Test with failsafehttp.Request
	fmt.Println("\nTesting failsafehttp.Request")
	assertHttpResult(testRequest(ctx, path, executorFn))
}

func testRoundTripper(ctx context.Context, path string, executorFn func() failsafe.Executor[*http.Response]) (resp *http.Response, err error) {
	client := http.Client{Transport: NewRoundTripperWithExecutor(nil, executorFn())}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	return client.Do(req)
}

func testRequest(ctx context.Context, path string, executorFn func() failsafe.Executor[*http.Response]) (resp *http.Response, err error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	return NewRequestWithExecutor(req, http.DefaultClient, executorFn()).Do()
}
