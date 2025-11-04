package services

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Global service reference for GRPCServer access
// This is set when the service is created to avoid circular dependencies
var globalServiceReference *Service

// NewAIStudioManagementServerFunc is a factory function for creating AIStudioManagementServer
// This is set by the grpc package to avoid circular imports
var NewAIStudioManagementServerFunc func(*Service) interface{}

// SetGlobalServiceReference sets the global service reference for GRPCServer access
func SetGlobalServiceReference(service *Service) {
	globalServiceReference = service
	logger.Info("Global service reference set for plugin GRPCServer access")
}


// AIStudioPluginManager manages AI Studio plugin lifecycle and execution
// Reuses proven patterns from microgateway's plugin manager
type AIStudioPluginManager struct {
	db              *gorm.DB
	ociClient       *ociplugins.OCIPluginClient
	manifestService *PluginManifestService
	service         *Service // Reference to main service for creating service providers
	mu              sync.RWMutex

	// Plugin runtime state
	loadedPlugins   map[uint]*LoadedAIStudioPlugin // plugin_id -> loaded plugin
	pluginClients   map[uint]*goplugin.Client       // plugin_id -> go-plugin client

	// Plugin configuration
	handshakeConfig goplugin.HandshakeConfig
	pluginMap       map[string]goplugin.Plugin
}

// LoadedAIStudioPlugin represents a loaded AI Studio plugin
type LoadedAIStudioPlugin struct {
	ID              uint
	Name            string
	PluginCategory  string // Human-readable category (e.g., "UI Extension", "Agent Plugin")
	Command         string
	IsOCI           bool
	Client          *goplugin.Client
	GRPCClient      pb.PluginServiceClient
	ServiceProvider plugin_services.AIStudioServiceProvider // Injected service provider
	LoadTime        time.Time
	IsHealthy       bool
	LastPing        time.Time
}

// NewAIStudioPluginManager creates a new AI Studio plugin manager
func NewAIStudioPluginManager(db *gorm.DB, ociClient *ociplugins.OCIPluginClient) *AIStudioPluginManager {
	return &AIStudioPluginManager{
		db:              db,
		ociClient:       ociClient,
		manifestService: nil, // Will be set later to avoid circular dependency
		loadedPlugins:   make(map[uint]*LoadedAIStudioPlugin),
		pluginClients:   make(map[uint]*goplugin.Client),
		handshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "AI_STUDIO_PLUGIN",
			MagicCookieValue: "v1",
		},
		pluginMap: map[string]goplugin.Plugin{
			"plugin": &AIStudioPluginGRPC{},
		},
	}
}

// SetManifestService sets the manifest service (to avoid circular dependency)
func (m *AIStudioPluginManager) SetManifestService(manifestService *PluginManifestService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manifestService = manifestService
}

// SetService sets the main service reference (to avoid circular dependency)
func (m *AIStudioPluginManager) SetService(service *Service) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.service = service
}

// AIStudioPluginGRPC implements the goplugin.Plugin interface for gRPC
type AIStudioPluginGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
}

func (p *AIStudioPluginGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// This method is not used on the host side - it's for plugin implementation
	return nil
}

func (p *AIStudioPluginGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	// Return client wrapper that stores broker for host-side service setup
	return &AIStudioPluginClient{
		broker:     broker,
		pluginStub: pb.NewPluginServiceClient(c),
		service:    globalServiceReference,
	}, nil
}

// AIStudioPluginClient wraps the plugin client with broker access for host service setup
type AIStudioPluginClient struct {
	broker     *goplugin.GRPCBroker
	pluginStub pb.PluginServiceClient
	service    *Service // Reference to AI Studio service for brokered servers
}

// SetupServiceBroker creates a long-lived brokered server for AI Studio services
// Returns the broker ID that the plugin can use to dial back to host services
func (c *AIStudioPluginClient) SetupServiceBroker() (uint32, error) {
	if c.broker == nil || c.service == nil {
		return 0, fmt.Errorf("broker or service not available")
	}

	// Allocate broker ID and start brokered server
	brokerID := c.broker.NextId()

	log.Info().
		Uint32("broker_id", brokerID).
		Msg("Setting up long-lived brokered server for AI Studio service API access")

	// Start brokered server with AI Studio management services
	go c.broker.AcceptAndServe(brokerID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)

		// Register AI Studio management services on brokered server
		// Use factory function to avoid circular import (set by grpc package)
		if NewAIStudioManagementServerFunc != nil {
			aiStudioServer := NewAIStudioManagementServerFunc(c.service)
			if serverImpl, ok := aiStudioServer.(mgmtpb.AIStudioManagementServiceServer); ok {
				mgmtpb.RegisterAIStudioManagementServiceServer(s, serverImpl)
				log.Info().
					Uint32("broker_id", brokerID).
					Msg("✅ AI Studio management services registered on long-lived brokered server")
			}
		} else {
			log.Error().Msg("NewAIStudioManagementServerFunc not set - cannot create service server")
		}

		return s
	})

	return brokerID, nil
}

// Delegate all PluginServiceClient methods to the plugin stub (with correct signatures)
func (c *AIStudioPluginClient) Initialize(ctx context.Context, req *pb.InitRequest, opts ...grpc.CallOption) (*pb.InitResponse, error) {
	return c.pluginStub.Initialize(ctx, req, opts...)
}

