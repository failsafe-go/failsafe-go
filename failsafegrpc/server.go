package failsafegrpc

import (
	"context"
	"github.com/failsafe-go/failsafe-go"
	"google.golang.org/grpc"
)

// UnaryServerInterceptor returns a gRPC unary server interceptor that wraps the handler with a failsafe executor.
// `any` in failsafe.Executor[any] refers to the response of the gRPC call.
func UnaryServerInterceptor(executor failsafe.Executor[any]) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		executorWithContext := executor.WithContext(ctx)

		operation := func(exec failsafe.Execution[any]) (any, error) {
			reply, err := handler(ctx, req)
			if err != nil {
				return reply, err
			}

			return reply, nil
		}

		reply, err := executorWithContext.GetWithExecution(operation)
		if err != nil {
			return nil, err
		}

		return reply, nil
	}
}
