package failsafehttp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
)

func TestServerSuccess(t *testing.T) {
	// Given
	handler := testutil.MockHandler(200, "foo")
	rp := NewRetryPolicyBuilder().Build()

	// When / Then
	testServer(t, handler).
		With(rp).
		AssertSuccess(1, 1, 200, "foo")
}

func TestServerBulkhead(t *testing.T) {
	// Given
	handler := testutil.MockHandler(200, "success")
	bh := bulkhead.New[*http.Response](2)
	bh.TryAcquirePermit()
	bh.TryAcquirePermit()

	// When / Then
	testServer(t, handler).
		With(bh).
		AssertFailureResult(1, 0, 429, bulkhead.ErrFull.Error())
}

// Asserts that an open circuit breaker prevents executions from occurring, even with outer retries.
func TestServerCircuitBreaker(t *testing.T) {
	// Given
	handler := testutil.MockHandler(200, "success")
	cb := circuitbreaker.NewWithDefaults[*http.Response]()
	rp := retrypolicy.NewWithDefaults[*http.Response]()
	cb.Open()

	// When / Then
	testServer(t, handler).
		With(rp, cb).
		AssertFailureResult(3, 0, 429, "retries exceeded. last result: <nil>, last error: circuit breaker open")
}

func TestServerHedgePolicy(t *testing.T) {
	// Given
	handler := testutil.MockDelayedHandler(200, "foo", 100*time.Millisecond)
	stats := &policytesting.Stats{}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[*http.Response](80*time.Millisecond), stats).Build()

	// When / Then
	testServer(t, handler).
		Reset(stats).
		With(hp).
		AssertSuccess(2, -1, 200, "foo", func() {
			assert.Equal(t, 1, stats.Hedges())
		})
}

// Asserts that providing a context to either the executor or a request that is canceled results in the execution being canceled.
func TestServerCancelWithContext(t *testing.T) {
	slowCtxFn := testutil.ContextWithCancel(time.Second)
	fastCtxFn := testutil.ContextWithCancel(500 * time.Millisecond)

	tests := []struct {
		name            string
		requestCtxFn    func() context.Context
		executorCtx     context.Context
		failureExpected bool
	}{
		{
			"with request context",
			testutil.CanceledContextFn,
			nil,
			false,
		},
		{
			"with executor context",
			nil,
			fastCtxFn(),
			true,
		},
		{
			"with canceled request context and slow executor context",
			testutil.CanceledContextFn,
			slowCtxFn(),
			false,
		},
		{
			"with canceling executor context and slow request context",
			slowCtxFn,
			fastCtxFn(),
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			handler := testutil.MockDelayedHandler(200, "bad", time.Second)
			rp := retrypolicy.NewBuilder[*http.Response]().AbortOnErrors(context.Canceled).Build()
			executor := failsafe.NewExecutor[*http.Response](rp)
			if tc.executorCtx != nil {
				executor = executor.WithContext(tc.executorCtx)
			}

			// When / Then
			start := time.Now()
			serverTester := testServer(t, handler).
				WithExecutor(executor).
				RequestContext(tc.requestCtxFn)
			if !tc.failureExpected {
				// Handle test cases that do not record a failed execution
				serverTester.AssertError(1, 1, context.Canceled)
			} else {
				// Other context cancelations result ina  500 with a response body
				serverTester.AssertFailureResult(1, 1, 500, context.Canceled.Error())
			}
			assert.True(t, start.Add(time.Second).After(time.Now()), "cancellation should immediately exit execution")
		})
	}
}

// Tests that an execution is canceled when a Timeout occurs.
func TestServerCancelWithTimeout(t *testing.T) {
	// Given
	handler := testutil.MockDelayedHandler(200, "bad", time.Second)
	to := timeout.New[*http.Response](100 * time.Millisecond)

	// When / Then
	start := time.Now()
	testServer(t, handler).
		With(to).
		AssertFailureResult(1, 1, 503, timeout.ErrExceeded.Error())
	assert.True(t, start.Add(time.Second).After(time.Now()), "timeout should immediately exit execution")
}

func testServer(t *testing.T, handler http.HandlerFunc) *tester {
	return &tester{
		tester:  testutil.Test[*http.Response](t),
		handler: handler,
	}
}