func (c *AIStudioPluginClient) Ping(ctx context.Context, req *pb.PingRequest, opts ...grpc.CallOption) (*pb.PingResponse, error) {
	return c.pluginStub.Ping(ctx, req, opts...)
}

func (c *AIStudioPluginClient) Shutdown(ctx context.Context, req *pb.ShutdownRequest, opts ...grpc.CallOption) (*pb.ShutdownResponse, error) {
	return c.pluginStub.Shutdown(ctx, req, opts...)
}

func (c *AIStudioPluginClient) Call(ctx context.Context, req *pb.CallRequest, opts ...grpc.CallOption) (*pb.CallResponse, error) {
	return c.pluginStub.Call(ctx, req, opts...)
}

func (c *AIStudioPluginClient) GetAsset(ctx context.Context, req *pb.GetAssetRequest, opts ...grpc.CallOption) (*pb.GetAssetResponse, error) {
	return c.pluginStub.GetAsset(ctx, req, opts...)
}

func (c *AIStudioPluginClient) GetManifest(ctx context.Context, req *pb.GetManifestRequest, opts ...grpc.CallOption) (*pb.GetManifestResponse, error) {
	return c.pluginStub.GetManifest(ctx, req, opts...)
}

func (c *AIStudioPluginClient) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest, opts ...grpc.CallOption) (*pb.GetConfigSchemaResponse, error) {
	return c.pluginStub.GetConfigSchema(ctx, req, opts...)
}

func (c *AIStudioPluginClient) ProcessPreAuth(ctx context.Context, req *pb.PluginRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return c.pluginStub.ProcessPreAuth(ctx, req, opts...)
}

func (c *AIStudioPluginClient) Authenticate(ctx context.Context, req *pb.AuthRequest, opts ...grpc.CallOption) (*pb.AuthResponse, error) {
	return c.pluginStub.Authenticate(ctx, req, opts...)
}

func (c *AIStudioPluginClient) GetAppByCredential(ctx context.Context, req *pb.GetAppRequest, opts ...grpc.CallOption) (*pb.GetAppResponse, error) {
	return c.pluginStub.GetAppByCredential(ctx, req, opts...)
}

func (c *AIStudioPluginClient) GetUserByCredential(ctx context.Context, req *pb.GetUserRequest, opts ...grpc.CallOption) (*pb.GetUserResponse, error) {
	return c.pluginStub.GetUserByCredential(ctx, req, opts...)
}

func (c *AIStudioPluginClient) ProcessPostAuth(ctx context.Context, req *pb.EnrichedRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return c.pluginStub.ProcessPostAuth(ctx, req, opts...)
}

func (c *AIStudioPluginClient) OnBeforeWriteHeaders(ctx context.Context, req *pb.HeadersRequest, opts ...grpc.CallOption) (*pb.HeadersResponse, error) {
	return c.pluginStub.OnBeforeWriteHeaders(ctx, req, opts...)
}

func (c *AIStudioPluginClient) OnBeforeWrite(ctx context.Context, req *pb.ResponseWriteRequest, opts ...grpc.CallOption) (*pb.ResponseWriteResponse, error) {
	return c.pluginStub.OnBeforeWrite(ctx, req, opts...)
}

func (c *AIStudioPluginClient) HandleProxyLog(ctx context.Context, req *pb.ProxyLogRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return c.pluginStub.HandleProxyLog(ctx, req, opts...)
}

func (c *AIStudioPluginClient) HandleAnalytics(ctx context.Context, req *pb.AnalyticsRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return c.pluginStub.HandleAnalytics(ctx, req, opts...)
}

func (c *AIStudioPluginClient) HandleBudgetUsage(ctx context.Context, req *pb.BudgetUsageRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return c.pluginStub.HandleBudgetUsage(ctx, req, opts...)
}

func (c *AIStudioPluginClient) ListAssets(ctx context.Context, req *pb.ListAssetsRequest, opts ...grpc.CallOption) (*pb.ListAssetsResponse, error) {
	return c.pluginStub.ListAssets(ctx, req, opts...)
}

func (c *AIStudioPluginClient) HandleAgentMessage(ctx context.Context, req *pb.AgentMessageRequest, opts ...grpc.CallOption) (pb.PluginService_HandleAgentMessageClient, error) {
	return c.pluginStub.HandleAgentMessage(ctx, req, opts...)
}

// ConfigOnlyGRPC implements goplugin.Plugin interface for config-only extraction
// Uses a universal handshake that works with any plugin type
type ConfigOnlyGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
}

func (p *ConfigOnlyGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// This is implemented by the plugin binary, not the host
	return nil
}

func (p *ConfigOnlyGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return configpb.NewConfigProviderServiceClient(c), nil
}

