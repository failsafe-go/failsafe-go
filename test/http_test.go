package test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestHttpSuccess(t *testing.T) {
	// Given
	server := testutil.MockResponse(200, "foo")
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](retrypolicy.WithDefaults[*http.Response]())

	// When / Then
	testutil.TestRequestSuccess(t, server.URL, executor,
		1, 1, 200, "foo")
}

func TestHttpRetryPolicyOn400(t *testing.T) {
	server := testutil.MockResponse(400, "foo")
	defer server.Close()
	rp := retrypolicy.Builder[*http.Response]().
		HandleIf(func(response *http.Response, err error) bool {
			return response.StatusCode == 400
		}).
		ReturnLastFailure().
		Build()
	executor := failsafe.NewExecutor[*http.Response](rp)

	// When / Then
	testutil.TestRequestFailureResult(t, server.URL, executor,
		3, 3, 400, "foo")
}

func TestHttpRetryPolicy400ThenSuccess(t *testing.T) {
	// Given
	count := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) < 3 {
			w.WriteHeader(400)
		} else {
			fmt.Fprintf(w, "foo")
		}
	}))
	defer server.Close()
	rp := retrypolicy.Builder[*http.Response]().HandleIf(func(response *http.Response, err error) bool {
		return response.StatusCode == 400
	}).Build()
	executor := failsafe.NewExecutor[*http.Response](rp)

	// When / Then
	testutil.TestRequestSuccess(t, server.URL, executor,
		3, 3, 200, "foo")
}

func TestHttpRetryPolicyFallback(t *testing.T) {
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
func TestHttpCircuitBreaker(t *testing.T) {
	cb := circuitbreaker.WithDefaults[*http.Response]()
	rp := retrypolicy.WithDefaults[*http.Response]()
	executor := failsafe.NewExecutor[*http.Response](rp, cb)
	cb.Open()

	// When / Then
	testutil.TestRequestFailureError(t, "", executor,
		3, 0, circuitbreaker.ErrOpen)
}

func TestHttpTimeout(t *testing.T) {
	server := testutil.MockDelayedResponse(200, "bad", time.Second)
	defer server.Close()
	executor := failsafe.NewExecutor[*http.Response](timeout.With[*http.Response](100 * time.Millisecond))

	// When / Then
	testutil.TestRequestFailureError(t, server.URL, executor,
		1, 1, timeout.ErrExceeded)
}

func TestHttpError(t *testing.T) {
	executor := failsafe.NewExecutor[*http.Response](retrypolicy.Builder[*http.Response]().ReturnLastFailure().Build())

	// When / Then
	testutil.TestRequestFailureError(t, "http://localhost:55555", executor,
		3, 3, syscall.ECONNREFUSED)
}
