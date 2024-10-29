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
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func TestBodyReader(t *testing.T) {
	tests := []struct {
		name         string
		input        any
		expectedBody string
		shouldError  bool
	}{
		{"with nil body", nil, "", false},
		{"with *bytes.Buffer body", bytes.NewBufferString("buffer data"), "buffer data", false},
		{"with *bytes.Reader body", bytes.NewReader([]byte("reader data")), "reader data", false},
		{"with io.ReadSeeker body", strings.NewReader("readseeker data"), "readseeker data", false},
		{"with io.Reader body", strings.NewReader("reader only data"), "reader only data", false},
		{"with unsupported body type", 123, "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFunc, err := bodyReader(tc.input)
			if tc.shouldError {
				assert.Error(t, err)
				return
			}
			if bodyFunc == nil {
				assert.Nil(t, tc.input)
				return
			}

			// Assert that the body can be read multiple times
			for i := 0; i < 2; i++ {
				body, err := bodyFunc()
				assert.NoError(t, err)
				bodyData, err := io.ReadAll(body)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedBody, string(bodyData))
			}
		})
	}
}

func TestSuccess(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	rp := RetryPolicyBuilder().Build()

	// When / Then
	test(t, server).
		With(rp).
		AssertSuccess(1, 1, 200, "foo")
}

func TestRetryPolicyWithError(t *testing.T) {
	test(t, nil).
		With(RetryPolicyBuilder().ReturnLastFailure().Build()).
		Url("http://localhost:55555").
		AssertFailure(3, 3, syscall.ECONNREFUSED)
}

func TestRetryPolicyWith429(t *testing.T) {
	// Given
	server := testutil.MockResponse(429, "foo")
	rp := RetryPolicyBuilder().ReturnLastFailure().Build()

	// When / Then
	test(t, server).
		With(rp).
		AssertFailureResult(3, 3, 429, "foo")
}

func TestRetryPolicyWith429ThenSuccess(t *testing.T) {
	// Given
	server, setup := testutil.MockFlakyServer(2, 429, 0, "foo")
	rp := RetryPolicyBuilder().Build()

	// When / Then
	test(t, server).
		Setup(setup).
		With(rp).
		AssertSuccess(3, 3, 200, "foo")
}

func TestRetryPolicyWithRedirects(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	rp := RetryPolicyBuilder().Build()

	// When / Then
	// expected attempts and executions are only 1 since redirects are followed by the HTTP client
	expectedErr := &url.Error{
		Op:  "Get",
		URL: "/",
		Err: errors.New("stopped after 10 redirects"),
	}
	test(t, server).
		With(rp).
		AssertSuccessError(1, 1, expectedErr)
}

func TestRetryPolicyWithUnsupportedProtocolScheme(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	rp := RetryPolicyBuilder().Build()

	// When / Then
	expectedErr := &url.Error{
		Op:  "Get",
		URL: "rstp://localhost",
		Err: errors.New("unsupported protocol scheme \"rstp\""),
	}
	test(t, server).
		Url("rstp://localhost").
		With(rp).
		AssertSuccessError(1, 1, expectedErr)
}

func TestRetryPolicyFallback(t *testing.T) {
	// Given
	server := testutil.MockResponse(429, "bad")
	rp := RetryPolicyBuilder().ReturnLastFailure().Build()
	fb := fallback.BuilderWithFunc[*http.Response](func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		response := &http.Response{}
		response.StatusCode = 200
		response.Body = io.NopCloser(bytes.NewBufferString("fallback"))
		return response, nil
	}).HandleIf(func(response *http.Response, err error) bool {
		return (response != nil && response.StatusCode == 429) || err != nil
	}).Build()

	tests := []struct {
		name             string
		requestCtxFn     func() context.Context
		expectedAttempts int
	}{
		{
			"with bad request",
			nil,
			3,
		},
		{
			"with canceled request",
			testutil.CanceledContextFn,
			1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// When / Then
			test(t, server).
				Context(tc.requestCtxFn).
				With(fb, rp).
				AssertSuccess(tc.expectedAttempts, tc.expectedAttempts, 200, "fallback")
		})
	}
}

// Asserts that an open circuit breaker prevents executions from occurring, even with outer retries.
func TestCircuitBreaker(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "success")
	cb := circuitbreaker.WithDefaults[*http.Response]()
	rp := retrypolicy.WithDefaults[*http.Response]()
	cb.Open()

	// When / Then
	test(t, server).
		With(rp, cb).
		AssertFailure(3, 0, circuitbreaker.ErrOpen)
}

func TestHedgePolicy(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "foo", 100*time.Millisecond)
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.BuilderWithDelay[*http.Response](80*time.Millisecond), stats).Build()

	// When / Then
	test(t, server).
		Reset(stats).
		With(hp).
		AssertSuccess(2, -1, 200, "foo", func() {
			assert.Equal(t, 1, stats.Hedges())
		})
}

