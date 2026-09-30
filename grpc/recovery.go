package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
)

// interceptors are the control server's interceptor chains. Recovery comes
// first, so a panic anywhere below it (authentication included) fails that
// call with Internal instead of ending the process: grpc-go recovers
// nothing itself.
func (s *ControlServer) interceptors() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(recoverUnary, s.authInterceptor),
		grpc.ChainStreamInterceptor(recoverStream, s.streamAuthInterceptor),
	}
}

// errRecovered is what a call that panicked returns; the panic itself is
// logged on this side only.
var errRecovered = status.Error(codes.Internal, "internal error")

func recoverUnary(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	if safe.Call("gRPC "+info.FullMethod, func() { resp, err = handler(ctx, req) }) {
		return nil, errRecovered
	}
	return resp, err
}

func recoverStream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	if safe.Call("gRPC "+info.FullMethod, func() { err = handler(srv, ss) }) {
		return errRecovered
	}
	return err
}