// LoadPlugin loads an AI Studio plugin by ID
func (m *AIStudioPluginManager) LoadPlugin(pluginID uint) (*LoadedAIStudioPlugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already loaded
	if loadedPlugin, exists := m.loadedPlugins[pluginID]; exists {
		return loadedPlugin, nil
	}

	// Get plugin from database
	var plugin models.Plugin
	if err := m.db.First(&plugin, pluginID).Error; err != nil {
		return nil, fmt.Errorf("plugin not found: %w", err)
	}

	// With unified handshake, we can load any plugin type
	// The plugin's hook_type determines its behavior
	log.Debug().
		Uint("plugin_id", pluginID).
		Str("plugin_category", plugin.GetCapabilityCategory()).
		Str("hook_type", plugin.HookType).
		Strs("all_hooks", plugin.GetAllHookTypes()).
		Msg("Loading plugin with unified handshake")

	if !plugin.IsActive {
		return nil, fmt.Errorf("plugin %d is not active", pluginID)
	}

	// Create plugin client based on command type
	client, err := m.createPluginClient(plugin.Command)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin client: %w", err)
	}

	// Connect to plugin
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// Note: Broker server setup will happen when needed, not during plugin loading
	// The host will set up brokered servers for specific service calls
	// and pass broker IDs to the plugin via request parameters

	// Get gRPC client
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	// Get plugin client wrapper from dispense
	clientWrapper, ok := raw.(*AIStudioPluginClient)
	if !ok {
		log.Fatal().
			Interface("received_type", raw).
			Str("expected_type", "*AIStudioPluginClient").
			Msg("FATAL: Plugin dispense type mismatch! This is the source of the plugin loading failure.")
	}

	// Set service reference in client wrapper for per-request broker setup
	clientWrapper.service = m.service

	log.Info().
		Uint("plugin_id", plugin.ID).
		Str("plugin_name", plugin.Name).
		Bool("has_broker", clientWrapper.broker != nil).
		Bool("has_service", clientWrapper.service != nil).
		Msg("✅ Plugin client wrapper connected with broker access")

	// Initialize plugin with config
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Convert config values to strings for gRPC transport
	// For complex types (arrays, objects), JSON-encode them so plugins can parse them
	configMap := make(map[string]string)
	for k, v := range plugin.Config {
		switch val := v.(type) {
		case string:
			configMap[k] = val
		case int, int64, uint, uint64, float64, bool:
			configMap[k] = fmt.Sprintf("%v", val)
		default:
			// Complex types (arrays, maps) - JSON encode
			jsonBytes, err := json.Marshal(val)
			if err != nil {
				log.Warn().
					Str("key", k).
					Err(err).
					Msg("Failed to JSON encode config value, using string representation")
				configMap[k] = fmt.Sprintf("%v", val)
			} else {
				configMap[k] = string(jsonBytes)
			}
		}
	}

	// Add plugin ID to config (broker ID will be passed per-request)
	configMap["plugin_id"] = fmt.Sprintf("%d", plugin.ID)
	configMap["has_service_api"] = "true"

	initResp, err := clientWrapper.Initialize(ctx, &pb.InitRequest{
		Config: configMap,
	})
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to initialize plugin: %w", err)
	}

	if !initResp.Success {
		client.Kill()
		return nil, fmt.Errorf("plugin initialization failed: %s", initResp.ErrorMessage)
	}

	// Create service provider for this plugin (if main service is available)
	var serviceProvider plugin_services.AIStudioServiceProvider
	if m.service != nil {
		// Create working service provider adapter
		serviceProvider = plugin_services.NewWorkingServiceProviderAdapter(m.service, plugin.ID)
		log.Info().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Msg("✅ Created working service provider for plugin - real analytics available")
	} else {
		log.Warn().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Msg("No service reference available - plugin will not have service access")
	}

	// Create loaded plugin record
	loadedPlugin := &LoadedAIStudioPlugin{
		ID:              plugin.ID,
		Name:            plugin.Name,
		PluginCategory:  plugin.GetCapabilityCategory(), // Use computed category
		Command:         plugin.Command,
		IsOCI:           plugin.IsOCIPlugin(),
		Client:          client,
		GRPCClient:      clientWrapper, // Use client wrapper with broker access
		ServiceProvider: serviceProvider,
		LoadTime:        time.Now(),
		IsHealthy:       true,
		LastPing:        time.Now(),
	}

	// Store in manager
	m.loadedPlugins[pluginID] = loadedPlugin
	m.pluginClients[pluginID] = client

	// Inject service provider into plugin if it supports it
	if serviceProvider != nil {
		err = m.injectServiceProvider(loadedPlugin)
		if err != nil {
			log.Warn().Err(err).
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Msg("Failed to inject service provider into plugin")
		} else {
			log.Info().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Msg("✅ Service provider injected into plugin successfully")
		}
	}

	log.Info().
		Uint("plugin_id", pluginID).
		Str("plugin_name", plugin.Name).
		Str("command", plugin.Command).
		Bool("is_oci", plugin.IsOCIPlugin()).
		Bool("has_service_provider", serviceProvider != nil).
		Msg("AI Studio plugin loaded successfully")

	// Auto-fetch and register manifest (new streamlined workflow)
	go func() {
		log.Info().
			Uint("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Msg("Auto-fetching manifest for loaded AI Studio plugin")

		// Give plugin a moment to fully initialize
		time.Sleep(1 * time.Second)

		manifestJSON, manifestErr := m.GetPluginManifest(pluginID)
		if manifestErr != nil {
			log.Warn().
				Uint("plugin_id", pluginID).
				Str("plugin_name", plugin.Name).
				Err(manifestErr).
				Msg("Failed to auto-fetch manifest - manual parse may be needed")
			return
		}

		// Parse manifest
		manifest := &models.PluginManifest{}
		if parseErr := json.Unmarshal([]byte(manifestJSON), manifest); parseErr != nil {
			log.Warn().
				Uint("plugin_id", pluginID).
				Str("plugin_name", plugin.Name).
				Err(parseErr).
				Msg("Failed to parse auto-fetched manifest")
			return
		}

		// Register UI components via manifest service
		// Note: We need to get plugin again since we're in a goroutine
		var pluginForUI models.Plugin
		if err := m.db.First(&pluginForUI, pluginID).Error; err != nil {
			log.Warn().
				Uint("plugin_id", pluginID).
				Err(err).
				Msg("Failed to get plugin for UI registration")
			return
		}

		// Register UI automatically if manifest service is available
		if m.manifestService != nil {
			if registerErr := m.manifestService.RegisterPluginUI(&pluginForUI, manifest); registerErr != nil {
				log.Warn().
					Uint("plugin_id", pluginID).
					Str("plugin_name", plugin.Name).
					Err(registerErr).
					Msg("Failed to auto-register UI components")
				return
			}

			log.Info().
				Uint("plugin_id", pluginID).
				Str("plugin_name", plugin.Name).
				Str("manifest_id", manifest.ID).
				Str("manifest_version", manifest.Version).
				Msg("✅ Auto-fetched manifest and registered UI components")
		} else {
			log.Info().
				Uint("plugin_id", pluginID).
				Str("plugin_name", plugin.Name).
				Str("manifest_id", manifest.ID).
				Str("manifest_version", manifest.Version).
				Msg("Auto-fetched manifest successfully - manifest service not available for UI registration")
		}
	}()

	return loadedPlugin, nil
}

