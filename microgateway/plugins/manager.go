// plugins/manager.go
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
)

// LoadedPlugin represents a loaded plugin instance
type LoadedPlugin struct {
	ID          uint
	Name        string
	Slug        string
	HookType    interfaces.HookType
	Client      *plugin.Client
	GRPCClient  pb.PluginServiceClient
	Config      map[string]interface{}
	Checksum    string
	IsHealthy   bool
}

// PluginManager manages the lifecycle of plugins
type PluginManager struct {
	mu              sync.RWMutex
	loadedPlugins   map[uint]*LoadedPlugin     // Plugin ID -> loaded plugin
	llmPluginMap    map[uint][]uint             // LLM ID -> Plugin IDs (ordered)
	pluginClients   map[uint]*plugin.Client     // Plugin ID -> go-plugin client
	reattachConfigs map[uint]*plugin.ReattachConfig // For reconnection
	service         services.PluginServiceInterface      // Database service
	handshakeConfig plugin.HandshakeConfig
	pluginMap       map[string]plugin.Plugin
}

// HandshakeConfig is used to do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "MICROGATEWAY_PLUGIN",
	MagicCookieValue: "v1",
}

// NewPluginManager creates a new plugin manager instance
func NewPluginManager(pluginService services.PluginServiceInterface) *PluginManager {
	return &PluginManager{
		loadedPlugins:   make(map[uint]*LoadedPlugin),
		llmPluginMap:    make(map[uint][]uint),
		pluginClients:   make(map[uint]*plugin.Client),
		reattachConfigs: make(map[uint]*plugin.ReattachConfig),
		service:         pluginService,
		handshakeConfig: HandshakeConfig,
		pluginMap: map[string]plugin.Plugin{
			"plugin": &PluginGRPC{},
		},
	}
}

// LoadPlugin loads a plugin by ID
func (pm *PluginManager) LoadPlugin(pluginID uint) (*LoadedPlugin, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if plugin is already loaded
	if existingPlugin, exists := pm.loadedPlugins[pluginID]; exists {
		return existingPlugin, nil
	}

	// Get plugin from database
	pluginData, err := pm.service.GetPlugin(pluginID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin from database: %w", err)
	}

	if !pluginData.IsActive {
		return nil, fmt.Errorf("plugin %d is not active", pluginID)
	}

	// Start plugin process
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pm.handshakeConfig,
		Plugins:          pm.pluginMap,
		Cmd:              exec.Command(pluginData.Command),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	// Connect via gRPC
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// Request the plugin interface
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	// Cast to gRPC client
	grpcClient, ok := raw.(pb.PluginServiceClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin does not implement PluginServiceClient interface")
	}

	// Parse plugin config
	var config map[string]interface{}
	if pluginData.Config != nil {
		// Convert datatypes.JSON to map[string]interface{}
		if err := json.Unmarshal(pluginData.Config, &config); err != nil {
			client.Kill()
			return nil, fmt.Errorf("failed to parse plugin config: %w", err)
		}
	}

	// Initialize plugin
	configStrings := make(map[string]string)
	for k, v := range config {
		configStrings[k] = fmt.Sprintf("%v", v)
	}

	initResp, err := grpcClient.Initialize(context.Background(), &pb.InitRequest{
		Config: configStrings,
	})
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to initialize plugin: %w", err)
	}

	if !initResp.Success {
		client.Kill()
		return nil, fmt.Errorf("plugin initialization failed: %s", initResp.ErrorMessage)
	}

	// Create loaded plugin instance
	loadedPlugin := &LoadedPlugin{
		ID:         pluginData.ID,
		Name:       pluginData.Name,
		Slug:       pluginData.Slug,
		HookType:   interfaces.HookType(pluginData.HookType),
		Client:     client,
		GRPCClient: grpcClient,
		Config:     config,
		Checksum:   pluginData.Checksum,
		IsHealthy:  true,
	}

	// Store references
	pm.loadedPlugins[pluginID] = loadedPlugin
	pm.pluginClients[pluginID] = client

	// Start health monitoring
	go pm.monitorPluginHealth(pluginID)

	log.Info().
		Uint("plugin_id", pluginID).
		Str("plugin_name", pluginData.Name).
		Str("hook_type", pluginData.HookType).
		Msg("Plugin loaded successfully")

	return loadedPlugin, nil
}

