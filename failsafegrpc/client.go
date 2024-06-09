package failsafegrpc

import (
	"context"
	"github.com/failsafe-go/failsafe-go"
	"google.golang.org/grpc"
)

// UnaryClientInterceptor returns a gRPC unary client interceptor that wraps the invoker with a failsafe executor.
// `any` in failsafe.Executor[any] refers to the response of the gRPC call.
func UnaryClientInterceptor(executor failsafe.Executor[any]) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		executorWithContext := executor.WithContext(ctx)

		operation := func(exec failsafe.Execution[any]) (any, error) {
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err != nil {
				return reply, err
			}

			return reply, nil
		}

		_, err := executorWithContext.GetWithExecution(operation)
		if err != nil {
			return err
		}

		return nil
	}
}

// StreamClientInterceptor returns a gRPC stream client interceptor that wraps the streamer with a failsafe executor.
// If you want to use the response of the gRPC call in the policies, override `RecvMsg` and `SendMsg` in the grpc.ClientStream.
func StreamClientInterceptor(executor failsafe.Executor[grpc.ClientStream]) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		executorWithContext := executor.WithContext(ctx)

		operation := func(exec failsafe.Execution[grpc.ClientStream]) (grpc.ClientStream, error) {
			clientStream, err := streamer(ctx, desc, cc, method, opts...)
			if err != nil {
				return clientStream, err
			}

			return clientStream, nil
		}

		result, err := executorWithContext.GetWithExecution(operation)
		if err != nil {
			return nil, err
		}

		return result, nil
	}
}