// injectServiceProvider injects the service provider into a plugin after loading
func (m *AIStudioPluginManager) injectServiceProvider(loadedPlugin *LoadedAIStudioPlugin) error {
	if loadedPlugin.ServiceProvider == nil {
		log.Warn().
			Uint("plugin_id", loadedPlugin.ID).
			Str("plugin_name", loadedPlugin.Name).
			Msg("No service provider to inject - plugin will use fallback data")
		return nil
	}


	log.Info().
		Uint("plugin_id", loadedPlugin.ID).
		Str("plugin_name", loadedPlugin.Name).
		Msg("✅ Service provider injected and available for plugin access")

	return nil
}

// GetServiceProvider returns the service provider for a loaded plugin
func (m *AIStudioPluginManager) GetServiceProvider(pluginID uint) (plugin_services.AIStudioServiceProvider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if loadedPlugin, exists := m.loadedPlugins[pluginID]; exists {
		return loadedPlugin.ServiceProvider, loadedPlugin.ServiceProvider != nil
	}

	return nil, false
}

// UnloadPlugin unloads an AI Studio plugin
func (m *AIStudioPluginManager) UnloadPlugin(pluginID uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	loadedPlugin, exists := m.loadedPlugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	// Shutdown plugin gracefully
	if loadedPlugin.GRPCClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := loadedPlugin.GRPCClient.Shutdown(ctx, &pb.ShutdownRequest{
			TimeoutSeconds: 5,
		})
		if err != nil {
			log.Warn().
				Uint("plugin_id", pluginID).
				Err(err).
				Msg("Failed to shutdown plugin gracefully")
		}
	}

	// Kill plugin process
	if loadedPlugin.Client != nil {
		loadedPlugin.Client.Kill()
	}

	// Remove from maps
	delete(m.loadedPlugins, pluginID)
	delete(m.pluginClients, pluginID)

	log.Info().
		Uint("plugin_id", pluginID).
		Str("plugin_name", loadedPlugin.Name).
		Msg("AI Studio plugin unloaded successfully")

	return nil
}

