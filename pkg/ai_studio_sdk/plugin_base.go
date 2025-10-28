package ai_studio_sdk

import (
	"context"
	"fmt"
	"strconv"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
)

// AIStudioPlugin provides a base class for AI Studio plugins with automatic service API access
type AIStudioPlugin struct {
	pb.UnimplementedPluginServiceServer

	// Service API client - automatically set up during initialization
	ServiceAPI mgmtpb.AIStudioManagementServiceClient

	// Plugin metadata
	PluginID uint32

	// Implementation interface
	impl AIStudioPluginImplementation

	// Internal state
	broker *goplugin.GRPCBroker
}


// NewAIStudioPlugin creates a new AI Studio plugin with the given implementation
func NewAIStudioPlugin(impl AIStudioPluginImplementation) *AIStudioPlugin {
	return &AIStudioPlugin{
		impl: impl,
	}
}

// Initialize implements the plugin lifecycle - automatically sets up service API client
func (p *AIStudioPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Info().Interface("config", req.Config).Msg("Initializing AI Studio plugin with SDK")

	// Extract plugin ID from config
	if pluginIDStr, ok := req.Config["plugin_id"]; ok {
		if pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32); err == nil {
			p.PluginID = uint32(pluginID)
			log.Info().Uint32("plugin_id", p.PluginID).Msg("Plugin ID set from config")
		} else {
			log.Warn().Str("plugin_id_str", pluginIDStr).Msg("Invalid plugin ID format in config")
		}
	} else {
		log.Warn().Msg("Plugin ID not found in config - service API calls may fail")
	}

	// Set up service API client if broker is available
	if p.broker != nil {
		if err := p.setupServiceAPIClient(); err != nil {
			log.Warn().Err(err).Msg("Failed to set up service API client - falling back to mock data")
		} else {
			log.Info().Msg("✅ Service API client set up successfully")
		}
	} else {
		log.Warn().Msg("GRPC broker not available - service API client not configured")
	}

	// Call implementation's initialization with config
	if p.impl != nil {
		if err := p.impl.OnInitialize(p.ServiceAPI, p.PluginID, req.Config); err != nil {
			log.Error().Err(err).Msg("Plugin implementation initialization failed")
			return &pb.InitResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Implementation initialization failed: %v", err),
			}, nil
		}
	}

	log.Info().Uint32("plugin_id", p.PluginID).Msg("✅ AI Studio plugin initialized successfully with SDK")
	return &pb.InitResponse{Success: true}, nil
}

// setupServiceAPIClient gets the service API client from the composite client
// The service client is automatically available via the existing bidirectional connection
func (p *AIStudioPlugin) setupServiceAPIClient() error {
	// Service API client will be injected via the composite client pattern
	// No separate connection needed - uses the existing bidirectional gRPC connection
	log.Info().Msg("Service API client will be available via existing bidirectional connection")
	return nil
}

// Ping implements plugin health check
func (p *AIStudioPlugin) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Healthy:   true,
		Timestamp: req.Timestamp,
	}, nil
}

// Shutdown implements graceful plugin shutdown
func (p *AIStudioPlugin) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Info().Msg("Shutting down AI Studio plugin")

	// Call implementation's shutdown
	if p.impl != nil {
		if err := p.impl.OnShutdown(); err != nil {
			log.Warn().Err(err).Msg("Plugin implementation shutdown error")
		}
	}

	return &pb.ShutdownResponse{
		Success: true,
	}, nil
}

// Call implements the generic RPC call interface, delegating to the implementation
func (p *AIStudioPlugin) Call(ctx context.Context, req *pb.CallRequest) (*pb.CallResponse, error) {
	if p.impl == nil {
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: "Plugin implementation not available",
		}, nil
	}

	// Call implementation's RPC handler
	responseData, err := p.impl.HandleCall(req.Method, []byte(req.Payload))
	if err != nil {
		log.Error().Err(err).Str("method", req.Method).Msg("Plugin RPC call failed")
		return &pb.CallResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.CallResponse{
		Success: true,
		Data:    string(responseData),
	}, nil
}

