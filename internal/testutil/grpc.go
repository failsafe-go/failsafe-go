package testutil

import (
	"context"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/tap"
	"time"
)

type MockInvoker struct {
	mock.Mock

	Sleep time.Duration
}

type MockInvokeRequest struct{}

type MockInvokeResponse struct {
	Message string
}

func (m *MockInvoker) Invoke(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
	args := m.Called(ctx, method, req, reply, cc, opts)

	time.Sleep(m.Sleep)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if args.Error(1) != nil {
		return args.Error(1)
	}

	result := args.Get(0).(*MockInvokeResponse)
	*reply.(*MockInvokeResponse) = *result

	return nil
}

type MockRetryableInvoker struct {
	mock.Mock

	AllowedRetries int
	RetryErrorCode codes.Code

	retryCount int
}

func (m *MockRetryableInvoker) Invoke(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
	args := m.Called(ctx, method, req, reply, cc, opts)

	if m.retryCount < m.AllowedRetries {
		m.retryCount++
		return status.Error(m.RetryErrorCode, "retry request")
	}

	if args.Error(1) != nil {
		return args.Error(1)
	}

	result := args.Get(0).(*MockInvokeResponse)
	*reply.(*MockInvokeResponse) = *result

	return nil
}

type MockUnaryHandler struct {
	mock.Mock
}

func (h *MockUnaryHandler) Handle(ctx context.Context, req any) (any, error) {
	args := h.Called(ctx, req)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	return args.Get(0), nil
}

type MockUnaryHandleRequest struct{}

type MockUnaryHandleResponse struct {
	Message string
}

type MockServerInHandle struct {
	mock.Mock
}

func (h *MockServerInHandle) Handle(ctx context.Context, info *tap.Info) (context.Context, error) {
	args := h.Called(ctx, info)

	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(context.Context), nil
}