// GetPluginAsset retrieves an asset from a loaded plugin via gRPC
func (m *AIStudioPluginManager) GetPluginAsset(pluginID uint, assetPath string) ([]byte, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	loadedPlugin, exists := m.loadedPlugins[pluginID]
	if !exists {
		return nil, "", fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	if !loadedPlugin.IsHealthy {
		return nil, "", fmt.Errorf("plugin %d is not healthy", pluginID)
	}

	// Call plugin's GetAsset gRPC method
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := loadedPlugin.GRPCClient.GetAsset(ctx, &pb.GetAssetRequest{
		AssetPath: assetPath,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get asset from plugin: %w", err)
	}

	if !resp.Success {
		return nil, "", fmt.Errorf("plugin asset request failed: %s", resp.ErrorMessage)
	}

	return resp.Content, resp.MimeType, nil
}

// GetPluginManifest retrieves the manifest from a loaded plugin via gRPC
func (m *AIStudioPluginManager) GetPluginManifest(pluginID uint) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	loadedPlugin, exists := m.loadedPlugins[pluginID]
	if !exists {
		return "", fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	if !loadedPlugin.IsHealthy {
		return "", fmt.Errorf("plugin %d is not healthy", pluginID)
	}

	// Call plugin's GetManifest gRPC method
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := loadedPlugin.GRPCClient.GetManifest(ctx, &pb.GetManifestRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to get manifest from plugin: %w", err)
	}

	if !resp.Success {
		return "", fmt.Errorf("plugin manifest request failed: %s", resp.ErrorMessage)
	}

	return resp.ManifestJson, nil
}

// ListPluginAssets lists all assets available from a plugin
func (m *AIStudioPluginManager) ListPluginAssets(pluginID uint, pathPrefix string) ([]AssetInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	loadedPlugin, exists := m.loadedPlugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	if !loadedPlugin.IsHealthy {
		return nil, fmt.Errorf("plugin %d is not healthy", pluginID)
	}

	// Call plugin's ListAssets gRPC method
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := loadedPlugin.GRPCClient.ListAssets(ctx, &pb.ListAssetsRequest{
		PathPrefix: pathPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list assets from plugin: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("plugin assets list request failed: %s", resp.ErrorMessage)
	}

	// Convert protobuf response to local type
	assets := make([]AssetInfo, len(resp.Assets))
	for i, asset := range resp.Assets {
		assets[i] = AssetInfo{
			Path:     asset.Path,
			MimeType: asset.MimeType,
			Size:     asset.Size,
		}
	}

	return assets, nil
}

// CallPluginRPC calls a plugin's RPC method via gRPC
func (m *AIStudioPluginManager) CallPluginRPC(pluginID uint, method string, payload map[string]interface{}) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	loadedPlugin, exists := m.loadedPlugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	if !loadedPlugin.IsHealthy {
		return nil, fmt.Errorf("plugin %d is not healthy", pluginID)
	}

	// Convert payload to JSON string
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RPC payload: %w", err)
	}

	// Set up per-request broker for service API access (if plugin needs it)
	var serviceBrokerID uint32
	if clientWrapper, ok := loadedPlugin.GRPCClient.(*AIStudioPluginClient); ok {
		if clientWrapper.broker != nil && clientWrapper.service != nil {
			// Set up per-request brokered server for this call
			brokerID := clientWrapper.broker.NextId()

			log.Info().
				Uint("plugin_id", pluginID).
				Str("method", method).
				Uint32("broker_id", brokerID).
				Msg("Setting up per-request broker for service API access")

			// Start brokered server for this request
			// Use a channel to ensure server is ready before proceeding
			serverReady := make(chan struct{})
			go func() {
				clientWrapper.broker.AcceptAndServe(brokerID, func(opts []grpc.ServerOption) *grpc.Server {
					// Inject plugin ID into context for all requests on this brokered server
					pluginIDInterceptor := CreatePluginIDInterceptor(pluginID)
					opts = append(opts, grpc.UnaryInterceptor(pluginIDInterceptor))

					s := grpc.NewServer(opts...)

					// Register AI Studio management services with full implementation
					// Use factory function to avoid circular import (set by grpc package)
					if NewAIStudioManagementServerFunc != nil {
						aiStudioServer := NewAIStudioManagementServerFunc(clientWrapper.service)
						if serverImpl, ok := aiStudioServer.(mgmtpb.AIStudioManagementServiceServer); ok {
							mgmtpb.RegisterAIStudioManagementServiceServer(s, serverImpl)
							log.Debug().
								Uint32("broker_id", brokerID).
								Uint("plugin_id", pluginID).
								Msg("AI Studio services registered on per-request brokered server with plugin ID context")
						}
					} else {
						log.Error().Msg("NewAIStudioManagementServerFunc not set - cannot create service server")
					}

					// Signal that server is ready
					close(serverReady)

					return s
				})
			}()

			// Wait for server to be ready before proceeding (with timeout)
			select {
			case <-serverReady:
				log.Debug().Uint32("broker_id", brokerID).Msg("Brokered server ready for plugin calls")
			case <-time.After(100 * time.Millisecond):
				log.Warn().Uint32("broker_id", brokerID).Msg("Brokered server setup timeout - proceeding anyway")
			}

			serviceBrokerID = brokerID
		}
	}

	// Add broker ID to payload if available
	if serviceBrokerID != 0 {
		// Add broker ID to the payload so plugin can access it
		if payload == nil {
			payload = make(map[string]interface{})
		}
		payload["_service_broker_id"] = serviceBrokerID

		// Re-marshal payload with broker ID
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal RPC payload with broker ID: %w", err)
		}
	}

	// Call plugin's Call gRPC method
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Info().
		Uint("plugin_id", pluginID).
		Str("method", method).
		Uint32("service_broker_id", serviceBrokerID).
		Str("payload", string(payloadBytes)).
		Msg("🔥 Calling plugin RPC with broker ID")

	resp, err := loadedPlugin.GRPCClient.Call(ctx, &pb.CallRequest{
		Method:           method,
		Payload:          string(payloadBytes),
		ServiceBrokerId:  serviceBrokerID,
	})

	log.Info().
		Uint("plugin_id", pluginID).
		Str("method", method).
		Bool("success", resp != nil && resp.Success).
		Str("error_message", func() string { if resp != nil { return resp.ErrorMessage } else { return "" } }()).
		Err(err).
		Msg("🔥 Plugin RPC returned")

	if err != nil {
		return nil, fmt.Errorf("failed to call plugin RPC method: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("plugin RPC call failed: %s", resp.ErrorMessage)
	}

	// Parse response data as JSON
	var responseData interface{}
	if err := json.Unmarshal([]byte(resp.Data), &responseData); err != nil {
		// If not valid JSON, return as string
		return resp.Data, nil
	}

	return responseData, nil
}

// createPluginClient creates a plugin client based on command scheme (adapted from microgateway)
func (m *AIStudioPluginManager) createPluginClient(command string) (*goplugin.Client, error) {
	if strings.HasPrefix(command, "oci://") {
		// OCI plugin - fetch from registry first
		return m.createOCIPluginClient(command)
	} else if strings.HasPrefix(command, "grpc://") {
		// External gRPC service - use ReattachConfig (for testing)
		return m.createGRPCPluginClient(command)
	} else {
		// Local executable - use exec.Command
		return m.createLocalPluginClient(command)
	}
}

// createOCIPluginClient fetches an OCI plugin and creates a client
func (m *AIStudioPluginManager) createOCIPluginClient(command string) (*goplugin.Client, error) {
	if m.ociClient == nil {
		return nil, fmt.Errorf("OCI client not configured")
	}

	// Parse OCI reference
	ref, params, err := ociplugins.ParseOCICommand(command)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI command: %w", err)
	}

	// Get or fetch plugin
	localPlugin, err := m.ociClient.GetPlugin(ref, params)
	if err != nil {
		// Try to fetch if not cached
		ctx := context.Background()
		localPlugin, err = m.ociClient.FetchPlugin(ctx, ref, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch OCI plugin: %w", err)
		}
	}

	log.Info().
		Str("command", command).
		Str("executable_path", localPlugin.ExecutablePath).
		Bool("verified", localPlugin.Verified).
		Msg("Using OCI plugin binary")

	// Create plugin client with the local executable
	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  m.handshakeConfig,
		Plugins:          m.pluginMap,
		Cmd:              exec.Command(localPlugin.ExecutablePath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           logger.NewHCLogAdapter("plugin"),
	}), nil
}