// GetAsset implements asset serving, delegating to the implementation
func (p *AIStudioPlugin) GetAsset(ctx context.Context, req *pb.GetAssetRequest) (*pb.GetAssetResponse, error) {
	if p.impl == nil {
		return &pb.GetAssetResponse{
			Success:      false,
			ErrorMessage: "Plugin implementation not available",
		}, nil
	}

	content, mimeType, err := p.impl.GetAsset(req.AssetPath)
	if err != nil {
		return &pb.GetAssetResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetAssetResponse{
		Success:  true,
		Content:  content,
		MimeType: mimeType,
	}, nil
}

// GetManifest implements manifest serving, delegating to the implementation
func (p *AIStudioPlugin) GetManifest(ctx context.Context, req *pb.GetManifestRequest) (*pb.GetManifestResponse, error) {
	if p.impl == nil {
		return &pb.GetManifestResponse{
			Success:      false,
			ErrorMessage: "Plugin implementation not available",
		}, nil
	}

	manifestData, err := p.impl.GetManifest()
	if err != nil {
		return &pb.GetManifestResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetManifestResponse{
		Success:      true,
		ManifestJson: string(manifestData),
	}, nil
}

// GetConfigSchema implements configuration schema, delegating to the implementation
func (p *AIStudioPlugin) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	if p.impl == nil {
		return &pb.GetConfigSchemaResponse{
			Success:      false,
			ErrorMessage: "Plugin implementation not available",
		}, nil
	}

	schemaData, err := p.impl.GetConfigSchema()
	if err != nil {
		return &pb.GetConfigSchemaResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(schemaData),
	}, nil
}

// Authenticate implements auth hook by delegating to implementation if it supports auth
// This allows hybrid plugins to implement both UI and auth capabilities
func (p *AIStudioPlugin) Authenticate(ctx context.Context, req *pb.AuthRequest) (*pb.AuthResponse, error) {
	// Check if implementation has Authenticate method (for hybrid plugins)
	type AuthPlugin interface {
		Authenticate(ctx context.Context, req *pb.AuthRequest) (*pb.AuthResponse, error)
	}

	if authImpl, ok := p.impl.(AuthPlugin); ok {
		return authImpl.Authenticate(ctx, req)
	}

	// Implementation doesn't support auth - return unimplemented
	return nil, fmt.Errorf("plugin does not implement authentication")
}

// GetAppByCredential delegates to implementation if supported
func (p *AIStudioPlugin) GetAppByCredential(ctx context.Context, req *pb.GetAppRequest) (*pb.GetAppResponse, error) {
	type AppCredentialPlugin interface {
		GetAppByCredential(ctx context.Context, req *pb.GetAppRequest) (*pb.GetAppResponse, error)
	}

	if appImpl, ok := p.impl.(AppCredentialPlugin); ok {
		return appImpl.GetAppByCredential(ctx, req)
	}

	return &pb.GetAppResponse{}, fmt.Errorf("plugin does not implement GetAppByCredential")
}

// GetUserByCredential delegates to implementation if supported
func (p *AIStudioPlugin) GetUserByCredential(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	type UserCredentialPlugin interface {
		GetUserByCredential(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error)
	}

	if userImpl, ok := p.impl.(UserCredentialPlugin); ok {
		return userImpl.GetUserByCredential(ctx, req)
	}

	return &pb.GetUserResponse{}, fmt.Errorf("plugin does not implement GetUserByCredential")
}

// SetBroker allows the SDK to access the GRPCBroker for service API setup
func (p *AIStudioPlugin) SetBroker(broker *goplugin.GRPCBroker) {
	p.broker = broker
}

// SetServiceAPI allows the composite client to inject the service API client
func (p *AIStudioPlugin) SetServiceAPI(serviceClient mgmtpb.AIStudioManagementServiceClient) {
	p.ServiceAPI = serviceClient
	log.Info().Msg("✅ Service API client injected into SDK plugin")
}

// AIStudioAgentPlugin provides a base class for AI Studio agent plugins with automatic service API access
type AIStudioAgentPlugin struct {
	pb.UnimplementedPluginServiceServer

	// Service API client - automatically set up during initialization
	ServiceAPI mgmtpb.AIStudioManagementServiceClient

	// Plugin metadata
	PluginID uint32

	// Implementation interface
	impl AgentPluginImplementation

	// Internal state
	broker *goplugin.GRPCBroker
}

// NewAIStudioAgentPlugin creates a new AI Studio agent plugin with the given implementation
func NewAIStudioAgentPlugin(impl AgentPluginImplementation) *AIStudioAgentPlugin {
	return &AIStudioAgentPlugin{
		impl: impl,
	}
}

// Initialize implements the plugin lifecycle - automatically sets up service API client
func (p *AIStudioAgentPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Info().Interface("config", req.Config).Msg("Initializing AI Studio agent plugin with SDK")

	// Extract plugin ID from config
	if pluginIDStr, ok := req.Config["plugin_id"]; ok {
		if pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32); err == nil {
			p.PluginID = uint32(pluginID)
			log.Info().Uint32("plugin_id", p.PluginID).Msg("Plugin ID set from config")
		} else {
			log.Warn().Str("plugin_id_str", pluginIDStr).Msg("Invalid plugin ID format in config")
		}
	} else {
		log.Warn().Msg("Plugin ID not found in config - service API calls may fail")
	}

	// Set up service API client if broker is available
	if p.broker != nil {
		if err := p.setupServiceAPIClient(); err != nil {
			log.Warn().Err(err).Msg("Failed to set up service API client - falling back to mock data")
		} else {
			log.Info().Msg("✅ Service API client set up successfully")
		}
	} else {
		log.Warn().Msg("GRPC broker not available - service API client not configured")
	}

	// Call implementation's initialization with config
	if p.impl != nil {
		if err := p.impl.OnInitialize(p.ServiceAPI, p.PluginID, req.Config); err != nil {
			log.Error().Err(err).Msg("Agent plugin implementation initialization failed")
			return &pb.InitResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Implementation initialization failed: %v", err),
			}, nil
		}
	}

	log.Info().Uint32("plugin_id", p.PluginID).Msg("✅ AI Studio agent plugin initialized successfully with SDK")
	return &pb.InitResponse{Success: true}, nil
}

