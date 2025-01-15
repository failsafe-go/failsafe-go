package testutil

import (
	"context"
	"log"
	"net"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/failsafe-go/failsafe-go/internal/testutil/pbfixtures"
)

type pingService struct {
	pbfixtures.UnimplementedPingServiceServer
	responseFn func(ctx context.Context) (*pbfixtures.PingResponse, error)
}

func (s *pingService) Ping(ctx context.Context, req *pbfixtures.PingRequest) (*pbfixtures.PingResponse, error) {
	return s.responseFn(ctx)
}

func MockGrpcResponses(responses ...string) pbfixtures.PingServiceServer {
	calls := atomic.Int32{}
	return &pingService{responseFn: func(context.Context) (*pbfixtures.PingResponse, error) {
		idx := int(calls.Add(1)) - 1
		idx = int(min(float64(idx), float64(len(responses)-1)))
		return &pbfixtures.PingResponse{Msg: responses[idx]}, nil
	}}
}

func MockDelayedGrpcResponse(response string, delay time.Duration) pbfixtures.PingServiceServer {
	return &pingService{responseFn: func(ctx context.Context) (*pbfixtures.PingResponse, error) {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			return &pbfixtures.PingResponse{Msg: response}, nil
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}}
}

func MockGrpcError(err error) pbfixtures.PingServiceServer {
	return &pingService{responseFn: func(context.Context) (*pbfixtures.PingResponse, error) {
		return nil, err
	}}
}

func MockFlakyGrpcServer(failTimes int, err error, finalResponse string) pbfixtures.PingServiceServer {
	failures := atomic.Int32{}
	return &pingService{responseFn: func(context.Context) (*pbfixtures.PingResponse, error) {
		if failures.Add(1) <= int32(failTimes) {
			return nil, err
		} else {
			return &pbfixtures.PingResponse{Msg: finalResponse}, nil
		}
	}}
}

type Dialer func(context.Context, string) (net.Conn, error)

func GrpcServer(service pbfixtures.PingServiceServer, options ...grpc.ServerOption) (*grpc.Server, Dialer) {
	server := grpc.NewServer(options...)
	pbfixtures.RegisterPingServiceServer(server, service)
	listen := bufconn.Listen(1024)
	go func() {
		if err := server.Serve(listen); err != nil {
			log.Fatalf("Server exited with error: %v", err)
		}
	}()
	return server, func(context.Context, string) (net.Conn, error) {
		return listen.Dial()
	}
}

func GrpcClient(dialer Dialer, options ...grpc.DialOption) *grpc.ClientConn {
	opts := []grpc.DialOption{grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials())}
	opts = append(opts, options...)
	client, err := grpc.NewClient("passthrough://bufnet", opts...)
	if err != nil {
		panic(err)
	}
	return client
}
