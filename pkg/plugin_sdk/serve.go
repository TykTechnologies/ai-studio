package plugin_sdk

import (
	"context"
	"fmt"
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// Serve starts the plugin and handles all the go-plugin setup.
// This is the main entry point for plugin developers.
//
// Example usage:
//
//	func main() {
//	    plugin := &MyPlugin{}
//	    plugin_sdk.Serve(plugin)
//	}
func Serve(userPlugin Plugin) {
	// Detect runtime from environment
	runtime := detectRuntime()

	log.Printf("Starting plugin: %s v%s (runtime: %s)",
		userPlugin.GetInfo().Name,
		userPlugin.GetInfo().Version,
		runtime)

	// Create the plugin wrapper
	// Note: We'll create the service broker later when we get the plugin ID from Initialize
	wrapper := &pluginServerWrapper{
		plugin:   userPlugin,
		runtime:  runtime,
		services: nil, // Will be set in Initialize
	}

	// Serve using go-plugin
	// Register both "plugin" and "config" services for full and config-only loading
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "AI_STUDIO_PLUGIN",
			MagicCookieValue: "v1",
		},
		Plugins: map[string]goplugin.Plugin{
			"plugin": &grpcPluginImpl{wrapper: wrapper},
			"config": &configPluginImpl{wrapper: wrapper},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}

// grpcPluginImpl implements goplugin.GRPCPlugin
// We embed NetRPCUnsupportedPlugin to indicate we only support gRPC, not the legacy netrpc protocol
type grpcPluginImpl struct {
	goplugin.NetRPCUnsupportedPlugin
	wrapper *pluginServerWrapper
}

// GRPCServer registers the plugin service with the gRPC server
func (p *grpcPluginImpl) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// Register the proto service
	pb.RegisterPluginServiceServer(s, p.wrapper)

	// Initialize SDK based on detected runtime
	// IMPORTANT: Only initialize ONE SDK - they share the same broker and will conflict
	// The broker has EITHER MicrogatewayManagementService OR AIStudioManagementService, not both

	runtime := detectRuntime()
	// IMPORTANT: Cannot use fmt.Printf during plugin startup - breaks go-plugin handshake
	// Use log.Printf which goes to hclog and doesn't interfere

	// Store broker for event service access (works in both contexts)
	SetEventServiceBroker(broker)

	if runtime == RuntimeGateway {
		// Gateway context - use Microgateway SDK
		if err := initializeMicrogatewaySDK(s, broker, 0); err != nil {
			log.Printf("Warning: Failed to initialize Microgateway SDK: %v", err)
		}
	} else {
		// Studio context - use AI Studio SDK
		if err := ai_studio_sdk.Initialize(s, broker, 0); err != nil {
			log.Printf("Warning: Failed to initialize AI Studio SDK: %v", err)
		}
	}

	return nil
}

// GRPCClient returns the plugin client interface
func (p *grpcPluginImpl) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

// configPluginImpl implements the config provider for schema extraction
type configPluginImpl struct {
	goplugin.NetRPCUnsupportedPlugin
	wrapper *pluginServerWrapper
}

// GRPCServer registers only the ConfigProviderService for config-only loading
func (p *configPluginImpl) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// Register config provider service that wraps our plugin
	configpb.RegisterConfigProviderServiceServer(s, &configProviderServer{wrapper: p.wrapper})
	return nil
}

// GRPCClient returns the config provider client
func (p *configPluginImpl) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return configpb.NewConfigProviderServiceClient(c), nil
}

// configProviderServer implements the ConfigProviderService
type configProviderServer struct {
	configpb.UnimplementedConfigProviderServiceServer
	wrapper *pluginServerWrapper
}

// Ping implements the ConfigProviderService
func (s *configProviderServer) Ping(ctx context.Context, req *configpb.ConfigPingRequest) (*configpb.ConfigPingResponse, error) {
	return &configpb.ConfigPingResponse{
		Timestamp: req.Timestamp,
		Healthy:   true,
	}, nil
}