// createLocalPluginClient creates a client for a local executable plugin
func (m *AIStudioPluginManager) createLocalPluginClient(command string) (*goplugin.Client, error) {
	cmdPath := command
	if strings.HasPrefix(command, "file://") {
		cmdPath = strings.TrimPrefix(command, "file://")
	}

	log.Info().
		Str("command", command).
		Str("path", cmdPath).
		Msg("Creating client for local plugin executable")

	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  m.handshakeConfig,
		Plugins:          m.pluginMap,
		Cmd:              exec.Command(cmdPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           logger.NewHCLogAdapter("plugin"),
	}), nil
}

// createGRPCPluginClient creates a client for an external gRPC plugin (for testing)
func (m *AIStudioPluginManager) createGRPCPluginClient(command string) (*goplugin.Client, error) {
	// Parse gRPC URL: grpc://host:port
	address := strings.TrimPrefix(command, "grpc://")

	log.Info().
		Str("command", command).
		Str("address", address).
		Msg("Creating client for external gRPC plugin")

	// For external gRPC, we would need to implement ReattachConfig
	// For MVP, return error to encourage using local binaries
	return nil, fmt.Errorf("external gRPC plugins not supported in MVP - use local binary or OCI")
}

// createConfigOnlyPluginClient creates a plugin client specifically for config extraction
// Uses universal handshake and config-only service
func (m *AIStudioPluginManager) createConfigOnlyPluginClient(command string) (*goplugin.Client, error) {
	if strings.HasPrefix(command, "oci://") {
		// OCI plugin - fetch from registry first, then create config-only client
		return m.createConfigOnlyOCIPluginClient(command)
	} else {
		// Local executable - create config-only client
		return m.createConfigOnlyLocalPluginClient(command)
	}
}

// createConfigOnlyOCIPluginClient creates a config-only client for OCI plugins
func (m *AIStudioPluginManager) createConfigOnlyOCIPluginClient(command string) (*goplugin.Client, error) {
	if m.ociClient == nil {
		return nil, fmt.Errorf("OCI client not configured")
	}

	// Parse OCI reference
	ref, params, err := ociplugins.ParseOCICommand(command)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI command: %w", err)
	}

	// Get or fetch plugin
	localPlugin, err := m.ociClient.GetPlugin(ref, params)
	if err != nil {
		// Try to fetch if not cached
		ctx := context.Background()
		localPlugin, err = m.ociClient.FetchPlugin(ctx, ref, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch OCI plugin: %w", err)
		}
	}

	log.Debug().
		Str("command", command).
		Str("executable_path", localPlugin.ExecutablePath).
		Bool("verified", localPlugin.Verified).
		Msg("Using OCI plugin binary for config-only access")

	// Create plugin client with unified AI_STUDIO_PLUGIN handshake
	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.handshakeConfig, // Use unified AI_STUDIO_PLUGIN handshake
		Plugins: map[string]goplugin.Plugin{
			"config": &ConfigOnlyGRPC{},
		},
		Cmd:              exec.Command(localPlugin.ExecutablePath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           logger.NewHCLogAdapter("plugin"),
	}), nil
}