// UnloadPlugin unloads a plugin by ID
func (pm *PluginManager) UnloadPlugin(pluginID uint) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	loadedPlugin, exists := pm.loadedPlugins[pluginID]
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
	delete(pm.loadedPlugins, pluginID)
	delete(pm.pluginClients, pluginID)
	delete(pm.reattachConfigs, pluginID)

	// Remove from LLM mappings
	for llmID, pluginIDs := range pm.llmPluginMap {
		var newPluginIDs []uint
		for _, pID := range pluginIDs {
			if pID != pluginID {
				newPluginIDs = append(newPluginIDs, pID)
			}
		}
		pm.llmPluginMap[llmID] = newPluginIDs
	}

	log.Info().
		Uint("plugin_id", pluginID).
		Str("plugin_name", loadedPlugin.Name).
		Msg("Plugin unloaded successfully")

	return nil
}

// ReloadPlugin reloads a plugin by unloading and loading it again
func (pm *PluginManager) ReloadPlugin(pluginID uint) error {
	// Unload first
	if err := pm.UnloadPlugin(pluginID); err != nil {
		// Log the error but continue with loading
		log.Warn().
			Uint("plugin_id", pluginID).
			Err(err).
			Msg("Error during plugin unload, continuing with load")
	}

	// Load again
	_, err := pm.LoadPlugin(pluginID)
	return err
}

// GetPluginsForLLM returns loaded plugins for a specific LLM and hook type
func (pm *PluginManager) GetPluginsForLLM(llmID uint, hookType interfaces.HookType) ([]*LoadedPlugin, error) {
	// Get plugins from database (this ensures we have the latest associations)
	plugins, err := pm.service.GetPluginsForLLM(llmID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugins for LLM: %w", err)
	}

	var result []*LoadedPlugin

	// Filter by hook type and ensure plugins are loaded
	for _, pluginData := range plugins {
		if interfaces.HookType(pluginData.HookType) != hookType {
			continue
		}

		pm.mu.RLock()
		loadedPlugin, exists := pm.loadedPlugins[pluginData.ID]
		pm.mu.RUnlock()
		
		if !exists {
			// Try to load the plugin (this will acquire its own lock)
			loadedPlugin, err = pm.LoadPlugin(pluginData.ID)
			if err != nil {
				log.Error().
					Uint("plugin_id", pluginData.ID).
					Err(err).
					Msg("Failed to auto-load plugin")
				continue
			}
		}

		// Check if plugin is healthy
		pm.mu.RLock()
		isHealthy := loadedPlugin.IsHealthy
		pm.mu.RUnlock()
		
		if !isHealthy {
			log.Warn().
				Uint("plugin_id", pluginData.ID).
				Msg("Skipping unhealthy plugin")
			continue
		}

		result = append(result, loadedPlugin)
	}

	return result, nil
}