// GetConfigSchema implements the ConfigProviderService
func (s *configProviderServer) GetConfigSchema(ctx context.Context, req *configpb.ConfigSchemaRequest) (*configpb.ConfigSchemaResponse, error) {
	// Check if plugin implements ConfigProvider
	if provider, ok := s.wrapper.plugin.(ConfigProvider); ok {
		schemaBytes, err := provider.GetConfigSchema()
		if err != nil {
			return &configpb.ConfigSchemaResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &configpb.ConfigSchemaResponse{
			Success:    true,
			SchemaJson: string(schemaBytes),
		}, nil
	}

	// Return empty schema if not implemented
	return &configpb.ConfigSchemaResponse{
		Success:    true,
		SchemaJson: "{}",
	}, nil
}

// GetManifest implements the ConfigProviderService
func (s *configProviderServer) GetManifest(ctx context.Context, req *configpb.GetManifestRequest) (*configpb.GetManifestResponse, error) {
	// Try UIProvider first
	if provider, ok := s.wrapper.plugin.(UIProvider); ok {
		manifestBytes, err := provider.GetManifest()
		if err != nil {
			return &configpb.GetManifestResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &configpb.GetManifestResponse{
			Success:      true,
			ManifestJson: string(manifestBytes),
		}, nil
	}

	// Try AgentPlugin
	if agent, ok := s.wrapper.plugin.(AgentPlugin); ok {
		manifestBytes, err := agent.GetManifest()
		if err != nil {
			return &configpb.GetManifestResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &configpb.GetManifestResponse{
			Success:      true,
			ManifestJson: string(manifestBytes),
		}, nil
	}

	// Try ManifestProvider (for gateway plugins that need manifest but not full UI)
	if manifestProvider, ok := s.wrapper.plugin.(ManifestProvider); ok {
		manifestBytes, err := manifestProvider.GetManifest()
		if err != nil {
			return &configpb.GetManifestResponse{
				Success:      false,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &configpb.GetManifestResponse{
			Success:      true,
			ManifestJson: string(manifestBytes),
		}, nil
	}

	// Return empty manifest
	return &configpb.GetManifestResponse{
		Success:      true,
		ManifestJson: "{}",
	}, nil
}

// Override the Initialize method to set up services after we get the plugin ID
func (w *pluginServerWrapper) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	// Extract plugin ID from config if available
	var pluginID uint32
	if req.Config != nil {
		// Try both _plugin_id (Microgateway) and plugin_id (AI Studio)
		if idStr, ok := req.Config["_plugin_id"]; ok {
			// Parse plugin ID
			var id int
			if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
				pluginID = uint32(id)
			}
		} else if idStr, ok := req.Config["plugin_id"]; ok {
			// Parse plugin ID
			var id int
			if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
				pluginID = uint32(id)
			}
		}

		// Set plugin ID in the appropriate SDK
		if pluginID > 0 {
			if w.runtime == RuntimeGateway {
				setPluginIDForMicrogatewaySDK(pluginID)
				log.Printf("Set Microgateway plugin ID: %d", pluginID)
			} else {
				ai_studio_sdk.SetPluginID(pluginID)
				log.Printf("Set AI Studio plugin ID: %d", pluginID)
			}
		}

		// Extract and set broker ID for service API access
		// Try both _service_broker_id (Microgateway) and service_broker_id (AI Studio)
		brokerIDStr := ""
		if id, ok := req.Config["_service_broker_id"]; ok {
			brokerIDStr = id
		} else if id, ok := req.Config["service_broker_id"]; ok {
			brokerIDStr = id
		}

		if brokerIDStr != "" {
			var brokerID int
			if _, err := fmt.Sscanf(brokerIDStr, "%d", &brokerID); err == nil {
				// Set broker ID for the appropriate SDK based on runtime
				if w.runtime == RuntimeGateway {
					setBrokerIDForMicrogatewaySDK(uint32(brokerID))
					log.Printf("Set Microgateway service broker ID: %d", brokerID)
				} else {
					ai_studio_sdk.SetServiceBrokerID(uint32(brokerID))
					log.Printf("Set AI Studio service broker ID: %d", brokerID)
				}

				// Also set broker ID for event service (works in both contexts)
				SetEventServiceBrokerID(uint32(brokerID))
				log.Printf("Set event service broker ID: %d", brokerID)

				// NOTE: Do NOT eagerly initialize event service client here.
				// The host's AcceptAndServe goroutine may not have sent the connection info yet
				// (there's a race between the goroutine calling Accept() and us calling Initialize).
				// Let the event service initialize lazily on first use (like AI Studio SDK does).
				// The lazy initialization in lazyEventService.getInner() will handle this correctly.
			}
		}
	}

	// Now that we have the plugin ID, create the service broker
	if w.services == nil {
		w.services = newServiceBroker(w.runtime, pluginID)
	}

	// Create plugin context and call user's Initialize
	pluginCtx := w.createPluginContext(ctx, nil)

	err := w.plugin.Initialize(pluginCtx, req.Config)
	if err != nil {
		return &pb.InitResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.InitResponse{Success: true}, nil
}
