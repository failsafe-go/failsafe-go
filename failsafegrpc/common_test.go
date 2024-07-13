package failsafegrpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/internal/testutil/pbfixtures"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/failsafe-go/failsafe-go/timeout"
)

// Asserts that load limiting policies prevent executions from occurring on the client or server side.
func TestLoadLimiting(t *testing.T) {
	cb := circuitbreaker.WithDefaults[any]()
	cb.Open()
	bh := bulkhead.With[any](1)
	bh.TryAcquirePermit() // Exhaust permits
	rl := ratelimiter.Bursty[any](1, time.Minute)
	rl.TryAcquirePermit() // Exhaust permits

	tests := []struct {
		name        string
		policy      failsafe.Policy[any]
		expectedErr error
	}{
		{
			"with circuit breaker",
			cb,
			circuitbreaker.ErrOpen,
		},
		{
			"with bulkhead",
			bh,
			bulkhead.ErrFull,
		},
		{
			"with rate limiter",
			rl,
			ratelimiter.ErrExceeded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			server := testutil.MockGrpcResponses("foo")
			executor := failsafe.NewExecutor[any](tc.policy)

			// When / Then
			testClientFailure(t, nil, server, executor,
				1, 0, tc.expectedErr)

			// When / Then
			testServerFailure(t, nil, server, executor,
				1, 0, tc.expectedErr, true)
		})
	}
}

func TestCircuitBreakerWithResult(t *testing.T) {
	server := testutil.MockGrpcResponses("test")

	tests := []struct {
		name     string
		assertFn func(*testing.T, failsafe.Executor[*pbfixtures.PingResponse])
	}{
		{
			"for client",
			func(t *testing.T, executor failsafe.Executor[*pbfixtures.PingResponse]) {
				testClientFailureResult(t, nil, server, executor,
					1, 1, "test")
			},
		},
		{
			"for server",
			func(t *testing.T, executor failsafe.Executor[*pbfixtures.PingResponse]) {
				testServerFailureResult(t, nil, server, executor,
					1, 1, "test", false)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			cb := circuitbreaker.Builder[*pbfixtures.PingResponse]().HandleIf(func(r *pbfixtures.PingResponse, err error) bool {
				return r.Msg == "test"
			}).Build()

			// When / Then
			executor := failsafe.NewExecutor[*pbfixtures.PingResponse](cb)
			tc.assertFn(t, executor)
			assert.True(t, cb.IsOpen())
		})
	}
}

// Tests that an execution is canceled when a Timeout occurs.
func TestCancelWithTimeout(t *testing.T) {
	server := testutil.MockDelayedGrpcResponse("pong", time.Second)

	tests := []struct {
		name     string
		assertFn func(*testing.T, failsafe.Executor[any])
	}{
		{
			"for client",
			func(t *testing.T, executor failsafe.Executor[any]) {
				testClientFailure(t, nil, server, executor,
					1, 1, timeout.ErrExceeded)
			},
		},
		{
			"for server",
			func(t *testing.T, executor failsafe.Executor[any]) {
				testServerFailure(t, nil, server, executor,
					1, 1, timeout.ErrExceeded, false)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			executor := failsafe.NewExecutor[any](timeout.With[any](100 * time.Millisecond))

			// When / Then
			start := time.Now()
			testClientFailure(t, nil, server, executor,
				1, 1, timeout.ErrExceeded)
			assert.True(t, start.Add(time.Second).After(time.Now()), "timeout should immediately exit execution")
		})
	}
}
