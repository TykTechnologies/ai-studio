package services

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pluginContextKey carries the calling plugin's ID on a brokered server.
// IMPORTANT: it must match pluginContextKeyString in services/grpc/auth_interceptor.go.
const pluginContextKey = "midsommar:plugin:id"

// CreatePluginIDInterceptor creates a gRPC interceptor that injects the plugin ID into context
// This is used for the brokered gRPC server where we already know the plugin ID from the caller
func CreatePluginIDInterceptor(pluginID uint) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(context.WithValue(ctx, pluginContextKey, pluginID), req)
	}
}

// createPluginIDStreamInterceptor is CreatePluginIDInterceptor for streaming RPCs.
func createPluginIDStreamInterceptor(pluginID uint) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &contextServerStream{ServerStream: ss, ctx: context.WithValue(ss.Context(), pluginContextKey, pluginID)})
	}
}

// contextServerStream overrides a stream's context.
type contextServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *contextServerStream) Context() context.Context { return s.ctx }

// PluginScopeInterceptorsFunc builds the interceptors that check a plugin's
// approved service scopes on the management service. It is set by the
// services/grpc package (which owns the method-to-scope map) to avoid an
// import cycle.
var PluginScopeInterceptorsFunc func(*Service) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor)

// managementServicePrefix is the gRPC method prefix of the AI Studio
// management service that plugins call back into.
const managementServicePrefix = "/ai_studio_management.AIStudioManagementService/"

// PluginBrokerServerOptions adds to opts the interceptors every brokered
// server that serves plugin pluginID must have: the plugin ID in the context
// (unary and streaming), then the scope check for management RPCs. Without
// the scope check registered, management RPCs are refused rather than served
// unchecked.
func PluginBrokerServerOptions(opts []grpc.ServerOption, pluginID uint, svc *Service) []grpc.ServerOption {
	unary := []grpc.UnaryServerInterceptor{CreatePluginIDInterceptor(pluginID)}
	stream := []grpc.StreamServerInterceptor{createPluginIDStreamInterceptor(pluginID)}
	if PluginScopeInterceptorsFunc != nil && svc != nil {
		u, s := PluginScopeInterceptorsFunc(svc)
		unary = append(unary, u)
		stream = append(stream, s)
	} else {
		unary = append(unary, denyManagementUnary)
		stream = append(stream, denyManagementStream)
	}
	return append(opts, grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(stream...))
}

var errScopeCheckUnavailable = status.Error(codes.Unavailable, "plugin scope checks are not available")

func denyManagementUnary(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if strings.HasPrefix(info.FullMethod, managementServicePrefix) {
		return nil, errScopeCheckUnavailable
	}
	return handler(ctx, req)
}

func denyManagementStream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if strings.HasPrefix(info.FullMethod, managementServicePrefix) {
		return errScopeCheckUnavailable
	}
	return handler(srv, ss)
}