// createConfigOnlyLocalPluginClient creates a config-only client for local plugins
func (m *AIStudioPluginManager) createConfigOnlyLocalPluginClient(command string) (*goplugin.Client, error) {
	cmdPath := command
	if strings.HasPrefix(command, "file://") {
		cmdPath = strings.TrimPrefix(command, "file://")
	}

	log.Debug().
		Str("command", command).
		Str("path", cmdPath).
		Msg("Creating config-only client for local plugin executable")

	// Create plugin client with unified AI_STUDIO_PLUGIN handshake
	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.handshakeConfig, // Use unified AI_STUDIO_PLUGIN handshake
		Plugins: map[string]goplugin.Plugin{
			"config": &ConfigOnlyGRPC{},
		},
		Cmd:              exec.Command(cmdPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           logger.NewHCLogAdapter("plugin"),
	}), nil
}

// GetLoadedPlugin returns a loaded plugin by ID
func (m *AIStudioPluginManager) GetLoadedPlugin(pluginID uint) (*LoadedAIStudioPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	loadedPlugin, exists := m.loadedPlugins[pluginID]
	return loadedPlugin, exists
}

// IsPluginLoaded checks if a plugin is currently loaded
func (m *AIStudioPluginManager) IsPluginLoaded(pluginID uint) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.loadedPlugins[pluginID]
	return exists
}

// PingPlugin performs a health check on a loaded plugin
func (m *AIStudioPluginManager) PingPlugin(pluginID uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	loadedPlugin, exists := m.loadedPlugins[pluginID]
	if !exists {
		return fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := loadedPlugin.GRPCClient.Ping(ctx, &pb.PingRequest{
		Timestamp: time.Now().Unix(),
	})

	if err != nil {
		loadedPlugin.IsHealthy = false
		return fmt.Errorf("plugin ping failed: %w", err)
	}

	loadedPlugin.IsHealthy = resp.Healthy
	loadedPlugin.LastPing = time.Now()

	log.Debug().
		Uint("plugin_id", pluginID).
		Bool("healthy", resp.Healthy).
		Msg("Plugin ping completed")

	return nil
}

// LoadAllUIAndAgentPlugins loads all active plugins that support studio_ui or agent hooks
func (m *AIStudioPluginManager) LoadAllUIAndAgentPlugins() error {
	// Get all active plugins
	var plugins []models.Plugin
	err := m.db.Where("is_active = ?", true).Find(&plugins).Error
	if err != nil {
		return fmt.Errorf("failed to get plugins: %w", err)
	}

	log.Info().Int("total_plugins", len(plugins)).Msg("Checking plugins for UI/Agent support")

	var loadErrors []string
	loadedCount := 0

	for _, plugin := range plugins {
		// Check if plugin supports studio_ui or agent hooks
		supportsUI := plugin.SupportsHookType(models.HookTypeStudioUI)
		supportsAgent := plugin.SupportsHookType(models.HookTypeAgent)

		if !supportsUI && !supportsAgent {
			log.Debug().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Strs("hooks", plugin.GetAllHookTypes()).
				Msg("Plugin does not support UI or Agent hooks, skipping")
			continue
		}

		// Attempt to load
		_, err := m.LoadPlugin(plugin.ID)
		if err != nil {
			log.Error().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Err(err).
				Msg("Failed to load UI/Agent plugin")
			loadErrors = append(loadErrors, fmt.Sprintf("Plugin %s (ID %d): %v", plugin.Name, plugin.ID, err))
		} else {
			loadedCount++
			log.Info().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Strs("hooks", plugin.GetAllHookTypes()).
				Msg("Successfully loaded UI/Agent plugin")
		}
	}

	log.Info().
		Int("loaded", loadedCount).
		Int("failed", len(loadErrors)).
		Msg("Completed UI/Agent plugin loading")

	if len(loadErrors) > 0 {
		return fmt.Errorf("failed to load some plugins: %s", strings.Join(loadErrors, "; "))
	}

	return nil
}

// Shutdown gracefully shuts down all loaded plugins
func (m *AIStudioPluginManager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Info().Msg("Shutting down AI Studio plugin manager")

	for pluginID := range m.loadedPlugins {
		if err := m.unloadPluginUnsafe(pluginID); err != nil {
			log.Error().
				Uint("plugin_id", pluginID).
				Err(err).
				Msg("Failed to unload plugin during shutdown")
		}
	}

	return nil
}

// unloadPluginUnsafe unloads a plugin without locking (assumes lock is held)
func (m *AIStudioPluginManager) unloadPluginUnsafe(pluginID uint) error {
	loadedPlugin, exists := m.loadedPlugins[pluginID]
	if !exists {
		return nil
	}

	// Shutdown plugin gracefully
	if loadedPlugin.GRPCClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		loadedPlugin.GRPCClient.Shutdown(ctx, &pb.ShutdownRequest{
			TimeoutSeconds: 3,
		})
	}

	// Kill plugin process
	if loadedPlugin.Client != nil {
		loadedPlugin.Client.Kill()
	}

	// Remove from maps
	delete(m.loadedPlugins, pluginID)
	delete(m.pluginClients, pluginID)

	return nil
}

