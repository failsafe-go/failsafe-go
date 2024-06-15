package testutil

import (
	"context"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

type MockClientStreamer struct {
	mock.Mock

	Sleep time.Duration
}

func (s *MockClientStreamer) Stream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	args := s.Called(ctx, desc, cc, method, opts)

	time.Sleep(s.Sleep)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(grpc.ClientStream), nil
}

type MockRetryableClientStreamer struct {
	mock.Mock

	AllowedRetries int
	RetryErrorCode codes.Code

	retryCount int
}

func (s *MockRetryableClientStreamer) Stream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	args := s.Called(ctx, desc, cc, method, opts)

	if s.retryCount < s.AllowedRetries {
		s.retryCount++
		return nil, status.Error(s.RetryErrorCode, "retry request")
	}

	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(grpc.ClientStream), nil
}

type MockClientStream struct {
	AllowedRetries int
	RetryErrorCode codes.Code
	LastError      error

	retryCount int
}

var _ grpc.ClientStream = &MockClientStream{}

func (s *MockClientStream) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (s *MockClientStream) Trailer() metadata.MD {
	return metadata.MD{}
}

func (s *MockClientStream) CloseSend() error {
	return nil
}

func (s *MockClientStream) Context() context.Context {
	return context.Background()
}

func (s *MockClientStream) SendMsg(m interface{}) error {
	if s.retryCount < s.AllowedRetries {
		s.retryCount++
		return status.Error(s.RetryErrorCode, "retry request")
	}

	if s.LastError != nil {
		return s.LastError
	}

	return nil
}

func (s *MockClientStream) RecvMsg(m any) error {
	if s.retryCount < s.AllowedRetries {
		s.retryCount++
		return status.Error(s.RetryErrorCode, "retry request")
	}

	if s.LastError != nil {
		return s.LastError
	}

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

type MockStreamHandler struct {
	mock.Mock
}

func (h *MockStreamHandler) Handle(srv interface{}, stream grpc.ServerStream) error {
	args := h.Called(srv, stream)

	if args.Error(0) != nil {
		return args.Error(0)
	}

	return nil
}

type MockStreamHandleRequest struct{}

type MockStreamHandleResponse struct {
	Message string
}

type MockServerStream struct {
	ExpectedError error
}

var _ grpc.ServerStream = &MockServerStream{}

func (s *MockServerStream) SetHeader(md metadata.MD) error {
	return nil
}

func (s *MockServerStream) SendHeader(md metadata.MD) error {
	return nil
}

func (s *MockServerStream) SetTrailer(md metadata.MD) {}

func (s *MockServerStream) Context() context.Context {
	return context.Background()
}

func (s *MockServerStream) SendMsg(m any) error {
	if s.ExpectedError != nil {
		return s.ExpectedError
	}

	return nil
}

func (s *MockServerStream) RecvMsg(m any) error {
	if s.ExpectedError != nil {
		return s.ExpectedError
	}

	return nil
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