// ExecutePluginChain executes a chain of plugins for a specific LLM and hook type
func (pm *PluginManager) ExecutePluginChain(llmID uint, hookType interfaces.HookType, input interface{}, pluginCtx *interfaces.PluginContext) (interface{}, error) {
	plugins, err := pm.GetPluginsForLLM(llmID, hookType)
	if err != nil {
		return nil, err
	}

	if len(plugins) == 0 {
		// No plugins for this hook type, return input unchanged
		return input, nil
	}

	result := input

	for _, plugin := range plugins {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		switch hookType {
		case interfaces.HookTypePreAuth:
			pluginReq, ok := result.(*interfaces.PluginRequest)
			if !ok {
				return nil, fmt.Errorf("invalid input type for pre-auth hook")
			}
			
			pbCtx := convertPluginContext(pluginCtx)
			pbReq := convertPluginRequest(pluginReq, pbCtx)
			
			resp, err := plugin.GRPCClient.ProcessPreAuth(ctx, pbReq)
			if err != nil {
				return nil, fmt.Errorf("plugin %s execution failed: %w", plugin.Name, err)
			}
			
			if resp.Block {
				// Plugin wants to block the request
				return convertPluginResponse(resp), nil
			}
			
			if resp.Modified {
				// Return the plugin response directly so modifications are preserved
				result = convertPluginResponse(resp)
			}

		case interfaces.HookTypeAuth:
			authReq, ok := result.(*interfaces.AuthRequest)
			if !ok {
				return nil, fmt.Errorf("invalid input type for auth hook")
			}
			
			pbCtx := convertPluginContext(pluginCtx)
			pbReq := convertAuthRequest(authReq, pbCtx)
			
			resp, err := plugin.GRPCClient.Authenticate(ctx, pbReq)
			if err != nil {
				return nil, fmt.Errorf("plugin %s execution failed: %w", plugin.Name, err)
			}
			
			result = convertAuthResponse(resp)

		case interfaces.HookTypePostAuth:
			enrichedReq, ok := result.(*interfaces.EnrichedRequest)
			if !ok {
				return nil, fmt.Errorf("invalid input type for post-auth hook")
			}
			
			pbCtx := convertPluginContext(pluginCtx)
			pbReq := convertEnrichedRequest(enrichedReq, pbCtx)
			
			resp, err := plugin.GRPCClient.ProcessPostAuth(ctx, pbReq)
			if err != nil {
				return nil, fmt.Errorf("plugin %s execution failed: %w", plugin.Name, err)
			}
			
			log.Debug().Bool("resp_modified", resp.Modified).Bool("resp_block", resp.Block).Int("body_len", len(resp.Body)).Msg("Post-auth plugin response received")
			
			if resp.Block {
				// Plugin wants to block the request
				return convertPluginResponse(resp), nil
			}
			
			if resp.Modified {
				// Return the plugin response directly so modifications are preserved
				log.Debug().Bool("resp_modified", resp.Modified).Int("body_len", len(resp.Body)).Msg("Post-auth plugin returned Modified=true, converting response")
				result = convertPluginResponse(resp)
				log.Debug().Interface("converted_result", result).Msg("Post-auth plugin response converted")
			}

		case interfaces.HookTypeOnResponse:
			// Response plugins are now handled via AI Gateway hooks, not here
			// The AI Gateway calls the plugins directly via hooks
			// Just return the input unchanged since hooks handle this
			return result, nil

		default:
			return nil, fmt.Errorf("unsupported hook type: %s", hookType)
		}

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("hook_type", string(hookType)).
			Msg("Plugin executed successfully")
	}

	return result, nil
}

// monitorPluginHealth monitors the health of a loaded plugin
func (pm *PluginManager) monitorPluginHealth(pluginID uint) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		pm.mu.RLock()
		loadedPlugin, exists := pm.loadedPlugins[pluginID]
		pm.mu.RUnlock()

		if !exists {
			// Plugin was unloaded, stop monitoring
			return
		}

		// Ping the plugin
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := loadedPlugin.GRPCClient.Ping(ctx, &pb.PingRequest{
			Timestamp: time.Now().Unix(),
		})
		cancel()

		if err != nil || !resp.Healthy {
			log.Warn().
				Uint("plugin_id", pluginID).
				Str("plugin_name", loadedPlugin.Name).
				Err(err).
				Msg("Plugin health check failed")

			pm.mu.Lock()
			loadedPlugin.IsHealthy = false
			pm.mu.Unlock()

			// TODO: Implement plugin restart logic
		} else {
			pm.mu.Lock()
			loadedPlugin.IsHealthy = true
			pm.mu.Unlock()
		}
	}
}

