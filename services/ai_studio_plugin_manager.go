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

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/services/grpc"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

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
	Slug            string
	PluginType      string
	Command         string
	IsOCI           bool
	Client          *goplugin.Client
	GRPCClient      pb.PluginServiceClient
	ServiceProvider grpc.AIStudioServiceProvider // Injected service provider
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
	// This is implemented by the plugin binary, not the host
	return nil
}

func (p *AIStudioPluginGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
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

// ConfigOnlyHandshake - Universal handshake for configuration extraction
// This handshake is independent of plugin type and allows config extraction from any plugin
var ConfigOnlyHandshake = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "CONFIG_PROVIDER",
	MagicCookieValue: "v1",
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

	// Only load AI Studio plugins
	if !plugin.IsAIStudioPlugin() {
		return nil, fmt.Errorf("plugin %d is not an AI Studio plugin", pluginID)
	}

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

	// Get gRPC client
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	grpcClient, ok := raw.(pb.PluginServiceClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin does not implement PluginServiceClient")
	}

	// Initialize plugin with config
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configMap := make(map[string]string)
	for k, v := range plugin.Config {
		if str, ok := v.(string); ok {
			configMap[k] = str
		}
	}

	// Add plugin ID to config for service provider setup
	configMap["plugin_id"] = fmt.Sprintf("%d", plugin.ID)
	configMap["has_service_provider"] = "true"

	initResp, err := grpcClient.Initialize(ctx, &pb.InitRequest{
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
	var serviceProvider grpc.AIStudioServiceProvider
	if m.service != nil {
		serviceProvider = grpc.NewInProcessServiceProvider(m.service, plugin.ID)
		log.Info().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Msg("Created in-process service provider for plugin")
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
		Slug:            plugin.Slug,
		PluginType:      plugin.PluginType,
		Command:         plugin.Command,
		IsOCI:           plugin.IsOCIPlugin(),
		Client:          client,
		GRPCClient:      grpcClient,
		ServiceProvider: serviceProvider, // Inject service provider
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
	// For now, the service provider is stored in the LoadedAIStudioPlugin structure
	// In a future enhancement, we could add a gRPC method to inject it directly
	// but the current approach of storing it and having the plugin access it is sufficient

	// The plugin will access the service provider via a reference mechanism
	// For the initial implementation, plugins will need to implement ServiceProviderInjectable
	// and call GetServiceProvider() to access the injected services

	log.Debug().
		Uint("plugin_id", loadedPlugin.ID).
		Str("plugin_name", loadedPlugin.Name).
		Msg("Service provider available for plugin access")

	return nil
}

// GetServiceProvider returns the service provider for a loaded plugin
func (m *AIStudioPluginManager) GetServiceProvider(pluginID uint) (grpc.AIStudioServiceProvider, bool) {
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

	// Call plugin's Call gRPC method
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := loadedPlugin.GRPCClient.Call(ctx, &pb.CallRequest{
		Method:  method,
		Payload: string(payloadBytes),
	})
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

	// Create plugin client with universal config handshake
	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: ConfigOnlyHandshake,
		Plugins: map[string]goplugin.Plugin{
			"config": &ConfigOnlyGRPC{},
		},
		Cmd:              exec.Command(localPlugin.ExecutablePath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
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

	// Create plugin client with universal config handshake
	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: ConfigOnlyHandshake,
		Plugins: map[string]goplugin.Plugin{
			"config": &ConfigOnlyGRPC{},
		},
		Cmd:              exec.Command(cmdPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
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

// LoadAllAIStudioPlugins loads all active AI Studio plugins
func (m *AIStudioPluginManager) LoadAllAIStudioPlugins() error {
	// Get all active AI Studio plugins
	var plugins []models.Plugin
	err := m.db.Where("plugin_type = ? AND is_active = ?", models.PluginTypeAIStudio, true).
		Find(&plugins).Error
	if err != nil {
		return fmt.Errorf("failed to get AI Studio plugins: %w", err)
	}

	log.Info().Int("count", len(plugins)).Msg("Loading AI Studio plugins")

	var loadErrors []string
	for _, plugin := range plugins {
		if _, err := m.LoadPlugin(plugin.ID); err != nil {
			log.Error().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Err(err).
				Msg("Failed to load AI Studio plugin")
			loadErrors = append(loadErrors, fmt.Sprintf("Plugin %s: %v", plugin.Name, err))
		}
	}

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

// LegacyConfigOnlyPlugin represents a plugin loaded using its original handshake for config extraction
// Uses the main PluginService's GetConfigSchema method
type LegacyConfigOnlyPlugin struct {
	Command    string
	Client     *goplugin.Client
	GRPCClient pb.PluginServiceClient
	LoadTime   time.Time
}

// MicrogatewaPluginGRPC implements goplugin.Plugin interface for microgateway plugins
type MicrogatewaPluginGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
}

func (p *MicrogatewaPluginGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	// This is implemented by the plugin binary, not the host
	return nil
}

func (p *MicrogatewaPluginGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

// LoadPluginForConfigOnly loads a plugin with minimal resources for schema extraction
// First tries the isolated ConfigProviderService, then falls back to plugin-specific handshakes
func (m *AIStudioPluginManager) LoadPluginForConfigOnly(ctx context.Context, command string) (ConfigProvider, error) {
	log.Debug().Str("command", command).Msg("Loading plugin for config-only access")

	// Get plugin from database to determine its type and appropriate handshake
	var plugin models.Plugin
	err := m.db.Where("command = ?", command).First(&plugin).Error
	if err != nil {
		log.Warn().Err(err).Str("command", command).Msg("Could not determine plugin type from database, trying universal config handshake")
		// Fall back to universal config handshake
		return m.loadPluginWithConfigHandshake(ctx, command)
	}

	// Try plugin-specific handshake first (for existing plugins)
	configProvider, err := m.loadPluginWithOriginalHandshake(ctx, command, &plugin)
	if err == nil {
		return configProvider, nil
	}

	log.Debug().Err(err).Str("command", command).Msg("Failed with original handshake, trying universal config handshake")

	// Fall back to universal config handshake
	return m.loadPluginWithConfigHandshake(ctx, command)
}

// loadPluginWithOriginalHandshake loads plugin using its original handshake and main service
func (m *AIStudioPluginManager) loadPluginWithOriginalHandshake(ctx context.Context, command string, plugin *models.Plugin) (ConfigProvider, error) {
	// Get appropriate handshake for this plugin type
	var handshake goplugin.HandshakeConfig
	var pluginMap map[string]goplugin.Plugin

	switch plugin.PluginType {
	case models.PluginTypeGateway:
		// Microgateway plugin
		handshake = goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "MICROGATEWAY_PLUGIN",
			MagicCookieValue: "v1",
		}
		pluginMap = map[string]goplugin.Plugin{
			"plugin": &MicrogatewaPluginGRPC{},
		}
	case models.PluginTypeAIStudio:
		// AI Studio plugin
		handshake = m.handshakeConfig
		pluginMap = m.pluginMap
	default:
		return nil, fmt.Errorf("unknown plugin type: %s", plugin.PluginType)
	}

	// Create plugin client
	client, err := m.createPluginClientWithConfig(command, handshake, pluginMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin client: %w", err)
	}

	// Connect and dispense main plugin service
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense plugin service: %w", err)
	}

	grpcClient, ok := raw.(pb.PluginServiceClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin does not implement PluginServiceClient")
	}

	// Initialize plugin with minimal config
	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	initResp, err := grpcClient.Initialize(initCtx, &pb.InitRequest{
		Config: make(map[string]string),
	})
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to initialize plugin: %w", err)
	}

	if !initResp.Success {
		client.Kill()
		return nil, fmt.Errorf("plugin initialization failed: %s", initResp.ErrorMessage)
	}

	// Create legacy config plugin instance
	configPlugin := &LegacyConfigOnlyPlugin{
		Command:    command,
		Client:     client,
		GRPCClient: grpcClient,
		LoadTime:   time.Now(),
	}

	return configPlugin, nil
}

// loadPluginWithConfigHandshake loads plugin using universal CONFIG_PROVIDER handshake
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

// createPluginClientWithConfig creates a plugin client with specified handshake and plugin map
func (m *AIStudioPluginManager) createPluginClientWithConfig(command string, handshake goplugin.HandshakeConfig, pluginMap map[string]goplugin.Plugin) (*goplugin.Client, error) {
	if strings.HasPrefix(command, "oci://") {
		return m.createOCIPluginClientWithConfig(command, handshake, pluginMap)
	} else {
		return m.createLocalPluginClientWithConfig(command, handshake, pluginMap)
	}
}

// createOCIPluginClientWithConfig creates an OCI plugin client with custom handshake
func (m *AIStudioPluginManager) createOCIPluginClientWithConfig(command string, handshake goplugin.HandshakeConfig, pluginMap map[string]goplugin.Plugin) (*goplugin.Client, error) {
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

	// Create plugin client with custom configuration
	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  handshake,
		Plugins:          pluginMap,
		Cmd:              exec.Command(localPlugin.ExecutablePath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	}), nil
}

// createLocalPluginClientWithConfig creates a local plugin client with custom handshake
func (m *AIStudioPluginManager) createLocalPluginClientWithConfig(command string, handshake goplugin.HandshakeConfig, pluginMap map[string]goplugin.Plugin) (*goplugin.Client, error) {
	cmdPath := command
	if strings.HasPrefix(command, "file://") {
		cmdPath = strings.TrimPrefix(command, "file://")
	}

	// Create plugin client with custom configuration
	return goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  handshake,
		Plugins:          pluginMap,
		Cmd:              exec.Command(cmdPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	}), nil
}

// GetConfigSchema implements ConfigProvider interface for LegacyConfigOnlyPlugin
func (cp *LegacyConfigOnlyPlugin) GetConfigSchema(ctx context.Context) ([]byte, error) {
	// Call plugin's GetConfigSchema via the main PluginService
	schemaCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := cp.GRPCClient.GetConfigSchema(schemaCtx, &pb.GetConfigSchemaRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get config schema from main PluginService: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("plugin config schema request failed: %s", resp.ErrorMessage)
	}

	return []byte(resp.SchemaJson), nil
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

	case *LegacyConfigOnlyPlugin:
		// Legacy plugin using main service - try graceful shutdown first
		if cp.GRPCClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			_, err := cp.GRPCClient.Shutdown(ctx, &pb.ShutdownRequest{
				TimeoutSeconds: 2,
			})
			if err != nil {
				log.Warn().Str("command", cp.Command).Err(err).Msg("Failed to shutdown legacy config plugin gracefully")
			}
		}

		if cp.Client != nil {
			cp.Client.Kill()
		}
		log.Debug().Str("command", cp.Command).Msg("Legacy config-only plugin unloaded successfully")

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

// GetPluginConfigSchema retrieves config schema from a plugin by ID
func (m *AIStudioPluginManager) GetPluginConfigSchema(pluginID uint) (string, error) {
	// Get plugin from database
	var plugin models.Plugin
	if err := m.db.First(&plugin, pluginID).Error; err != nil {
		return "", fmt.Errorf("plugin not found: %w", err)
	}

	// Only load AI Studio plugins
	if !plugin.IsAIStudioPlugin() {
		return "", fmt.Errorf("plugin %d is not an AI Studio plugin", pluginID)
	}

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