// ConfigOnlyPlugin represents a plugin loaded only for configuration schema extraction
// Uses the isolated ConfigProviderService instead of the main PluginService
type ConfigOnlyPlugin struct {
	Command          string
	Client           *goplugin.Client
	ConfigGRPCClient configpb.ConfigProviderServiceClient
	LoadTime         time.Time
}

// LoadPluginForConfigOnly loads a plugin with minimal resources for schema extraction
// Uses universal config provider handshake (AI_STUDIO_PLUGIN + "config" service)
// All plugins now use the unified handshake, eliminating the need for fallback logic
func (m *AIStudioPluginManager) LoadPluginForConfigOnly(ctx context.Context, command string) (ConfigProvider, error) {
	log.Debug().Str("command", command).Msg("Loading plugin for config-only access with unified handshake")

	// Use universal config handshake - all plugins now support this via unified SDK
	return m.loadPluginWithConfigHandshake(ctx, command)
}

// loadPluginWithConfigHandshake loads plugin using universal AI_STUDIO_PLUGIN handshake + config service
func (m *AIStudioPluginManager) loadPluginWithConfigHandshake(ctx context.Context, command string) (ConfigProvider, error) {
	// Create plugin client with universal config-only handshake and service
	client, err := m.createConfigOnlyPluginClient(command)
	if err != nil {
		return nil, fmt.Errorf("failed to create config-only plugin client: %w", err)
	}

	// Connect to plugin
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// Dispense the config-only service
	raw, err := rpcClient.Dispense("config")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense config provider service: %w", err)
	}

	configGRPCClient, ok := raw.(configpb.ConfigProviderServiceClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin does not implement ConfigProviderServiceClient")
	}

	// Test the config service with a ping
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = configGRPCClient.Ping(pingCtx, &configpb.ConfigPingRequest{
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("config provider service ping failed: %w", err)
	}

	// Create config-only plugin instance
	configPlugin := &ConfigOnlyPlugin{
		Command:          command,
		Client:           client,
		ConfigGRPCClient: configGRPCClient,
		LoadTime:         time.Now(),
	}

	return configPlugin, nil
}

// UnloadConfigProvider releases resources used by a ConfigProvider
func (m *AIStudioPluginManager) UnloadConfigProvider(provider ConfigProvider) error {
	switch cp := provider.(type) {
	case *ConfigOnlyPlugin:
		// ConfigProviderService doesn't have shutdown - just kill
		if cp.Client != nil {
			cp.Client.Kill()
		}
		log.Debug().Str("command", cp.Command).Msg("Config-only plugin unloaded successfully")

	default:
		return fmt.Errorf("unknown config provider type")
	}

	return nil
}

// GetConfigSchema implements ConfigProvider interface for ConfigOnlyPlugin
func (cp *ConfigOnlyPlugin) GetConfigSchema(ctx context.Context) ([]byte, error) {
	// Call plugin's GetConfigSchema via the ConfigProviderService
	schemaCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := cp.ConfigGRPCClient.GetConfigSchema(schemaCtx, &configpb.ConfigSchemaRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get config schema from ConfigProviderService: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("config provider schema request failed: %s", resp.ErrorMessage)
	}

	return []byte(resp.SchemaJson), nil
}

// GetManifest implements EnhancedConfigProvider interface for ConfigOnlyPlugin
func (cp *ConfigOnlyPlugin) GetManifest(ctx context.Context) ([]byte, error) {
	// Call plugin's GetManifest via the ConfigProviderService
	manifestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := cp.ConfigGRPCClient.GetManifest(manifestCtx, &configpb.GetManifestRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest from ConfigProviderService: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("config provider manifest request failed: %s", resp.ErrorMessage)
	}

	return []byte(resp.ManifestJson), nil
}

// GetPluginConfigSchema retrieves config schema from a plugin by ID
func (m *AIStudioPluginManager) GetPluginConfigSchema(pluginID uint) (string, error) {
	// Get plugin from database
	var plugin models.Plugin
	if err := m.db.First(&plugin, pluginID).Error; err != nil {
		return "", fmt.Errorf("plugin not found: %w", err)
	}

	// With unified handshake, any plugin can provide a manifest
	if !plugin.IsActive {
		return "", fmt.Errorf("plugin %d is not active", pluginID)
	}

	// Load plugin for config-only access
	ctx := context.Background()
	configProvider, err := m.LoadPluginForConfigOnly(ctx, plugin.Command)
	if err != nil {
		return "", fmt.Errorf("failed to load plugin for config access: %w", err)
	}
	defer m.UnloadConfigProvider(configProvider)

	// Get schema
	schemaBytes, err := configProvider.GetConfigSchema(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get config schema: %w", err)
	}

	return string(schemaBytes), nil
}

// AssetInfo represents information about a plugin asset
type AssetInfo struct {
	Path     string `json:"path"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// DetectMimeType detects MIME type from file extension
func DetectMimeType(filename string) string {
	ext := filepath.Ext(filename)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		// Default MIME types for common plugin assets
		switch ext {
		case ".js":
			return "application/javascript"
		case ".css":
			return "text/css"
		case ".svg":
			return "image/svg+xml"
		case ".json":
			return "application/json"
		default:
			return "application/octet-stream"
		}
	}
	return mimeType
}