// Shutdown gracefully shuts down all loaded plugins
func (pm *PluginManager) Shutdown(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var errors []error

	for pluginID, loadedPlugin := range pm.loadedPlugins {
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		
		// Attempt graceful shutdown
		if loadedPlugin.GRPCClient != nil {
			_, err := loadedPlugin.GRPCClient.Shutdown(shutdownCtx, &pb.ShutdownRequest{
				TimeoutSeconds: 5,
			})
			if err != nil {
				errors = append(errors, fmt.Errorf("failed to shutdown plugin %d: %w", pluginID, err))
			}
		}
		
		cancel()

		// Force kill if still running
		if loadedPlugin.Client != nil {
			loadedPlugin.Client.Kill()
		}
	}

	// Clear all maps
	pm.loadedPlugins = make(map[uint]*LoadedPlugin)
	pm.pluginClients = make(map[uint]*plugin.Client)
	pm.reattachConfigs = make(map[uint]*plugin.ReattachConfig)
	pm.llmPluginMap = make(map[uint][]uint)

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// SaveReattachConfig saves the reattach configuration for a plugin
func (pm *PluginManager) SaveReattachConfig(pluginID uint) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	client, exists := pm.pluginClients[pluginID]
	if !exists {
		return fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	reattachConfig := client.ReattachConfig()
	if reattachConfig == nil {
		return fmt.Errorf("plugin %d does not support reattach", pluginID)
	}

	pm.reattachConfigs[pluginID] = reattachConfig
	
	log.Info().
		Uint("plugin_id", pluginID).
		Msg("Reattach config saved for plugin")

	return nil
}

// ReattachPlugin reattaches to an existing plugin process
func (pm *PluginManager) ReattachPlugin(pluginID uint, config *plugin.ReattachConfig) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if plugin is already loaded
	if _, exists := pm.loadedPlugins[pluginID]; exists {
		return fmt.Errorf("plugin %d is already loaded", pluginID)
	}

	// Get plugin from database
	pluginData, err := pm.service.GetPlugin(pluginID)
	if err != nil {
		return fmt.Errorf("failed to get plugin from database: %w", err)
	}

	// Create client with reattach config
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pm.handshakeConfig,
		Plugins:          pm.pluginMap,
		Reattach:         config,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	// Connect via gRPC
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// Request the plugin interface
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense plugin: %w", err)
	}

	grpcClient, ok := raw.(pb.PluginServiceClient)
	if !ok {
		client.Kill()
		return fmt.Errorf("plugin does not implement PluginServiceClient interface")
	}

	// Parse plugin config
	var pluginConfig map[string]interface{}
	if pluginData.Config != nil {
		if err := json.Unmarshal(pluginData.Config, &pluginConfig); err != nil {
			client.Kill()
			return fmt.Errorf("failed to parse plugin config: %w", err)
		}
	}

	// Create loaded plugin instance
	loadedPlugin := &LoadedPlugin{
		ID:         pluginData.ID,
		Name:       pluginData.Name,
		Slug:       pluginData.Slug,
		HookType:   interfaces.HookType(pluginData.HookType),
		Client:     client,
		GRPCClient: grpcClient,
		Config:     pluginConfig,
		Checksum:   pluginData.Checksum,
		IsHealthy:  true,
	}

	// Store references
	pm.loadedPlugins[pluginID] = loadedPlugin
	pm.pluginClients[pluginID] = client
	pm.reattachConfigs[pluginID] = config

	// Start health monitoring
	go pm.monitorPluginHealth(pluginID)

	log.Info().
		Uint("plugin_id", pluginID).
		Str("plugin_name", pluginData.Name).
		Msg("Plugin reattached successfully")

	return nil
}

// GetLoadedPlugins returns a list of all loaded plugins
func (pm *PluginManager) GetLoadedPlugins() []*LoadedPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var plugins []*LoadedPlugin
	for _, plugin := range pm.loadedPlugins {
		plugins = append(plugins, plugin)
	}

	return plugins
}

// IsPluginLoaded checks if a plugin is currently loaded
func (pm *PluginManager) IsPluginLoaded(pluginID uint) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	_, exists := pm.loadedPlugins[pluginID]
	return exists
}

// RefreshLLMPluginMapping refreshes the LLM to plugin mapping from the database
func (pm *PluginManager) RefreshLLMPluginMapping(llmID uint) error {
	plugins, err := pm.service.GetPluginsForLLM(llmID)
	if err != nil {
		return fmt.Errorf("failed to get plugins for LLM: %w", err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clear existing mapping for this LLM
	delete(pm.llmPluginMap, llmID)

	// Build new mapping
	var pluginIDs []uint
	for _, plugin := range plugins {
		pluginIDs = append(pluginIDs, plugin.ID)
	}

	if len(pluginIDs) > 0 {
		pm.llmPluginMap[llmID] = pluginIDs
	}

	return nil
}