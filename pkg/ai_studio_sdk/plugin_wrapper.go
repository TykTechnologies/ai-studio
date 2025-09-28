package ai_studio_sdk

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// Global service reference access for plugin GRPCServer
// This is set by the AI Studio plugin manager
var globalServiceReference interface{}

// SetGlobalServiceReference allows the plugin manager to provide service access
func SetGlobalServiceReference(service interface{}) {
	globalServiceReference = service
}

// NewAIStudioServiceServer creates service server (declaration to avoid import cycle)
var NewAIStudioServiceServer func(service interface{}) interface{}

// AIStudioPluginGRPC implements the go-plugin Plugin interface for AI Studio plugins
// This wrapper automatically provides service API access to plugin implementations
type AIStudioPluginGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl AIStudioPluginImplementation
}

// GRPCServer is called by go-plugin framework to register services on plugin's server
func (p *AIStudioPluginGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// Create SDK plugin instance
	plugin := NewAIStudioPlugin(p.Impl)

	// Initialize SDK with broker access (this is the plugin process)
	// Plugin ID will be set later during Initialize() call from config
	if err := Initialize(s, broker, 0); err != nil {
		log.Warn().Err(err).Msg("Failed to initialize AI Studio SDK")
	}

	// Register plugin services (Host → Plugin direction)
	pb.RegisterPluginServiceServer(s, plugin)

	log.Info().Msg("✅ Plugin services registered - broker stored for service API access")
	return nil
}

// GRPCClient is called by go-plugin framework to get client interface
func (p *AIStudioPluginGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	// Simple plugin client - service API access happens via broker pattern
	return pb.NewPluginServiceClient(c), nil
}

// ServePlugin is a convenience function for plugin developers to serve their plugin
// This replaces the manual go-plugin setup with SDK-based serving
func ServePlugin(impl AIStudioPluginImplementation) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "AI_STUDIO_PLUGIN",
			MagicCookieValue: "v1",
		},
		Plugins: map[string]goplugin.Plugin{
			"plugin": &AIStudioPluginGRPC{
				Impl: impl,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer, // Force gRPC protocol
	})
}