package pbfixtures

import (
	"context"
)

type MockPingServer struct {
	UnimplementedPingServiceServer
	OnPing func(context.Context, *PingRequest) (*PingResponse, error)
}

func (m *MockPingServer) Ping(ctx context.Context, req *PingRequest) (*PingResponse, error) {
	if m.OnPing != nil {
		return m.OnPing(ctx, req)
	}
	return &PingResponse{Msg: "pong"}, nil
}
