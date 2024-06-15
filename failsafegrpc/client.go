package failsafegrpc

import (
	"context"
	"github.com/failsafe-go/failsafe-go"
	"google.golang.org/grpc"
)

// UnaryClientResponse is a struct that contains the context, method, request, and response of a unary gRPC call.
type UnaryClientResponse struct {
	Ctx    context.Context
	Method string
	Req    any
	Reply  any
}

// UnaryClientInterceptor returns a gRPC unary client interceptor that wraps the invoker with a failsafe executor.
func UnaryClientInterceptor(executor failsafe.Executor[*UnaryClientResponse]) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		executorWithContext := executor.WithContext(ctx)

		operation := func(exec failsafe.Execution[*UnaryClientResponse]) (*UnaryClientResponse, error) {
			err := invoker(ctx, method, req, reply, cc, opts...)

			resp := &UnaryClientResponse{
				Ctx:    ctx,
				Method: method,
				Req:    req,
				Reply:  reply,
			}
			if err != nil {
				return resp, err
			}

			return resp, nil
		}

		_, err := executorWithContext.GetWithExecution(operation)
		if err != nil {
			return err
		}

		return nil
	}
}

// StreamClientResponse is a struct that contains the context, stream descriptor, method, and streamer of a stream gRPC call.
type StreamClientResponse struct {
	Ctx    context.Context
	Desc   *grpc.StreamDesc
	Method string
	Stream grpc.ClientStream
}

// StreamClientInterceptor returns a gRPC stream client interceptor that wraps the streamer with a failsafe executor.
// If you want to use the response of the gRPC call in the policies, override `RecvMsg` and `SendMsg` in the grpc.ClientStream.
func StreamClientInterceptor(executor failsafe.Executor[*StreamClientResponse]) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		executorWithContext := executor.WithContext(ctx)

		operation := func(exec failsafe.Execution[*StreamClientResponse]) (*StreamClientResponse, error) {
			clientStream, err := streamer(ctx, desc, cc, method, opts...)

			resp := &StreamClientResponse{
				Ctx:    ctx,
				Desc:   desc,
				Method: method,
				Stream: clientStream,
			}
			if err != nil {
				return resp, err
			}

			return resp, nil
		}

		result, err := executorWithContext.GetWithExecution(operation)
		if err != nil {
			return nil, err
		}

		return result.Stream, nil
	}
}