// Asserts that providing a context to either the executor or a request that is canceled results in the execution being canceled.
func TestCancelWithContext(t *testing.T) {
	slowCtxFn := testutil.SetupWithContextSleep(time.Second)
	fastCtxFn := testutil.SetupWithContextSleep(50 * time.Millisecond)

	tests := []struct {
		name         string
		requestCtxFn func() context.Context
		executorCtx  context.Context
	}{
		{
			"with request context",
			fastCtxFn,
			nil,
		},
		{
			"with executor context",
			nil,
			fastCtxFn(),
		},
		{
			"with canceling request context and slow executor context",
			fastCtxFn,
			slowCtxFn(),
		},
		{
			"with canceling executor context and slow request context",
			slowCtxFn,
			fastCtxFn(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			server := testutil.MockDelayedResponse(200, "bad", time.Second)
			t.Cleanup(server.Close)
			rp := retrypolicy.Builder[*http.Response]().AbortOnErrors(context.Canceled).Build()
			executor := failsafe.NewExecutor[*http.Response](rp)
			if tc.executorCtx != nil {
				executor = executor.WithContext(tc.executorCtx)
			}

			// When / Then
			start := time.Now()
			test(t, server).
				WithExecutor(executor).
				Context(tc.requestCtxFn).
				AssertFailure(1, 1, context.Canceled)
			assert.True(t, start.Add(time.Second).After(time.Now()), "cancellation should immediately exit execution")
		})
	}
}

// Tests that an execution is canceled when a Timeout occurs.
func TestCancelWithTimeout(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	to := timeout.With[*http.Response](100 * time.Millisecond)

	// When / Then
	start := time.Now()
	test(t, server).
		With(to).
		AssertFailure(1, 1, timeout.ErrExceeded)
	assert.True(t, start.Add(time.Second).After(time.Now()), "timeout should immediately exit execution")
}

type tester struct {
	tester *testutil.Tester[*http.Response]
	server *httptest.Server
	url    string
}

func test(t *testing.T, server *httptest.Server) *tester {
	return &tester{
		tester: testutil.Test[*http.Response](t),
		server: server,
	}
}

func (t *tester) Url(url string) *tester {
	t.url = url
	return t
}

func (t *tester) Setup(fn func()) *tester {
	t.tester.Setup(fn)
	return t
}

func (t *tester) Context(fn func() context.Context) *tester {
	t.tester.Context(fn)
	return t
}

func (t *tester) Reset(stats ...testutil.Resetable) *tester {
	t.tester.Reset(stats...)
	return t
}

func (t *tester) With(policies ...failsafe.Policy[*http.Response]) *tester {
	t.tester.With(policies...)
	return t
}

func (t *tester) WithExecutor(executor failsafe.Executor[*http.Response]) *tester {
	t.tester.WithExecutor(executor)
	return t
}

func (t *tester) AssertSuccess(expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult string, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, true, then...)
}

func (t *tester) AssertSuccessError(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, 0, "", expectedError, true, then...)
}

func (t *tester) AssertFailure(expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, 0, "", expectedError, false, then...)
}

func (t *tester) AssertFailureResult(expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult string, then ...func()) {
	t.assertResult(expectedAttempts, expectedExecutions, expectedStatus, expectedResult, nil, false, then...)
}

func (t *tester) assertResult(expectedAttempts int, expectedExecutions int, expectedStatus int, expectedResult string, expectedError error, expectedSuccess bool, then ...func()) {
	t.tester.T.Helper()

	executorFn, assertResult := testutil.PrepareTest(t.tester.T, t.tester.SetupFn, nil, t.tester.Executor)
	assertHttpResult := func(resp *http.Response, err error) {
		// Read body
		var body string
		if resp != nil {
			t.tester.T.Cleanup(func() {
				resp.Body.Close()
			})
			bodyBytes, err := io.ReadAll(resp.Body)
			if err == nil {
				body = string(bodyBytes)
			}
		}

		// Assert result
		if expectedResult != "" {
			assert.Equal(t.tester.T, expectedResult, body)
		}

		// Unwrap and assert URL errors
		expectedErrCopy := expectedError
		var urlErr1 *url.Error
		var urlErr2 *url.Error
		if errors.As(err, &urlErr1) && errors.As(expectedError, &urlErr2) {
			assert.Equal(t.tester.T, urlErr1.Err.Error(), urlErr2.Err.Error(), "expected error did not match")
			// Clear error vars so that assertResult doesn't assert them again
			expectedErrCopy = nil
			err = nil
		}

		// Assert status
		if resp != nil && expectedStatus > 0 {
			assert.Equal(t.tester.T, expectedStatus, resp.StatusCode)
		}

		// Assert remaining error and events
		assertResult(expectedAttempts, expectedExecutions, nil, nil, expectedErrCopy, err, expectedSuccess, !expectedSuccess, false, then...)
	}
	ctxFn := func() context.Context {
		if t.tester.ContextFn != nil {
			return t.tester.ContextFn()
		}
		return context.Background()
	}
	if t.url == "" {
		t.url = t.server.URL
	}

	// Test with roundtripper
	fmt.Println("Testing RoundTripper")
	assertHttpResult(testRoundTripper(ctxFn(), t.url, executorFn()))

	// Test with failsafehttp.Request
	fmt.Println("\nTesting failsafehttp.Request")
	assertHttpResult(testRequest(ctxFn(), t.url, executorFn()))

	if t.server != nil {
		t.server.Close()
	}
}

func testRoundTripper(ctx context.Context, path string, executor failsafe.Executor[*http.Response]) (resp *http.Response, err error) {
	client := http.Client{Transport: NewRoundTripperWithExecutor(nil, executor)}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	return client.Do(req)
}

func testRequest(ctx context.Context, path string, executor failsafe.Executor[*http.Response]) (resp *http.Response, err error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	return NewRequestWithExecutor(req, http.DefaultClient, executor).Do()
}
