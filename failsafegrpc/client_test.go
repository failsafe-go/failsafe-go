package failsafegrpc

import (
	"bytes"
	"context"
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/fallback"
	"github.com/failsafe-go/failsafe-go/internal/testutil"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/failsafe-go/failsafe-go/timeout"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

func TestUnaryClientInterceptorSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		UnaryCallRetryPolicyBuilder().Build(),
	)
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Times(1).
		Return(&testutil.MockInvokeResponse{
			Message: "Success",
		}, nil)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		1,
		1,
		&testutil.MockInvokeResponse{
			Message: "Success",
		},
		nil,
		true,
	)
}

func TestUnaryClientInterceptorError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		retrypolicy.Builder[any]().ReturnLastFailure().Build(),
	)
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)
	mockError := status.Error(codes.Unknown, "mock error")
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Times(3).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		3,
		3,
		nil,
		mockError,
		false,
	)
}

func TestUnaryClientInterceptorWithRetry(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		retrypolicy.Builder[any]().
			HandleIf(func(reply any, err error) bool {
				resp, ok := reply.(*testutil.MockInvokeResponse)
				if !ok {
					return false
				}

				return resp.Message == "Retry"
			}).
			ReturnLastFailure().
			Build(),
	)
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Times(1).
		Return(&testutil.MockInvokeResponse{
			Message: "Retry",
		}, nil)
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{Message: "Retry"}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Times(1).
		Return(&testutil.MockInvokeResponse{
			Message: "Success",
		}, nil)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		2,
		2,
		&testutil.MockInvokeResponse{
			Message: "Success",
		},
		nil,
		true,
	)
}

func TestUnaryClientInterceptorWithFallback(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		fallback.BuilderWithFunc[any](func(exec failsafe.Execution[any]) (any, error) {
			return &testutil.MockInvokeResponse{
				Message: "Fallback",
			}, nil
		}).HandleIf(func(reply any, err error) bool {
			s, ok := status.FromError(err)
			if !ok {
				return false
			}

			return s.Code() == codes.Unknown
		}).Build(),
		retrypolicy.Builder[any]().
			ReturnLastFailure().
			Build(),
	)
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)
	mockError := status.Error(codes.Unknown, "mock error")
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Times(3).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		3,
		3,
		&testutil.MockInvokeResponse{
			Message: "Fallback",
		},
		nil,
		true,
	)
}

func TestUnaryClientInterceptorWithCircuitBreaker(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[any]()
	executor := failsafe.NewExecutor[any](
		cb,
	)
	cb.Open()
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		1,
		0,
		nil,
		circuitbreaker.ErrOpen,
		false,
	)
}

func TestUnaryClientInterceptorWithContextTimeout(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		retrypolicy.Builder[any]().ReturnLastFailure().Build(),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	invoker := &testutil.MockInvoker{
		Sleep: 100 * time.Millisecond,
	}
	invoker.Test(t)
	invoker.On("Invoke", ctx, "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Return(&testutil.MockInvokeResponse{
			Message: "Success",
		}, nil)

	// When / Then
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		1,
		1,
		nil,
		context.DeadlineExceeded,
		false,
	)
}

func TestUnaryClientInterceptorWithTimeout(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		timeout.With[any](50 * time.Millisecond),
	)
	invoker := &testutil.MockInvoker{
		Sleep: 100 * time.Millisecond,
	}
	invoker.Test(t)
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Return(&testutil.MockInvokeResponse{
			Message: "Success",
		}, nil)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		1,
		1,
		nil,
		timeout.ErrExceeded,
		false,
	)

}

func TestUnaryCallRetryPolicyWithRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		UnaryCallRetryPolicyBuilder().
			ReturnLastFailure().
			Build(),
	)
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)
	mockError := status.Error(codes.Unavailable, "mock error")
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Times(3).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		3,
		3,
		nil,
		mockError,
		false,
	)
}

func TestUnaryCallRetryPolicyWithRetryableErrorAndSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		UnaryCallRetryPolicyBuilder().
			Build(),
	)
	invoker := &testutil.MockRetryableInvoker{
		AllowedRetries: 1,
		RetryErrorCode: codes.Unavailable,
	}
	invoker.Test(t)
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Times(2).
		Return(&testutil.MockInvokeResponse{
			Message: "Success",
		}, nil)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		2,
		2,
		&testutil.MockInvokeResponse{
			Message: "Success",
		},
		nil,
		true,
	)
}

func TestUnaryCallRetryPolicyWithNonRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[any](
		UnaryCallRetryPolicyBuilder().
			ReturnLastFailure().
			Build(),
	)
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)
	mockError := status.Error(codes.InvalidArgument, "mock error")
	invoker.On("Invoke", context.Background(), "method", &testutil.MockInvokeRequest{}, &testutil.MockInvokeResponse{}, (*grpc.ClientConn)(nil), []grpc.CallOption(nil)).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnary(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		1,
		1,
		nil,
		mockError,
		true,
	)
}

func TestStreamClientInterceptorSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		StreamCallRetryPolicyBuilder().Build(),
	)
	streamer := &testutil.MockClientStreamer{}
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Times(1).
		Return(&testutil.MockClientStream{}, nil)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		1,
		1,
		&testutil.MockClientStream{},
		nil,
		true,
	)
}

