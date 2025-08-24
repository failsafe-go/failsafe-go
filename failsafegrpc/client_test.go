package failsafegrpc

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/hedgepolicy"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/internal/testutil/pbfixtures"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestClientSuccess(t *testing.T) {
	// Given
	mockedResponse := "pong"
	server := testutil.MockGrpcResponses(mockedResponse)
	executor := failsafe.NewExecutor[any](NewRetryPolicyBuilder[any]().Build())

	// When / Then
	testClientSuccess(t, nil, server, executor,
		1, 1, mockedResponse)
}

func TestClientRetryPolicy(t *testing.T) {
	tests := []struct {
		name string
		code codes.Code
	}{
		{
			"with unavailable error",
			codes.Unavailable,
		},
		{
			"with deadline exceeded error",
			codes.DeadlineExceeded,
		},
		{
			"with resource exhausted error",
			codes.ResourceExhausted,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			mockedErr := status.Error(tc.code, "err")
			server := testutil.MockGrpcError(mockedErr)
			executor := failsafe.NewExecutor[any](retrypolicy.NewBuilder[any]().ReturnLastFailure().Build())

			// When / Then
			testClientFailure(t, nil, server, executor,
				3, 3, mockedErr)
		})
	}
}

func TestClientRetryPolicyWithUnavailableThenSuccess(t *testing.T) {
	// Given
	server := testutil.MockFlakyGrpcServer(2, status.Error(codes.Unavailable, "err"), "pong")
	executor := failsafe.NewExecutor[any](NewRetryPolicyBuilder[any]().ReturnLastFailure().Build())

	// When / Then
	testClientSuccess(t, nil, server, executor,
		3, 3, "pong")
}

func TestClientRetryOnResult(t *testing.T) {
	// Given
	server := testutil.MockGrpcResponses("retry", "retry", "pong")
	retryPolicy := NewRetryPolicyBuilder[*pbfixtures.PingResponse]().
		HandleIf(func(response *pbfixtures.PingResponse, err error) bool {
			return response.Msg == "retry"
		}).
		Build()
	executor := failsafe.NewExecutor[*pbfixtures.PingResponse](retryPolicy)

	// When / Then
	testClientSuccess(t, nil, server, executor,
		3, 3, "pong")
}

func TestClientRetryPolicyFallback(t *testing.T) {
	// Given
	server := testutil.MockGrpcError(errors.New("err"))
	rp := retrypolicy.NewBuilder[*pbfixtures.PingResponse]().ReturnLastFailure().Build()
	fb := fallback.NewWithFunc(func(exec failsafe.Execution[*pbfixtures.PingResponse]) (*pbfixtures.PingResponse, error) {
		exec.LastResult().Msg = "pong"
		return nil, nil
	})
	executor := failsafe.NewExecutor[*pbfixtures.PingResponse](fb, rp)

	// When / Then
	testClientSuccess(t, nil, server, executor,
		3, 3, "pong")
}

func TestHedgePolicy(t *testing.T) {
	// Given
	server := testutil.MockDelayedGrpcResponse("foo", 100*time.Millisecond)
	stats := &policytesting.Stats{}
	setup := func() context.Context {
		stats.Reset()
		return context.Background()
	}
	hp := policytesting.WithHedgeStatsAndLogs(hedgepolicy.NewBuilderWithDelay[*http.Response](80*time.Millisecond), stats).Build()
	executor := failsafe.NewExecutor[*http.Response](hp)

	// When / Then
	testClientSuccess(t, setup, server, executor,
		2, -1, "foo", func() {
			assert.Equal(t, 1, stats.Hedges())
		})
}

// Asserts that providing a context to either the executor or a request that is canceled results in the execution being canceled.
func TestClientCancelWithContext(t *testing.T) {
	slowCtxFn := testutil.ContextWithCancel(time.Second)
	fastCtxFn := testutil.ContextWithCancel(50 * time.Millisecond)

	tests := []struct {
		name         string
		expectedErr  error
		requestCtxFn func() context.Context
		executorCtx  context.Context
	}{
		{
			"with request context",
			status.Error(codes.Canceled, "context canceled"),
			fastCtxFn,
			nil,
		},
		{
			"with executor context",
			context.Canceled,
			nil,
			fastCtxFn(),
		},
		{
			"with canceling request context and slow executor context",
			context.Canceled,
			fastCtxFn,
			slowCtxFn(),
		},
		{
			"with canceling executor context and slow request context",
			context.Canceled,
			slowCtxFn,
			fastCtxFn(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			server := testutil.MockDelayedGrpcResponse("pong", time.Second)
			rp := retrypolicy.NewBuilder[any]().AbortOnErrors(tc.expectedErr).Build()
			executor := failsafe.NewExecutor[any](rp)
			if tc.executorCtx != nil {
				executor = executor.WithContext(tc.executorCtx)
			}

			// When / Then
			start := time.Now()
			testClientFailure(t, tc.requestCtxFn, server, executor,
				1, 1, tc.expectedErr)
			assert.True(t, start.Add(time.Second).After(time.Now()), "cancellation should immediately exit execution")
		})
	}
}

func testClientSuccess[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult any, then ...func()) {
	testClient(t, requestCtxFn, server, executor, expectedAttempts, expectedExecutions, expectedResult, nil, true, then...)
}

func testClientFailure[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedError error, then ...func()) {
	testClient(t, requestCtxFn, server, executor, expectedAttempts, expectedExecutions, "", expectedError, false, then...)
}

func testClientFailureResult[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult any, then ...func()) {
	testClient(t, requestCtxFn, server, executor, expectedAttempts, expectedExecutions, expectedResult, nil, false, then...)
}

func testClient[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult any, expectedError error, expectedSuccess bool, thens ...func()) {
	t.Helper()

	// Given
	executorFn, assertResult := testutil.PrepareTest(t, nil, nil, executor)
	grpcServer, dialer := testutil.GrpcServer(server)
	grpcClient := testutil.GrpcClient(dialer, grpc.WithUnaryInterceptor(NewUnaryClientInterceptorWithExecutor(executorFn())))
	t.Cleanup(func() {
		grpcServer.Stop()
		grpcClient.Close()
	})
	client := pbfixtures.NewPingServiceClient(grpcClient)
	ctx := context.Background()
	if requestCtxFn != nil {
		ctx = requestCtxFn()
	}

	// When
	response, err := client.Ping(ctx, &pbfixtures.PingRequest{Msg: "ping"})
	var msg string
	if response != nil {
		msg = response.Msg
	}
	assert.Equal(t, expectedResult, msg)

	// Then
	var nilR R
	assertResult(expectedAttempts, expectedExecutions, nilR, nilR, expectedError, err, expectedSuccess, !expectedSuccess, false, thens...)
}
