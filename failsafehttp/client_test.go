package failsafehttp

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func TestClientSuccess(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	rp := NewRetryPolicyBuilder().Build()

	// When / Then
	test(t, server).
		With(rp).
		AssertSuccess(1, 1, 200, "foo")
}

func TestClientRetryPolicyWithError(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	server.Close()

	// When / Then
	test(t, server).
		With(NewRetryPolicyBuilder().ReturnLastFailure().Build()).
		Url("http://localhost:55555").
		AssertFailure(3, 3, syscall.ECONNREFUSED)
}

func TestClientRetryPolicyWith429(t *testing.T) {
	// Given
	server := testutil.MockResponse(429, "foo")
	rp := NewRetryPolicyBuilder().ReturnLastFailure().Build()

	// When / Then
	test(t, server).
		With(rp).
		AssertFailureResult(3, 3, 429, "foo")
}

func TestClientRetryPolicyWith429ThenSuccess(t *testing.T) {
	// Given
	server, setup := testutil.MockFlakyServer(2, 429, 0, "foo")
	rp := NewRetryPolicyBuilder().Build()

	// When / Then
	test(t, server).
		Setup(setup).
		With(rp).
		AssertSuccess(3, 3, 200, "foo")
}

func TestClientRetryPolicyWithRedirects(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	rp := NewRetryPolicyBuilder().Build()

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

func TestClientRetryPolicyWithUnsupportedProtocolScheme(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	rp := NewRetryPolicyBuilder().Build()

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

func TestClientRetryPolicyFallback(t *testing.T) {
	// Given
	server := testutil.MockResponse(429, "bad")
	rp := NewRetryPolicyBuilder().ReturnLastFailure().Build()
	fb := fallback.NewBuilderWithFunc[*http.Response](func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
		response := &http.Response{}
		response.StatusCode = 200
		response.Body = io.NopCloser(bytes.NewBufferString("fallback"))
		return response, nil
	}).HandleIf(func(response *http.Response, err error) bool {
		return (response != nil && response.StatusCode == 429) || err != nil
	}).Build()

	// When / Then
	test(t, server).
		With(fb, rp).
		AssertSuccess(3, 3, 200, "fallback")
}

func TestClientBulkhead(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "success")
	bh := bulkhead.New[*http.Response](2)
	bh.TryAcquirePermit()
	bh.TryAcquirePermit()

	// When / Then
	test(t, server).
		With(bh).
		AssertFailure(1, 0, bulkhead.ErrFull)
}

// Asserts that an open circuit breaker prevents executions from occurring, even with outer retries.
func TestClientCircuitBreaker(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "success")
	cb := circuitbreaker.NewWithDefaults[*http.Response]()
	rp := retrypolicy.NewWithDefaults[*http.Response]()
	cb.Open()

	// When / Then
	test(t, server).
		With(rp, cb).
		AssertFailure(3, 0, circuitbreaker.ErrOpen)
}

func TestClientHedgePolicy(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "foo", 100*time.Millisecond)
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[*http.Response](80*time.Millisecond), stats).Build()

	// When / Then
	test(t, server).
		Reset(stats).
		With(hp).
		AssertSuccess(2, -1, 200, "foo", func() {
			assert.Equal(t, 1, stats.Hedges())
		})
}

// Asserts that providing a context to either the executor or a request that is canceled results in the execution being canceled.
func TestClientCancelWithContext(t *testing.T) {
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
			rp := retrypolicy.NewBuilder[*http.Response]().AbortOnErrors(context.Canceled).Build()
			executor := failsafe.NewExecutor[*http.Response](rp)
			if tc.executorCtx != nil {
				executor = executor.WithContext(tc.executorCtx)
			}

			// When / Then
			start := time.Now()
			test(t, server).
				WithExecutor(executor).
				RequestContext(tc.requestCtxFn).
				AssertFailure(1, 1, context.Canceled)
			assert.True(t, start.Add(time.Second).After(time.Now()), "cancellation should immediately exit execution")
		})
	}
}

// Tests that an execution is canceled when a Timeout occurs.
func TestClientCancelWithTimeout(t *testing.T) {
	// Given
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	to := timeout.New[*http.Response](100 * time.Millisecond)

	// When / Then
	start := time.Now()
	test(t, server).
		With(to).
		AssertFailure(1, 1, timeout.ErrExceeded)
	assert.True(t, start.Add(time.Second).After(time.Now()), "timeout should immediately exit execution")
}

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