func TestStreamClientInterceptorError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		retrypolicy.Builder[grpc.ClientStream]().ReturnLastFailure().Build(),
	)
	streamer := &testutil.MockClientStreamer{}
	mockError := status.Error(codes.Unknown, "mock error")
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Times(3).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		3,
		3,
		nil,
		mockError,
		false,
	)
}

func TestStreamClientInterceptorWithRetry(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		retrypolicy.Builder[grpc.ClientStream]().
			HandleIf(func(stream grpc.ClientStream, err error) bool {
				s, ok := stream.(*testutil.MockClientStream)
				if !ok {
					return false
				}

				buffer := &bytes.Buffer{}
				err = s.RecvMsg(buffer)
				if err != nil {
					return true
				}
				return false
			}).
			ReturnLastFailure().
			Build(),
	)
	streamer := &testutil.MockClientStreamer{}
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Times(2).
		Return(&testutil.MockClientStream{
			AllowedRetries: 1,
			RetryErrorCode: codes.Unavailable,
		}, nil)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		2,
		2,
		&testutil.MockClientStream{},
		nil,
		true,
	)
}

func TestStreamClientInterceptorWithFallback(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		fallback.BuilderWithFunc[grpc.ClientStream](func(exec failsafe.Execution[grpc.ClientStream]) (grpc.ClientStream, error) {
			return &testutil.MockClientStream{}, nil
		}).HandleIf(func(stream grpc.ClientStream, err error) bool {
			s, ok := status.FromError(err)
			if !ok {
				return false
			}

			return s.Code() == codes.Unknown
		}).Build(),
		retrypolicy.Builder[grpc.ClientStream]().
			ReturnLastFailure().
			Build(),
	)
	streamer := &testutil.MockClientStreamer{}
	mockError := status.Error(codes.Unknown, "mock error")
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Times(3).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		3,
		3,
		&testutil.MockClientStream{},
		nil,
		true,
	)
}

func TestStreamClientInterceptorWithCircuitBreaker(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[grpc.ClientStream]()
	executor := failsafe.NewExecutor[grpc.ClientStream](
		cb,
	)
	cb.Open()
	streamer := &testutil.MockClientStreamer{}

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		1,
		0,
		nil,
		circuitbreaker.ErrOpen,
		false,
	)
}

func TestStreamClientInterceptorWithContextTimeout(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		retrypolicy.Builder[grpc.ClientStream]().ReturnLastFailure().Build(),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	streamer := &testutil.MockClientStreamer{
		Sleep: 100 * time.Millisecond,
	}
	streamer.On("Stream", ctx, &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Return(&testutil.MockClientStream{}, nil)

	// When / Then
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		1,
		1,
		nil,
		context.DeadlineExceeded,
		false,
	)
}

func TestStreamClientInterceptorWithTimeout(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		timeout.With[grpc.ClientStream](50 * time.Millisecond),
	)
	streamer := &testutil.MockClientStreamer{
		Sleep: 100 * time.Millisecond,
	}
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Return(&testutil.MockClientStream{}, nil)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		1,
		1,
		nil,
		timeout.ErrExceeded,
		false,
	)
}

func TestStreamCallRetryPolicyWithRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		StreamCallRetryPolicyBuilder().
			ReturnLastFailure().
			Build(),
	)
	streamer := &testutil.MockClientStreamer{}
	mockError := status.Error(codes.Unavailable, "mock error")
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Times(3).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		3,
		3,
		nil,
		mockError,
		false,
	)
}

func TestStreamCallRetryPolicyWithRetryableErrorAndSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		StreamCallRetryPolicyBuilder().
			Build(),
	)
	streamer := &testutil.MockRetryableClientStreamer{
		AllowedRetries: 1,
		RetryErrorCode: codes.Unavailable,
	}
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Times(2).
		Return(&testutil.MockClientStream{}, nil)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		2,
		2,
		&testutil.MockClientStream{},
		nil,
		true,
	)
}

func TestStreamCallRetryPolicyWithNonRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[grpc.ClientStream](
		StreamCallRetryPolicyBuilder().
			ReturnLastFailure().
			Build(),
	)
	streamer := &testutil.MockClientStreamer{}
	mockError := status.Error(codes.InvalidArgument, "mock error")
	streamer.On("Stream", context.Background(), &grpc.StreamDesc{}, &grpc.ClientConn{}, "method", []grpc.CallOption(nil)).
		Return(nil, mockError)

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStream(
		t,
		executor,
		ctx,
		desc,
		cc,
		method,
		streamer.Stream,
		nil,
		StreamClientInterceptor(executor),
		1,
		1,
		nil,
		mockError,
		true,
	)
}

func testUnary(
	t *testing.T,
	executor failsafe.Executor[any],
	ctx context.Context,
	req any,
	reply any,
	invoker grpc.UnaryInvoker,
	interceptor grpc.UnaryClientInterceptor,
	expectedAttempts int,
	expectedExecutions int,
	expectedResult any,
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
	err := interceptor(ctx, "method", req, reply, nil, invoker)

	// Then
	assertResult(expectedResult, err)
}

func testStream(
	t *testing.T,
	executor failsafe.Executor[grpc.ClientStream],
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts []grpc.CallOption,
	interceptor grpc.StreamClientInterceptor,
	expectedAttempts int,
	expectedExecutions int,
	expectedResult grpc.ClientStream,
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
	_, err := interceptor(ctx, desc, cc, method, streamer, opts...)

	// Then
	assertResult(expectedResult, err)
}
