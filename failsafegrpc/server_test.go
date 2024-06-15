package failsafegrpc

import (
	"context"
	"errors"
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/bulkhead"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/ratelimiter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/tap"
	"testing"
	"time"
)

func TestUnaryServerInterceptorSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryServerResponse](
		bulkhead.Builder[*UnaryServerResponse](1).
			WithMaxWaitTime(1 * time.Millisecond).
			Build(),
	)
	handler := &testutil.MockUnaryHandler{}
	handler.Test(t)
	handler.On("Handle", context.Background(), &testutil.MockUnaryHandleRequest{}).
		Times(1).
		Return(&testutil.MockUnaryHandleResponse{
			Message: "success",
		}, nil)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockUnaryHandleRequest{}
	testUnaryServer(
		t,
		executor,
		ctx,
		req,
		handler.Handle,
		UnaryServerInterceptor(executor),
		1,
		1,
		&UnaryServerResponse{
			Ctx:  ctx,
			Req:  req,
			Resp: &testutil.MockUnaryHandleResponse{Message: "success"},
		},
		nil,
		true,
	)
}

func TestUnaryServerInterceptorError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryServerResponse](
		bulkhead.Builder[*UnaryServerResponse](0).
			WithMaxWaitTime(1 * time.Millisecond).
			Build(),
	)
	handler := &testutil.MockUnaryHandler{}
	handler.Test(t)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockUnaryHandleRequest{}
	interceptor := UnaryServerInterceptor(executor)
	testUnaryServer(
		t,
		executor,
		ctx,
		req,
		handler.Handle,
		interceptor,
		1,
		0,
		&UnaryServerResponse{
			Ctx:  ctx,
			Req:  req,
			Resp: nil,
		},
		bulkhead.ErrFull,
		false,
	)
}

func TestUnaryServerInterceptorWithCircuitBreaker(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[*UnaryServerResponse]()
	executor := failsafe.NewExecutor[*UnaryServerResponse](
		cb,
	)
	cb.Open()
	invoker := &testutil.MockUnaryHandler{}
	invoker.Test(t)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockUnaryHandleRequest{}
	interceptor := UnaryServerInterceptor(executor)
	testUnaryServer(
		t,
		executor,
		ctx,
		req,
		invoker.Handle,
		interceptor,
		1,
		0,
		&UnaryServerResponse{
			Ctx:  ctx,
			Req:  req,
			Resp: nil,
		},
		circuitbreaker.ErrOpen,
		false,
	)
}

func TestUnaryServerInterceptorWithCircuitBreakerOnResult(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder[*UnaryServerResponse]().
		HandleIf(func(resp *UnaryServerResponse, err error) bool {
			result, ok := resp.Resp.(*testutil.MockUnaryHandleResponse)
			return ok && result.Message == "error"
		}).
		Build()
	executor := failsafe.NewExecutor[*UnaryServerResponse](
		cb,
	)
	invoker := &testutil.MockUnaryHandler{}
	invoker.Test(t)
	invoker.On("Handle", context.Background(), &testutil.MockUnaryHandleRequest{}).
		Times(1).
		Return(&testutil.MockUnaryHandleResponse{
			Message: "error",
		}, nil)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockUnaryHandleRequest{}
	interceptor := UnaryServerInterceptor(executor)
	testUnaryServer(
		t,
		executor,
		ctx,
		req,
		invoker.Handle,
		interceptor,
		1,
		1,
		&UnaryServerResponse{
			Ctx:  ctx,
			Req:  req,
			Resp: &testutil.MockUnaryHandleResponse{Message: "error"},
		},
		nil,
		false,
	)
	assert.True(t, cb.IsOpen())
}

func TestUnaryServerInterceptorWithRateLimiter(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderWithMaxRate[*UnaryServerResponse](1 * time.Hour).
		Build()
	limiter.TryAcquirePermit()
	executor := failsafe.NewExecutor[*UnaryServerResponse](limiter)
	handler := &testutil.MockUnaryHandler{}
	handler.Test(t)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockUnaryHandleRequest{}
	interceptor := UnaryServerInterceptor(executor)
	testUnaryServer(
		t,
		executor,
		ctx,
		req,
		handler.Handle,
		interceptor,
		1,
		0,
		&UnaryServerResponse{
			Ctx:  ctx,
			Req:  req,
			Resp: nil,
		},
		ratelimiter.ErrExceeded,
		false,
	)
}

func TestStreamServerInterceptorSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamServerResponse](
		bulkhead.Builder[*StreamServerResponse](1).
			WithMaxWaitTime(1 * time.Millisecond).
			Build(),
	)
	handler := &testutil.MockStreamHandler{}
	handler.Test(t)
	handler.On("Handle", mock.Anything, &testutil.MockServerStream{}).
		Times(1).
		Return(nil)

	// When / Then
	interceptor := StreamServerInterceptor(executor)
	testStreamServer(
		t,
		executor,
		nil,
		&testutil.MockServerStream{},
		&grpc.StreamServerInfo{},
		handler.Handle,
		interceptor,
		1,
		1,
		nil,
		nil,
		true,
	)
}

