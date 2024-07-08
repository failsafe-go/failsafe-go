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
