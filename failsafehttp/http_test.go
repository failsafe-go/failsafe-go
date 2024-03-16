package failsafehttp

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"syscall"
	"testing"
	"time"

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
	testutil.TestRequestSuccess(t, server.URL, executor,
		1, 1, 200, "foo")
}

func TestError(t *testing.T) {
	executor := failsafe.NewExecutor[*http.Response](retrypolicy.Builder[*http.Response]().ReturnLastFailure().Build())

	// When / Then
	testutil.TestRequestFailureError(t, "http://localhost:55555", executor,
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
	testutil.TestRequestFailureResult(t, server.URL, executor,
		3, 3, 429, "foo")
}

func TestRetryPolicyWith429ThenSuccess(t *testing.T) {
	// Given
	server := testutil.MockFlakyServer(2, 429, 0, "foo")
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](RetryPolicyBuilder().Build())

	// When / Then
	testutil.TestRequestSuccess(t, server.URL, executor,
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
	testutil.TestRequestSuccessError(t, server.URL, executor,
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
	testutil.TestRequestSuccessError(t, "rstp://localhost", executor,
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
	testutil.TestRequestSuccess(t, server.URL, executor,
		3, 3, 200, "fallback")
}

// Asserts that an open circuit breaker prevents executions from occurring, even with outer retries.
func TestCircuitBreaker(t *testing.T) {
	cb := circuitbreaker.WithDefaults[*http.Response]()
	rp := retrypolicy.WithDefaults[*http.Response]()
	executor := failsafe.NewExecutor[*http.Response](rp, cb)
	cb.Open()

	// When / Then
	testutil.TestRequestFailureError(t, "", executor,
		3, 0, circuitbreaker.ErrOpen)
}

func TestTimeout(t *testing.T) {
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](timeout.With[*http.Response](100 * time.Millisecond))

	// When / Then
	testutil.TestRequestFailureError(t, server.URL, executor,
		1, 1, timeout.ErrExceeded)
}
