package failsafegrpc

import (
	"context"
	"github.com/failsafe-go/failsafe-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/tap"
)

// UnaryServerResponse is a struct that contains the context, request, and response of a unary gRPC call.
type UnaryServerResponse struct {
	Ctx  context.Context
	Info *grpc.UnaryServerInfo
	Req  any
	Resp any
}

// UnaryServerInterceptor returns a gRPC unary server interceptor that wraps the handler with a failsafe executor.
// `any` in failsafe.Executor[any] refers to the response of the gRPC call.
func UnaryServerInterceptor(executor failsafe.Executor[*UnaryServerResponse]) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		executorWithContext := executor.WithContext(ctx)

		operation := func(exec failsafe.Execution[*UnaryServerResponse]) (*UnaryServerResponse, error) {
			reply, err := handler(ctx, req)

			resp := &UnaryServerResponse{
				Ctx:  ctx,
				Req:  req,
				Resp: reply,
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

		return result.Resp, nil
	}
}

// InHandleResult is a struct that contains the context and info of a tap event.
// It is used to pass the context and info to the after hook.
type InHandleResult struct {
	Ctx  context.Context
	Info *tap.Info
}

// InHandleAfterHook is a tap.ServerInHandle that wraps the handler with a failsafe executor.
// It returns a tap.ServerInHandle that executes the handler and returns the context and info of the tap event.
func InHandleAfterHook(
	executor failsafe.Executor[*InHandleResult],
	serverInHandle tap.ServerInHandle,
) tap.ServerInHandle {
	return func(originCtx context.Context, info *tap.Info) (context.Context, error) {
		executorWithContext := executor.WithContext(originCtx)

		operation := func(exec failsafe.Execution[*InHandleResult]) (*InHandleResult, error) {
			ctx, err := serverInHandle(originCtx, info)

			res := &InHandleResult{
				Ctx:  ctx,
				Info: info,
			}
			if err != nil {
				return res, err
			}

			return res, nil
		}

		result, err := executorWithContext.GetWithExecution(operation)
		if err != nil {
			return nil, err
		}

		return result.Ctx, nil
	}
}
