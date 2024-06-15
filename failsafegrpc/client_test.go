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
	executor := failsafe.NewExecutor[*UnaryClientResponse](
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
	testUnaryClient(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		1,
		1,
		&UnaryClientResponse{
			Ctx:    ctx,
			Req:    req,
			Method: "method",
			Reply: &testutil.MockInvokeResponse{
				Message: "Success",
			},
		},
		nil,
		true,
	)
}

func TestUnaryClientInterceptorError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryClientResponse](
		retrypolicy.Builder[*UnaryClientResponse]().ReturnLastFailure().Build(),
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
	testUnaryClient(
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

func TestUnaryClientInterceptorWithRetryOnResult(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryClientResponse](
		retrypolicy.Builder[*UnaryClientResponse]().
			HandleIf(func(resp *UnaryClientResponse, err error) bool {
				mockResp, ok := resp.Reply.(*testutil.MockInvokeResponse)
				if !ok {
					return false
				}

				return mockResp.Message == "Retry"
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
	testUnaryClient(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		2,
		2,
		&UnaryClientResponse{
			Ctx:    ctx,
			Req:    req,
			Method: "method",
			Reply: &testutil.MockInvokeResponse{
				Message: "Success",
			},
		},
		nil,
		true,
	)
}

func TestUnaryClientInterceptorWithFallback(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryClientResponse](
		fallback.BuilderWithFunc[*UnaryClientResponse](func(exec failsafe.Execution[*UnaryClientResponse]) (*UnaryClientResponse, error) {
			result := exec.LastResult()

			return &UnaryClientResponse{
				Ctx: result.Ctx,
				Req: result.Req,
				Reply: &testutil.MockInvokeResponse{
					Message: "Fallback",
				},
			}, nil
		}).HandleIf(func(resp *UnaryClientResponse, err error) bool {
			s, ok := status.FromError(err)
			if !ok {
				return false
			}

			return s.Code() == codes.Unknown
		}).Build(),
		retrypolicy.Builder[*UnaryClientResponse]().
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
	testUnaryClient(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		3,
		3,
		&UnaryClientResponse{
			Ctx:    ctx,
			Req:    req,
			Method: "method",
			Reply: &testutil.MockInvokeResponse{
				Message: "Fallback",
			},
		},
		nil,
		true,
	)
}

func TestUnaryClientInterceptorWithCircuitBreaker(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[*UnaryClientResponse]()
	executor := failsafe.NewExecutor[*UnaryClientResponse](
		cb,
	)
	cb.Open()
	invoker := &testutil.MockInvoker{}
	invoker.Test(t)

	// When / Then
	ctx := context.Background()
	req := &testutil.MockInvokeRequest{}
	reply := &testutil.MockInvokeResponse{}
	testUnaryClient(
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
	executor := failsafe.NewExecutor[*UnaryClientResponse](
		retrypolicy.Builder[*UnaryClientResponse]().ReturnLastFailure().Build(),
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
	testUnaryClient(
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
	executor := failsafe.NewExecutor[*UnaryClientResponse](
		timeout.With[*UnaryClientResponse](50 * time.Millisecond),
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
	testUnaryClient(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		1,
		1,
		&UnaryClientResponse{
			Ctx:    ctx,
			Req:    req,
			Method: "method",
			Reply: &testutil.MockInvokeResponse{
				Message: "Success",
			},
		},
		timeout.ErrExceeded,
		false,
	)

}

func TestUnaryCallRetryPolicyWithRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryClientResponse](
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
	testUnaryClient(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		3,
		3,
		&UnaryClientResponse{
			Ctx:    ctx,
			Req:    req,
			Method: "method",
			Reply:  nil,
		},
		mockError,
		false,
	)
}

func TestUnaryCallRetryPolicyWithRetryableErrorAndSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryClientResponse](
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
	testUnaryClient(
		t,
		executor,
		ctx,
		req,
		reply,
		invoker.Invoke,
		UnaryClientInterceptor(executor),
		2,
		2,
		&UnaryClientResponse{
			Ctx:    ctx,
			Req:    req,
			Method: "method",
			Reply: &testutil.MockInvokeResponse{
				Message: "Success",
			},
		},
		nil,
		true,
	)
}

func TestUnaryCallRetryPolicyWithNonRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*UnaryClientResponse](
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
	testUnaryClient(
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
	executor := failsafe.NewExecutor[*StreamClientResponse](
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: &testutil.MockClientStream{},
		},
		nil,
		true,
	)
}

func TestStreamClientInterceptorError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamClientResponse](
		retrypolicy.Builder[*StreamClientResponse]().ReturnLastFailure().Build(),
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: nil,
		},
		mockError,
		false,
	)
}

func TestStreamClientInterceptorWithRetryOnResult(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamClientResponse](
		retrypolicy.Builder[*StreamClientResponse]().
			HandleIf(func(resp *StreamClientResponse, err error) bool {
				stream := resp.Stream

				buffer := &bytes.Buffer{}
				err = stream.RecvMsg(buffer)
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: &testutil.MockClientStream{},
		},
		nil,
		true,
	)
}

func TestStreamClientInterceptorWithFallback(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamClientResponse](
		fallback.BuilderWithFunc[*StreamClientResponse](func(exec failsafe.Execution[*StreamClientResponse]) (*StreamClientResponse, error) {
			result := exec.LastResult()
			return &StreamClientResponse{
				Ctx:    result.Ctx,
				Desc:   result.Desc,
				Method: result.Method,
				Stream: &testutil.MockClientStream{},
			}, nil
		}).HandleIf(func(resp *StreamClientResponse, err error) bool {
			s, ok := status.FromError(err)
			if !ok {
				return false
			}

			return s.Code() == codes.Unknown
		}).Build(),
		retrypolicy.Builder[*StreamClientResponse]().
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: nil,
		},
		nil,
		true,
	)
}