func TestStreamServerInterceptorError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamServerResponse](
		bulkhead.Builder[*StreamServerResponse](0).
			WithMaxWaitTime(1 * time.Millisecond).
			Build(),
	)
	handler := &testutil.MockStreamHandler{}
	handler.Test(t)

	// When / Then
	interceptor := StreamServerInterceptor(executor)
	testStreamServer(
		t,
		executor,
		nil,
		&testutil.MockServerStream{},
		&grpc.StreamServerInfo{},
		handler.Handle,
		interceptor,
		1,
		0,
		nil,
		bulkhead.ErrFull,
		false,
	)
}

func TestStreamServerInterceptorWithCircuitBreaker(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[*StreamServerResponse]()
	executor := failsafe.NewExecutor[*StreamServerResponse](
		cb,
	)
	cb.Open()
	handler := &testutil.MockStreamHandler{}
	handler.Test(t)

	// When / Then
	interceptor := StreamServerInterceptor(executor)
	testStreamServer(
		t,
		executor,
		nil,
		&testutil.MockServerStream{},
		&grpc.StreamServerInfo{},
		handler.Handle,
		interceptor,
		1,
		0,
		nil,
		circuitbreaker.ErrOpen,
		false,
	)
}

func TestStreamServerInterceptorWithCircuitBreakerOnResult(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder[*StreamServerResponse]().
		HandleIf(func(resp *StreamServerResponse, err error) bool {
			stream := resp.Stream

			buffer := make([]byte, 1024)
			err = stream.RecvMsg(buffer)
			return err != nil
		}).
		Build()
	executor := failsafe.NewExecutor[*StreamServerResponse](
		cb,
	)
	mockError := errors.New("error")
	handler := &testutil.MockStreamHandler{}
	handler.Test(t)
	handler.On("Handle", mock.Anything, &testutil.MockServerStream{ExpectedError: mockError}).
		Times(1).
		Return(nil)

	// When / Then
	interceptor := StreamServerInterceptor(executor)
	testStreamServer(
		t,
		executor,
		nil,
		&testutil.MockServerStream{
			ExpectedError: mockError,
		},
		&grpc.StreamServerInfo{},
		handler.Handle,
		interceptor,
		1,
		1,
		nil,
		nil,
		false,
	)
}

func TestInHandleAfterHookSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*InHandleResult](
		bulkhead.Builder[*InHandleResult](1).
			WithMaxWaitTime(1 * time.Millisecond).
			Build(),
	)
	mockHandle := &testutil.MockServerInHandle{}
	mockHandle.Test(t)
	mockHandle.On("Handle", mock.Anything, &tap.Info{}).
		Times(1).
		Return(context.Background(), nil)

	// When / Then
	ctx := context.Background()
	info := &tap.Info{}
	testServerInHandle(
		t,
		executor,
		ctx,
		info,
		mockHandle.Handle,
		1,
		1,
		&InHandleResult{
			Ctx:  ctx,
			Info: info,
		},
		nil,
		true,
	)
}

func TestInHandleAfterHookError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*InHandleResult](
		bulkhead.Builder[*InHandleResult](0).
			WithMaxWaitTime(1 * time.Millisecond).
			Build(),
	)
	mockHandle := &testutil.MockServerInHandle{}
	mockHandle.Test(t)

	// When / Then
	ctx := context.Background()
	info := &tap.Info{}
	testServerInHandle(
		t,
		executor,
		ctx,
		info,
		mockHandle.Handle,
		1,
		0,
		&InHandleResult{
			Ctx:  nil,
			Info: info,
		},
		bulkhead.ErrFull,
		false,
	)
}

func TestInHandleAfterHookWithCircuitBreaker(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[*InHandleResult]()
	executor := failsafe.NewExecutor[*InHandleResult](
		cb,
	)
	cb.Open()
	mockHandle := &testutil.MockServerInHandle{}
	mockHandle.Test(t)

	// When / Then
	ctx := context.Background()
	info := &tap.Info{}
	testServerInHandle(
		t,
		executor,
		ctx,
		info,
		mockHandle.Handle,
		1,
		0,
		&InHandleResult{
			Ctx:  nil,
			Info: info,
		},
		circuitbreaker.ErrOpen,
		false,
	)
}

