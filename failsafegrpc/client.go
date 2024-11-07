package failsafegrpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
)

// NewUnaryClientInterceptor returns a grpc.UnaryClientInterceptor that wraps the invoker with the policies.
//
// R is the response type.
func NewUnaryClientInterceptor[R any](policies ...failsafe.Policy[R]) grpc.UnaryClientInterceptor {
	return NewUnaryClientInterceptorWithExecutor(failsafe.NewExecutor(policies...))
}

// NewUnaryClientInterceptorWithExecutor returns a grpc.UnaryClientInterceptor that wraps the invoker with a failsafe.Executor.
//
// R is the response type.
func NewUnaryClientInterceptorWithExecutor[R any](executor failsafe.Executor[R]) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := executor.GetWithExecution(func(exec failsafe.Execution[R]) (R, error) {
			mergedCtx, cancel := util.MergeContexts(ctx, exec.Context())
			defer cancel(nil)
			var response R
			response, _ = reply.(R)
			return response, invoker(mergedCtx, method, req, reply, cc, opts...)
		})
		return err
	}
}
