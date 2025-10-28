package ai_studio_sdk

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
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

// AIStudioConfigProviderGRPC implements the go-plugin Plugin interface for config-only access
type AIStudioConfigProviderGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl AIStudioPluginImplementation
}

// GRPCServer registers the ConfigProviderService for config-only loading
func (p *AIStudioConfigProviderGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// Register config provider service
	configpb.RegisterConfigProviderServiceServer(s, &AIStudioConfigProviderServer{impl: p.Impl})
	log.Debug().Msg("✅ ConfigProviderService registered for config-only access")
	return nil
}

// GRPCClient returns the config provider client
func (p *AIStudioConfigProviderGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return configpb.NewConfigProviderServiceClient(c), nil
}

// AIStudioConfigProviderServer implements the ConfigProviderService for config-only access
type AIStudioConfigProviderServer struct {
	configpb.UnimplementedConfigProviderServiceServer
	impl AIStudioPluginImplementation
}

// Ping implements config provider health check
func (s *AIStudioConfigProviderServer) Ping(ctx context.Context, req *configpb.ConfigPingRequest) (*configpb.ConfigPingResponse, error) {
	return &configpb.ConfigPingResponse{
		Healthy:   true,
		Timestamp: req.Timestamp,
	}, nil
}

// GetConfigSchema returns the plugin's configuration schema
func (s *AIStudioConfigProviderServer) GetConfigSchema(ctx context.Context, req *configpb.ConfigSchemaRequest) (*configpb.ConfigSchemaResponse, error) {
	if s.impl == nil {
		return &configpb.ConfigSchemaResponse{
			Success:      false,
			ErrorMessage: "Plugin implementation not available",
		}, nil
	}

	schema, err := s.impl.GetConfigSchema()
	if err != nil {
		return &configpb.ConfigSchemaResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &configpb.ConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(schema),
	}, nil
}

// GetManifest returns the plugin's manifest
func (s *AIStudioConfigProviderServer) GetManifest(ctx context.Context, req *configpb.GetManifestRequest) (*configpb.GetManifestResponse, error) {
	if s.impl == nil {
		return &configpb.GetManifestResponse{
			Success:      false,
			ErrorMessage: "Plugin implementation not available",
		}, nil
	}

	manifest, err := s.impl.GetManifest()
	if err != nil {
		return &configpb.GetManifestResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &configpb.GetManifestResponse{
		Success:      true,
		ManifestJson: string(manifest),
	}, nil
}

// ServePlugin is a convenience function for plugin developers to serve their plugin
// This replaces the manual go-plugin setup with SDK-based serving
// Registers both "plugin" and "config" services for full and config-only loading
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
			"config": &AIStudioConfigProviderGRPC{
				Impl: impl,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer, // Force gRPC protocol
	})
}

// AIStudioAgentPluginGRPC implements the go-plugin Plugin interface for AI Studio agent plugins
// This wrapper automatically provides service API access to agent plugin implementations
type AIStudioAgentPluginGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl AgentPluginImplementation
}

// GRPCServer is called by go-plugin framework to register services on agent plugin's server
func (p *AIStudioAgentPluginGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// Create SDK agent plugin instance
	plugin := NewAIStudioAgentPlugin(p.Impl)

	// Initialize SDK with broker access (this is the plugin process)
	// Plugin ID will be set later during Initialize() call from config
	if err := Initialize(s, broker, 0); err != nil {
		log.Warn().Err(err).Msg("Failed to initialize AI Studio SDK for agent plugin")
	}

	// Register plugin services (Host → Plugin direction)
	pb.RegisterPluginServiceServer(s, plugin)

	log.Info().Msg("✅ Agent plugin services registered - broker stored for service API access")
	return nil
}

// GRPCClient is called by go-plugin framework to get client interface
func (p *AIStudioAgentPluginGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	// Simple plugin client - service API access happens via broker pattern
	return pb.NewPluginServiceClient(c), nil
}

// AIStudioAgentConfigProviderGRPC implements config-only access for agent plugins
type AIStudioAgentConfigProviderGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl AgentPluginImplementation
}

// GRPCServer registers the ConfigProviderService for agent config-only loading
func (p *AIStudioAgentConfigProviderGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	configpb.RegisterConfigProviderServiceServer(s, &AIStudioAgentConfigProviderServer{impl: p.Impl})
	log.Debug().Msg("✅ ConfigProviderService registered for agent config-only access")
	return nil
}

// GRPCClient returns the config provider client
func (p *AIStudioAgentConfigProviderGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return configpb.NewConfigProviderServiceClient(c), nil
}

// AIStudioAgentConfigProviderServer implements ConfigProviderService for agent plugins
type AIStudioAgentConfigProviderServer struct {
	configpb.UnimplementedConfigProviderServiceServer
	impl AgentPluginImplementation
}

// Ping implements config provider health check
func (s *AIStudioAgentConfigProviderServer) Ping(ctx context.Context, req *configpb.ConfigPingRequest) (*configpb.ConfigPingResponse, error) {
	return &configpb.ConfigPingResponse{
		Healthy:   true,
		Timestamp: req.Timestamp,
	}, nil
}

// GetConfigSchema returns the agent plugin's configuration schema
func (s *AIStudioAgentConfigProviderServer) GetConfigSchema(ctx context.Context, req *configpb.ConfigSchemaRequest) (*configpb.ConfigSchemaResponse, error) {
	if s.impl == nil {
		return &configpb.ConfigSchemaResponse{
			Success:      false,
			ErrorMessage: "Agent plugin implementation not available",
		}, nil
	}

	schema, err := s.impl.GetConfigSchema()
	if err != nil {
		return &configpb.ConfigSchemaResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &configpb.ConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(schema),
	}, nil
}

// GetManifest returns the agent plugin's manifest
func (s *AIStudioAgentConfigProviderServer) GetManifest(ctx context.Context, req *configpb.GetManifestRequest) (*configpb.GetManifestResponse, error) {
	if s.impl == nil {
		return &configpb.GetManifestResponse{
			Success:      false,
			ErrorMessage: "Agent plugin implementation not available",
		}, nil
	}

	manifest, err := s.impl.GetManifest()
	if err != nil {
		return &configpb.GetManifestResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &configpb.GetManifestResponse{
		Success:      true,
		ManifestJson: string(manifest),
	}, nil
}

// ServeAgentPlugin is a convenience function for agent plugin developers to serve their plugin
// This replaces the manual go-plugin setup with SDK-based serving for agent plugins
// Registers both "plugin" and "config" services for full and config-only loading
func ServeAgentPlugin(impl AgentPluginImplementation) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "AI_STUDIO_PLUGIN",
			MagicCookieValue: "v1",
		},
		Plugins: map[string]goplugin.Plugin{
			"plugin": &AIStudioAgentPluginGRPC{
				Impl: impl,
			},
			"config": &AIStudioAgentConfigProviderGRPC{
				Impl: impl,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer, // Force gRPC protocol
	})
}