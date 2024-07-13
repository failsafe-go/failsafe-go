package failsafegrpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/tap"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
)

// ServerInHandle returns a tap.ServerInHandle that wraps the handler with a failsafe.Executor. This can be used to limit
// server side load with policies such as CircuitBreaker, Bulkhead, RateLimiter, and Cache, and should be prefered over
// UnaryServerInterceptor since it does not waste resources for requests that are rejected.
func ServerInHandle[R any](executor failsafe.Executor[R]) tap.ServerInHandle {
	return func(ctx context.Context, info *tap.Info) (context.Context, error) {
		return ctx, executor.Run(func() error {
			// The execution is a noop since it's meant to be used with load limiting policies
			return nil
		})
	}
}

// UnaryServerInterceptor returns a grpc.UnaryServerInterceptor that wraps the handler with a failsafe.Executor. This can
// be used to limit server side load where the content of the request might influence whether it's rejected or not, such
// as with a CircuitBreaker. For load limiting that does not require inspecting requests, prefer ServerInHandle.
func UnaryServerInterceptor[R any](executor failsafe.Executor[R]) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return executor.GetWithExecution(func(exec failsafe.Execution[R]) (R, error) {
			mergedCtx, cancel := util.MergeContexts(ctx, exec.Context())
			defer cancel(nil)
			resp, err := handler(mergedCtx, req)
			var response R
			response, _ = resp.(R)
			return response, err
		})
	}
}
