package services

import (
	"context"

	"google.golang.org/grpc"
)

// CreatePluginIDInterceptor creates a gRPC interceptor that injects the plugin ID into context
// This is used for the brokered gRPC server where we already know the plugin ID from the caller
func CreatePluginIDInterceptor(pluginID uint) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Inject plugin ID into context using string key
		// IMPORTANT: This constant must match pluginContextKeyString in services/grpc/auth_interceptor.go
		const pluginContextKey = "midsommar:plugin:id"
		ctx = context.WithValue(ctx, pluginContextKey, pluginID)

		// Call the handler with the enriched context
		return handler(ctx, req)
	}
}