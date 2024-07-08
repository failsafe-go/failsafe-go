package failsafegrpc

import (
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
