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

	// Call implementation's initialization
	if p.impl != nil {
		if err := p.impl.OnInitialize(p.ServiceAPI, p.PluginID); err != nil {
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

// SetBroker allows the SDK to access the GRPCBroker for service API setup
func (p *AIStudioPlugin) SetBroker(broker *goplugin.GRPCBroker) {
	p.broker = broker
}

// SetServiceAPI allows the composite client to inject the service API client
func (p *AIStudioPlugin) SetServiceAPI(serviceClient mgmtpb.AIStudioManagementServiceClient) {
	p.ServiceAPI = serviceClient
	log.Info().Msg("✅ Service API client injected into SDK plugin")
}