// setupServiceAPIClient gets the service API client from the composite client
func (p *AIStudioAgentPlugin) setupServiceAPIClient() error {
	log.Info().Msg("Service API client will be available via existing bidirectional connection")
	return nil
}

// Ping implements plugin health check for agent plugins
func (p *AIStudioAgentPlugin) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Healthy:   true,
		Timestamp: req.Timestamp,
	}, nil
}

// Shutdown implements graceful plugin shutdown
func (p *AIStudioAgentPlugin) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Info().Msg("Shutting down AI Studio agent plugin")

	// Call implementation's shutdown
	if p.impl != nil {
		if err := p.impl.OnShutdown(); err != nil {
			log.Warn().Err(err).Msg("Agent plugin implementation shutdown error")
		}
	}

	return &pb.ShutdownResponse{
		Success: true,
	}, nil
}

// HandleAgentMessage implements the agent message handler, delegating to the implementation
func (p *AIStudioAgentPlugin) HandleAgentMessage(req *pb.AgentMessageRequest, stream pb.PluginService_HandleAgentMessageServer) error {
	if p.impl == nil {
		return fmt.Errorf("agent plugin implementation not available")
	}

	// Extract broker ID and plugin ID from context metadata for service API access
	if req.Context != nil && req.Context.Metadata != nil {
		// Extract broker ID
		if brokerIDStr, ok := req.Context.Metadata["_service_broker_id"]; ok {
			if brokerID, err := strconv.ParseUint(brokerIDStr, 10, 32); err == nil {
				SetServiceBrokerID(uint32(brokerID))
				log.Debug().Uint32("broker_id", uint32(brokerID)).Msg("Service broker ID set from agent request context")
			}
		}

		// Extract plugin ID for authentication
		if pluginIDStr, ok := req.Context.Metadata["plugin_id"]; ok {
			if pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32); err == nil {
				SetPluginID(uint32(pluginID))
				log.Debug().Uint32("plugin_id", uint32(pluginID)).Msg("Plugin ID set from agent request context")
			}
		}
	}

	// Delegate to implementation
	return p.impl.HandleAgentMessage(req, stream)
}

// GetManifest implements manifest serving, delegating to the implementation
func (p *AIStudioAgentPlugin) GetManifest(ctx context.Context, req *pb.GetManifestRequest) (*pb.GetManifestResponse, error) {
	if p.impl == nil {
		return &pb.GetManifestResponse{
			Success:      false,
			ErrorMessage: "Agent plugin implementation not available",
		}, nil
	}

	manifestData, err := p.impl.GetManifest()
	if err != nil {
		return &pb.GetManifestResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetManifestResponse{
		Success:      true,
		ManifestJson: string(manifestData),
	}, nil
}

// GetConfigSchema implements configuration schema, delegating to the implementation
func (p *AIStudioAgentPlugin) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	if p.impl == nil {
		return &pb.GetConfigSchemaResponse{
			Success:      false,
			ErrorMessage: "Agent plugin implementation not available",
		}, nil
	}

	schemaData, err := p.impl.GetConfigSchema()
	if err != nil {
		return &pb.GetConfigSchemaResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(schemaData),
	}, nil
}

// SetBroker allows the SDK to access the GRPCBroker for service API setup
func (p *AIStudioAgentPlugin) SetBroker(broker *goplugin.GRPCBroker) {
	p.broker = broker
}

// SetServiceAPI allows the composite client to inject the service API client
func (p *AIStudioAgentPlugin) SetServiceAPI(serviceClient mgmtpb.AIStudioManagementServiceClient) {
	p.ServiceAPI = serviceClient
	log.Info().Msg("✅ Service API client injected into SDK agent plugin")
}