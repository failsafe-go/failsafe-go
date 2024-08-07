package failsafegrpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/cachepolicy"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/policytesting"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/internal/testutil/pbfixtures"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

func TestServerSuccess(t *testing.T) {
	// Given
	mockedResponse := "pong"
	server := testutil.MockGrpcResponses(mockedResponse)
	executor := failsafe.NewExecutor[any](RetryPolicyBuilder[any]().Build())

	// When / Then
	testServerSuccess(t, nil, server, executor,
		1, 1, mockedResponse, true)
}

func TestServerFallback(t *testing.T) {
	// Given
	server := testutil.MockGrpcError(errors.New("err"))
	fb := fallback.WithResult(&pbfixtures.PingResponse{Msg: "pong"})
	executor := failsafe.NewExecutor[*pbfixtures.PingResponse](fb)

	// When / Then
	testServerSuccess(t, nil, server, executor,
		1, 1, "pong", false)
}

func TestServerCache(t *testing.T) {
	// Given
	server := testutil.MockGrpcResponses("foo")
	cache, failsafeCache := policytesting.NewCache[any]()
	cache["foo"] = &pbfixtures.PingResponse{Msg: "bar"}
	cp := cachepolicy.Builder[any](failsafeCache).WithKey("foo").Build()
	executor := failsafe.NewExecutor[any](cp)

	// When / Then
	testServerSuccess(t, nil, server, executor,
		1, 0, "bar", false)
}

// Asserts that providing a context to either the executor or a request that is canceled results in the execution being canceled.
func TestServerCancelWithContext(t *testing.T) {
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
			"with canceling executor context and slow request context",
			slowCtxFn,
			fastCtxFn(),
		},
		// We don't include a test case for a request context here since a canceled request context may cause the client to return a result
		// before the Interceptor is finished handling it, so the expected listeners don't record results in time.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given
			server := testutil.MockDelayedGrpcResponse("pong", time.Second)
			rp := retrypolicy.Builder[any]().AbortOnErrors(context.Canceled).Build()
			executor := failsafe.NewExecutor[any](rp)
			if tc.executorCtx != nil {
				executor = executor.WithContext(tc.executorCtx)
			}

			// When / Then
			start := time.Now()
			testServerFailure(t, tc.requestCtxFn, server, executor,
				1, 1, context.Canceled, true)
			assert.True(t, start.Add(time.Second).After(time.Now()), "cancellation should immediately exit execution")
		})
	}
}

func testServerSuccess[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult any, testServerInHandle bool, then ...func()) {
	testServer(t, requestCtxFn, server, executor, expectedAttempts, expectedExecutions, expectedResult, nil, true, testServerInHandle, then...)
}

func testServerFailure[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedError error, testServerInHandle bool, then ...func()) {
	testServer(t, requestCtxFn, server, executor, expectedAttempts, expectedExecutions, "", expectedError, false, testServerInHandle, then...)
}

func testServerFailureResult[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult any, testServerInHandle bool, then ...func()) {
	testServer(t, requestCtxFn, server, executor, expectedAttempts, expectedExecutions, expectedResult, nil, false, testServerInHandle, then...)
}

func testServer[R any](t *testing.T, requestCtxFn func() context.Context, server pbfixtures.PingServiceServer, executor failsafe.Executor[R], expectedAttempts int, expectedExecutions int, expectedResult any, expectedError error, expectedSuccess bool, testServerInHandle bool, thens ...func()) {
	t.Helper()

	// Given
	executorFn, assertResult := testutil.PrepareTest(t, nil, nil, executor)
	testGrpc := func(option grpc.ServerOption) {
		grpcServer, dialer := testutil.GrpcServer(server, option)
		grpcClient := testutil.GrpcClient(dialer)
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
		// Assert msg
		var msg string
		if response != nil {
			msg = response.Msg
		}
		assert.Equal(t, expectedResult, msg)
		// Assert error msg
		if expectedError != nil {
			if stat, ok := status.FromError(err); ok {
				assert.Equal(t, expectedError.Error(), stat.Message(), "expected error did not match")
			} else {
				assert.ErrorIs(t, err, expectedError, "expected error did not match")
			}
		}

		var nilR R
		assertResult(expectedAttempts, expectedExecutions, nilR, nilR, nil, nil, expectedSuccess, !expectedSuccess, false, thens...)
	}

	// Then
	fmt.Println("Testing NewUnaryServerInterceptorWithExecutor")
	testGrpc(grpc.UnaryInterceptor(NewUnaryServerInterceptorWithExecutor(executorFn())))

	if testServerInHandle {
		fmt.Println("Testing NewServerInHandleWithExecutor")
		testGrpc(grpc.InTapHandle(NewServerInHandleWithExecutor(executorFn())))
	}
}
