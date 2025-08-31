package failsafegrpc

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/internal/util"
	"github.com/failsafe-go/failsafe-go/priority"
)

const (
	priorityMetadataKey = "x-failsafe-priority"
	levelMetadataKey    = "x-failsafe-level"
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
		// Merge the request context with the Executor so it's available for policies
		mergedCtx, cancel := util.MergeContexts(ctx, executor.Context())
		defer cancel(nil)
		if mergedCtx != ctx {
			executor = executor.WithContext(mergedCtx)
		}

		_, err := executor.GetWithExecution(func(exec failsafe.Execution[R]) (R, error) {
			// Merge the latest execution context for each attempt
			innerCtx, innerCancel := util.MergeContexts(mergedCtx, exec.Context())
			defer innerCancel(nil)

			var response R
			response, _ = reply.(R)
			return response, invoker(innerCtx, method, req, reply, cc, opts...)
		})
		return err
	}
}

// NewUnaryClientInterceptorWithLevel propagates adaptivelimiter priority and level information from a client context to
// a server via metadata. If a level is present it's propagated, else a priority is propagated if present.
func NewUnaryClientInterceptorWithLevel() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		// Lazily construct or copy metadata
		lazyMd := func() metadata.MD {
			if md == nil {
				return metadata.New(nil)
			} else {
				return md.Copy()
			}
		}

		if untypedLevel := ctx.Value(priority.LevelKey); untypedLevel != nil {
			if level, ok := untypedLevel.(int); ok {
				md = lazyMd()
				md.Set(levelMetadataKey, strconv.Itoa(level))
			}
		} else if untypedPriority := ctx.Value(priority.PriorityKey); untypedPriority != nil {
			if priority, ok := untypedPriority.(priority.Priority); ok {
				md = lazyMd()
				md.Set(priorityMetadataKey, strconv.Itoa(int(priority)))
			}
		}

		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