func TestStreamClientInterceptorWithCircuitBreaker(t *testing.T) {
	// Given
	cb := circuitbreaker.WithDefaults[*StreamClientResponse]()
	executor := failsafe.NewExecutor[*StreamClientResponse](
		cb,
	)
	cb.Open()
	streamer := &testutil.MockClientStreamer{}

	// When / Then
	ctx := context.Background()
	desc := &grpc.StreamDesc{}
	cc := &grpc.ClientConn{}
	method := "method"
	testStreamClient(
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
	executor := failsafe.NewExecutor[*StreamClientResponse](
		retrypolicy.Builder[*StreamClientResponse]().ReturnLastFailure().Build(),
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: &testutil.MockClientStream{},
		},
		context.DeadlineExceeded,
		false,
	)
}

func TestStreamClientInterceptorWithTimeout(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamClientResponse](
		timeout.With[*StreamClientResponse](50 * time.Millisecond),
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: &testutil.MockClientStream{},
		},
		timeout.ErrExceeded,
		false,
	)
}

func TestStreamCallRetryPolicyWithRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamClientResponse](
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: nil,
		},
		mockError,
		false,
	)
}

func TestStreamCallRetryPolicyWithRetryableErrorAndSuccess(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamClientResponse](
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
	testStreamClient(
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
		&StreamClientResponse{
			Ctx:    ctx,
			Desc:   desc,
			Method: method,
			Stream: &testutil.MockClientStream{},
		},
		nil,
		true,
	)
}

func TestStreamCallRetryPolicyWithNonRetryableError(t *testing.T) {
	// Given
	executor := failsafe.NewExecutor[*StreamClientResponse](
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
	testStreamClient(
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

func testUnaryClient(
	t *testing.T,
	executor failsafe.Executor[*UnaryClientResponse],
	ctx context.Context,
	req any,
	reply any,
	invoker grpc.UnaryInvoker,
	interceptor grpc.UnaryClientInterceptor,
	expectedAttempts int,
	expectedExecutions int,
	expectedResult *UnaryClientResponse,
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

func testStreamClient(
	t *testing.T,
	executor failsafe.Executor[*StreamClientResponse],
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts []grpc.CallOption,
	interceptor grpc.StreamClientInterceptor,
	expectedAttempts int,
	expectedExecutions int,
	expectedResult *StreamClientResponse,
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
