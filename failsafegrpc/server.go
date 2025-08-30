package failsafegrpc

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/tap"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/adaptivelimiter"
	"github.com/failsafe-go/failsafe-go/internal/util"
)

// NewServerInHandle returns a tap.ServerInHandle that wraps the handler with the policies. This can be used to limit
// server side load with policies such as CircuitBreaker, Bulkhead, RateLimiter, and Cache, and should be prefered over
// NewUnaryServerInterceptor since it does not waste resources for requests that are rejected.
func NewServerInHandle[R any](policies ...failsafe.Policy[R]) tap.ServerInHandle {
	return NewServerInHandleWithExecutor(failsafe.NewExecutor(policies...))
}

// NewServerInHandleWithExecutor returns a tap.ServerInHandle that wraps the handler with a failsafe.Executor. This can be used to limit
// server side load with policies such as CircuitBreaker, Bulkhead, RateLimiter, and Cache, and should be prefered over
// NewUnaryServerInterceptorWithExecutor since it does not waste resources for requests that are rejected.
func NewServerInHandleWithExecutor[R any](executor failsafe.Executor[R]) tap.ServerInHandle {
	return func(ctx context.Context, info *tap.Info) (context.Context, error) {
		return ctx, executor.Run(func() error {
			// The execution is a noop since it's meant to be used with load limiting policies
			return nil
		})
	}
}

// NewUnaryServerInterceptor returns a grpc.UnaryServerInterceptor that wraps the handler the policies. This can be used
// to limit server side load where the content of the request might influence whether it's rejected or not, such as with
// a CircuitBreaker. For load limiting that does not require inspecting requests, prefer NewServerInHandle.
// R is the response type.
func NewUnaryServerInterceptor[R any](policies ...failsafe.Policy[R]) grpc.UnaryServerInterceptor {
	return NewUnaryServerInterceptorWithExecutor(failsafe.NewExecutor(policies...))
}

// NewUnaryServerInterceptorWithExecutor returns a grpc.UnaryServerInterceptor that wraps the handler with a failsafe.Executor. This can
// be used to limit server side load where the content of the request might influence whether it's rejected or not, such
// as with a CircuitBreaker. For load limiting that does not require inspecting requests, prefer NewServerInHandleWithExecutor.
// R is the response type.
func NewUnaryServerInterceptorWithExecutor[R any](executor failsafe.Executor[R]) grpc.UnaryServerInterceptor {
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

// NewUnaryServerInterceptorWithLevel extracts adaptivelimiter priority and level information from an incoming request
// and adds a level to the handling context. If a level is present in the incoming request metadata, it's added to the
// context. If a level is not present and ensureLevel is true, then a level will be generated from a priority, if one is
// present, else a level 0 will be used.
func NewUnaryServerInterceptorWithLevel(ensureLevel bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !ensureLevel {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return handler(adaptivelimiter.ContextWithLevel(ctx, 0), req)
		}
		if levels := md.Get(levelMetadataKey); len(levels) > 0 {
			if level, err := strconv.Atoi(levels[0]); err == nil {
				return handler(adaptivelimiter.ContextWithLevel(ctx, level), req)
			}
		}
		if priorities := md.Get(priorityMetadataKey); len(priorities) > 0 {
			if priorityInt, err := strconv.Atoi(priorities[0]); err == nil {
				level := adaptivelimiter.Priority(priorityInt).RandomLevel()
				return handler(adaptivelimiter.ContextWithLevel(ctx, level), req)
			}
		}
		return handler(adaptivelimiter.ContextWithLevel(ctx, 0), req)
	}
}