func TestInHandleAfterHookWithCircuitBreakerOnResult(t *testing.T) {
	// Given
	cb := circuitbreaker.Builder[*InHandleResult]().
		HandleIf(func(result *InHandleResult, err error) bool {
			return result.Info != nil
		}).
		Build()
	executor := failsafe.NewExecutor[*InHandleResult](
		cb,
	)
	mockHandle := &testutil.MockServerInHandle{}
	mockHandle.Test(t)
	mockHandle.On("Handle", mock.Anything, &tap.Info{}).
		Times(1).
		Return(context.Background(), nil)

	// When / Then
	ctx := context.Background()
	info := &tap.Info{}
	testServerInHandle(
		t,
		executor,
		ctx,
		info,
		mockHandle.Handle,
		1,
		1,
		&InHandleResult{
			Ctx:  ctx,
			Info: info,
		},
		nil,
		false,
	)
	assert.True(t, cb.IsOpen())
}

func TestInHandleAfterHookWithRateLimiter(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderWithMaxRate[*InHandleResult](1 * time.Hour).
		Build()
	limiter.TryAcquirePermit()
	executor := failsafe.NewExecutor[*InHandleResult](limiter)
	mockHandle := &testutil.MockServerInHandle{}
	mockHandle.Test(t)

	// When / Then
	ctx := context.Background()
	info := &tap.Info{}
	testServerInHandle(
		t,
		executor,
		ctx,
		info,
		mockHandle.Handle,
		1,
		0,
		&InHandleResult{
			Ctx:  nil,
			Info: info,
		},
		ratelimiter.ErrExceeded,
		false,
	)
}

func testUnaryServer(
	t *testing.T,
	executor failsafe.Executor[*UnaryServerResponse],
	ctx context.Context,
	req any,
	handler grpc.UnaryHandler,
	interceptor grpc.UnaryServerInterceptor,
	expectedAttempts int,
	expectedExecutions int,
	expectedResult *UnaryServerResponse,
	expectedError error,
	expectedSuccess bool,
	thens ...func(),
) {
	var then func()
	if len(thens) > 0 {
		then = thens[0]
	}
	var expectedErrPtr *error
	expectedErrPtr = &expectedError
	executorFn, assertResult := testutil.PrepareTest(t, nil, executor, expectedAttempts, expectedExecutions, expectedResult, expectedErrPtr, expectedSuccess, !expectedSuccess, then)
	executorFn()

	// When
	resp, err := interceptor(ctx, req, nil, handler)

	// Then
	assertResult(
		&UnaryServerResponse{
			Ctx:  ctx,
			Req:  req,
			Resp: resp,
		},
		err,
	)
}

func TestStreamServerInterceptorWithRateLimiter(t *testing.T) {
	// Given
	limiter := ratelimiter.SmoothBuilderWithMaxRate[*StreamServerResponse](1 * time.Hour).
		Build()
	limiter.TryAcquirePermit()
	executor := failsafe.NewExecutor[*StreamServerResponse](limiter)
	handler := &testutil.MockStreamHandler{}
	handler.Test(t)

	// When / Then
	interceptor := StreamServerInterceptor(executor)
	testStreamServer(
		t,
		executor,
		nil,
		&testutil.MockServerStream{},
		&grpc.StreamServerInfo{},
		handler.Handle,
		interceptor,
		1,
		0,
		nil,
		ratelimiter.ErrExceeded,
		false,
	)
}

func testStreamServer(
	t *testing.T,
	executor failsafe.Executor[*StreamServerResponse],
	srv interface{},
	stream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
	interceptor grpc.StreamServerInterceptor,
	expectedAttempts int,
	expectedExecutions int,
	expectedResult *StreamServerResponse,
	expectedError error,
	expectedSuccess bool,
	thens ...func(),
) {
	var then func()
	if len(thens) > 0 {
		then = thens[0]
	}
	var expectedErrPtr *error
	expectedErrPtr = &expectedError
	executorFn, assertResult := testutil.PrepareTest(t, nil, executor, expectedAttempts, expectedExecutions, expectedResult, expectedErrPtr, expectedSuccess, !expectedSuccess, then)
	executorFn()

	// When
	err := interceptor(srv, stream, info, handler)

	// Then
	assertResult(nil, err)
}

func testServerInHandle(
	t *testing.T,
	executor failsafe.Executor[*InHandleResult],
	ctx context.Context,
	info *tap.Info,
	handler tap.ServerInHandle,
	expectedAttempts int,
	expectedExecutions int,
	expectedResult *InHandleResult,
	expectedError error,
	expectedSuccess bool,
	thens ...func(),
) {
	var then func()
	if len(thens) > 0 {
		then = thens[0]
	}
	var expectedErrPtr *error
	expectedErrPtr = &expectedError
	executorFn, assertResult := testutil.PrepareTest(t, nil, executor, expectedAttempts, expectedExecutions, expectedResult, expectedErrPtr, expectedSuccess, !expectedSuccess, then)
	executorFn()

	// When
	result, err := InHandleAfterHook(executor, handler)(ctx, info)

	// Then
	assertResult(
		&InHandleResult{
			Ctx:  result,
			Info: info,
		},
		err,
	)
}
