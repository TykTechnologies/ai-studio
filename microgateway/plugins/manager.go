// plugins/manager.go
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
)

// PluginServiceInterface defines minimal interface needed by plugin manager
// This avoids circular dependency with services package
type PluginServiceInterface interface {
	GetPlugin(id uint) (PluginData, error)
	GetPluginsByLLMID(llmID uint) ([]PluginData, error)
	GetPluginsForLLM(llmID uint) ([]PluginData, error)
}

// PluginData represents plugin data from database (minimal interface)
type PluginData struct {
	ID          uint
	Name        string
	Slug        string
	HookType    string
	Command     string
	Config      []byte  // JSON-encoded config (matches datatypes.JSON from database)
	Checksum    string
	IsActive    bool
}

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
	IsGlobal    bool   // True for global plugins (vs per-LLM plugins)
}

// GlobalPlugin represents a global data collection plugin instance
type GlobalPlugin struct {
	Config         DataCollectionPluginConfig
	Client         *plugin.Client
	GRPCClient     pb.PluginServiceClient
	LoadedPlugin   *LoadedPlugin
	IsHealthy      bool
}

// PluginManager manages the lifecycle of plugins
type PluginManager struct {
	mu                      sync.RWMutex
	loadedPlugins           map[uint]*LoadedPlugin           // Plugin ID -> loaded plugin
	llmPluginMap            map[uint][]uint                  // LLM ID -> Plugin IDs (ordered)
	pluginClients           map[uint]*plugin.Client          // Plugin ID -> go-plugin client
	reattachConfigs         map[uint]*plugin.ReattachConfig  // For reconnection
	service                 PluginServiceInterface // Database service
	handshakeConfig         plugin.HandshakeConfig
	pluginMap               map[string]plugin.Plugin

	// Global data collection plugins
	globalDataPlugins       map[string]*GlobalPlugin         // Plugin name -> global plugin
	dataCollectionHookTypes map[string][]string              // Plugin name -> hook types it handles

	// OCI plugin support
	ociClient               *ociplugins.OCIPluginClient      // OCI plugin client for fetching
}

// HandshakeConfig is used to do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "MICROGATEWAY_PLUGIN",
	MagicCookieValue: "v1",
}

// NewPluginManager creates a new plugin manager instance
func NewPluginManager(pluginService PluginServiceInterface) *PluginManager {
	return &PluginManager{
		loadedPlugins:           make(map[uint]*LoadedPlugin),
		llmPluginMap:            make(map[uint][]uint),
		pluginClients:           make(map[uint]*plugin.Client),
		reattachConfigs:         make(map[uint]*plugin.ReattachConfig),
		service:                 pluginService,
		handshakeConfig:         HandshakeConfig,
		pluginMap: map[string]plugin.Plugin{
			"plugin": &PluginGRPC{},
		},
		globalDataPlugins:       make(map[string]*GlobalPlugin),
		dataCollectionHookTypes: make(map[string][]string),
		ociClient:               nil, // Will be initialized when needed
	}
}

// NewPluginManagerWithOCI creates a new plugin manager with OCI support
func NewPluginManagerWithOCI(pluginService PluginServiceInterface, ociConfig *ociplugins.OCIConfig) (*PluginManager, error) {
	pm := NewPluginManager(pluginService)

	if ociConfig != nil {
		// Initialize OCI client
		ociClient, err := ociplugins.NewOCIPluginClient(ociConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create OCI plugin client: %w", err)
		}
		pm.ociClient = ociClient
	}

	return pm, nil
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

	// Create plugin client based on command scheme
	client, err := pm.createPluginClient(pluginData.Command)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin client: %w", err)
	}

	// Connect via gRPC with timeout and retry for external services
	rpcClient, err := pm.connectWithRetry(client, pluginData.Command)
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
	pluginsData, err := pm.service.GetPluginsForLLM(llmID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugins for LLM: %w", err)
	}

	var result []*LoadedPlugin

	// Filter by hook type and ensure plugins are loaded
	for _, pluginData := range pluginsData {
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
			// NOTE: Response plugins are handled by the AI Gateway through the adapter
			// The plugin manager loads response plugins, but the AI Gateway handles execution
			// This case should not be reached since response plugins are routed through the adapter
			log.Debug().Msg("Response plugin execution requested - this should be handled by AI Gateway adapter")
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

			// Attempt to restart the plugin after health check failure
			log.Info().
				Uint("plugin_id", pluginID).
				Str("plugin_name", loadedPlugin.Name).
				Msg("Attempting automatic plugin restart")
			
			if restartErr := pm.ReloadPlugin(pluginID); restartErr != nil {
				log.Error().
					Uint("plugin_id", pluginID).
					Str("plugin_name", loadedPlugin.Name).
					Err(restartErr).
					Msg("Failed to restart plugin automatically")
			} else {
				log.Info().
					Uint("plugin_id", pluginID).
					Str("plugin_name", loadedPlugin.Name).
					Msg("Plugin restarted successfully after health check failure")
			}
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

// createPluginClient creates a plugin client based on command scheme
func (pm *PluginManager) createPluginClient(command string) (*plugin.Client, error) {
	if strings.HasPrefix(command, "oci://") {
		// OCI plugin - fetch from registry first
		return pm.createOCIPluginClient(command)
	} else if strings.HasPrefix(command, "grpc://") {
		// External gRPC service - use ReattachConfig
		reattachConfig, err := pm.parseGRPCReattachConfig(command)
		if err != nil {
			return nil, fmt.Errorf("failed to parse gRPC address: %w", err)
		}

		log.Info().
			Str("command", command).
			Str("address", reattachConfig.Addr.String()).
			Msg("Creating client for external gRPC plugin")

		return plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig:  pm.handshakeConfig,
			Plugins:          pm.pluginMap,
			Reattach:         reattachConfig,
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		}), nil
	} else {
		// Local executable - use exec.Command
		cmdPath := command
		if strings.HasPrefix(command, "file://") {
			cmdPath = strings.TrimPrefix(command, "file://")
		}

		log.Info().
			Str("command", command).
			Str("path", cmdPath).
			Msg("Creating client for local plugin executable")

		return plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig:  pm.handshakeConfig,
			Plugins:          pm.pluginMap,
			Cmd:              exec.Command(cmdPath),
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		}), nil
	}
}

// createOCIPluginClient fetches an OCI plugin and creates a client
func (pm *PluginManager) createOCIPluginClient(command string) (*plugin.Client, error) {
	if pm.ociClient == nil {
		return nil, fmt.Errorf("OCI client not initialized - microgateway must be configured with OCI support")
	}

	// Parse OCI reference
	ref, params, err := ociplugins.ParseOCICommand(command)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI command: %w", err)
	}

	log.Info().
		Str("command", command).
		Str("registry", ref.Registry).
		Str("repository", ref.Repository).
		Str("digest", ref.Digest).
		Str("architecture", params.Architecture).
		Msg("Fetching OCI plugin")

	// Fetch plugin from OCI registry
	localPlugin, err := pm.ociClient.FetchPlugin(context.Background(), ref, params)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OCI plugin: %w", err)
	}

	log.Info().
		Str("command", command).
		Str("local_path", localPlugin.ExecutablePath).
		Bool("verified", localPlugin.Verified).
		Msg("OCI plugin fetched successfully")

	// Create go-plugin client with the fetched executable
	return plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pm.handshakeConfig,
		Plugins:          pm.pluginMap,
		Cmd:              exec.Command(localPlugin.ExecutablePath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	}), nil
}

// parseGRPCReattachConfig parses a gRPC URL and creates a ReattachConfig
func (pm *PluginManager) parseGRPCReattachConfig(grpcURL string) (*plugin.ReattachConfig, error) {
	// Remove grpc:// prefix
	address := strings.TrimPrefix(grpcURL, "grpc://")

	// Parse host:port
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid gRPC address format '%s': %w", address, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port in gRPC address '%s': %w", address, err)
	}

	// Create TCP address
	tcpAddr := &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}

	// If host is not an IP, resolve it
	if tcpAddr.IP == nil {
		tcpAddr, err = net.ResolveTCPAddr("tcp", address)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve gRPC address '%s': %w", address, err)
		}
	}

	return &plugin.ReattachConfig{
		Protocol: plugin.ProtocolGRPC,
		Addr:     tcpAddr,
		Pid:      0, // Not applicable for network connections
	}, nil
}

// connectWithRetry connects to a plugin with retry logic for external services
func (pm *PluginManager) connectWithRetry(client *plugin.Client, command string) (plugin.ClientProtocol, error) {
	isExternal := strings.HasPrefix(command, "grpc://")

	if !isExternal {
		// For local plugins, use standard connection
		return client.Client()
	}

	// For external gRPC services, implement retry logic
	maxRetries := 3
	retryDelay := time.Second * 2

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Info().
				Str("command", command).
				Int("attempt", attempt+1).
				Int("max_retries", maxRetries).
				Dur("delay", retryDelay).
				Msg("Retrying connection to external gRPC plugin")
			time.Sleep(retryDelay)
		}

		// Create context with timeout for connection attempt
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Attempt connection
		rpcClient, err := client.Client()
		if err != nil {
			lastErr = err
			log.Warn().
				Str("command", command).
				Int("attempt", attempt+1).
				Err(err).
				Msg("Failed to connect to external gRPC plugin")
			continue
		}

		// Test the connection with a ping
		raw, err := rpcClient.Dispense("plugin")
		if err != nil {
			lastErr = err
			log.Warn().
				Str("command", command).
				Int("attempt", attempt+1).
				Err(err).
				Msg("Failed to dispense plugin interface")
			continue
		}

		pluginClient, ok := raw.(pb.PluginServiceClient)
		if !ok {
			lastErr = fmt.Errorf("plugin does not implement PluginServiceClient interface")
			continue
		}

		// Test with a ping request
		_, pingErr := pluginClient.Ping(ctx, &pb.PingRequest{
			Timestamp: time.Now().Unix(),
		})

		if pingErr != nil {
			lastErr = pingErr
			log.Warn().
				Str("command", command).
				Int("attempt", attempt+1).
				Err(pingErr).
				Msg("Plugin ping failed during connection test")
			continue
		}

		log.Info().
			Str("command", command).
			Int("attempt", attempt+1).
			Msg("Successfully connected to external gRPC plugin")

		return rpcClient, nil
	}

	return nil, fmt.Errorf("failed to connect to external gRPC plugin after %d attempts: %w", maxRetries, lastErr)
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
	pluginsData, err := pm.service.GetPluginsForLLM(llmID)
	if err != nil {
		return fmt.Errorf("failed to get plugins for LLM: %w", err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clear existing mapping for this LLM
	delete(pm.llmPluginMap, llmID)

	// Build new mapping
	var pluginIDs []uint
	for _, plugin := range pluginsData {
		pluginIDs = append(pluginIDs, plugin.ID)
	}

	if len(pluginIDs) > 0 {
		pm.llmPluginMap[llmID] = pluginIDs
	}

	return nil
}
// LoadGlobalDataCollectionPlugins loads global data collection plugins from configuration
func (pm *PluginManager) LoadGlobalDataCollectionPlugins(configs []DataCollectionPluginConfig) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	for _, cfg := range configs {
		if !cfg.Enabled {
			log.Debug().Str("plugin", cfg.Name).Msg("Skipping disabled plugin")
			continue
		}
		
		// Load plugin from path
		globalPlugin, err := pm.loadGlobalPluginFromConfig(cfg)
		if err != nil {
			log.Error().
				Str("plugin", cfg.Name).
				Str("path", cfg.Path).
				Err(err).
				Msg("Failed to load global data collection plugin")
			continue
		}
		
		// Store global plugin
		pm.globalDataPlugins[cfg.Name] = globalPlugin
		pm.dataCollectionHookTypes[cfg.Name] = cfg.HookTypes
		
		log.Info().
			Str("plugin", cfg.Name).
			Strs("hook_types", cfg.HookTypes).
			Bool("replace_database", cfg.ReplaceDatabase).
			Msg("Loaded global data collection plugin")
	}
	
	return nil
}

// loadGlobalPluginFromConfig loads a global plugin from configuration
func (pm *PluginManager) loadGlobalPluginFromConfig(cfg DataCollectionPluginConfig) (*GlobalPlugin, error) {
	// Create plugin client based on path scheme
	client, err := pm.createPluginClient(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin client for global plugin: %w", err)
	}
	
	// Connect via gRPC with timeout and retry for external services
	rpcClient, err := pm.connectWithRetry(client, cfg.Path)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}
	
	// Get the plugin client
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}
	
	pluginClient := raw.(pb.PluginServiceClient)
	
	// Initialize the plugin
	initReq := &pb.InitRequest{
		Config: make(map[string]string),
	}
	
	// Convert config map to string map for protobuf
	for key, value := range cfg.Config {
		if str, ok := value.(string); ok {
			initReq.Config[key] = str
		} else {
			// Try to JSON encode non-string values
			if jsonBytes, err := json.Marshal(value); err == nil {
				initReq.Config[key] = string(jsonBytes)
			}
		}
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	initResp, err := pluginClient.Initialize(ctx, initReq)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("failed to initialize plugin: %w", err)
	}
	
	if !initResp.Success {
		client.Kill()
		return nil, fmt.Errorf("plugin initialization failed: %s", initResp.ErrorMessage)
	}
	
	// Create global plugin instance
	globalPlugin := &GlobalPlugin{
		Config:     cfg,
		Client:     client,
		GRPCClient: pluginClient,
		IsHealthy:  true,
		LoadedPlugin: &LoadedPlugin{
			Name:       cfg.Name,
			HookType:   interfaces.HookTypeDataCollection,
			Client:     client,
			GRPCClient: pluginClient,
			Config:     cfg.Config,
			IsHealthy:  true,
			IsGlobal:   true,
		},
	}
	
	return globalPlugin, nil
}

// ExecuteDataCollectionPlugins executes global data collection plugins for the specified hook type
func (pm *PluginManager) ExecuteDataCollectionPlugins(hookType string, data interface{}) error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	executedCount := 0
	
	// Find plugins that handle this hook type
	for pluginName, hookTypes := range pm.dataCollectionHookTypes {
		if !pm.pluginHandlesHookType(hookTypes, hookType) {
			log.Debug().
				Str("plugin", pluginName).
				Str("hook_type", hookType).
				Strs("plugin_hook_types", hookTypes).
				Msg("Plugin does not handle this hook type - skipping")
			continue
		}
		
		globalPlugin, exists := pm.globalDataPlugins[pluginName]
		if !exists {
			log.Warn().Str("plugin", pluginName).Msg("Plugin not found in global plugins map")
			continue
		}
		if !globalPlugin.IsHealthy {
			log.Warn().Str("plugin", pluginName).Msg("Plugin is unhealthy - skipping")
			continue
		}
		
		log.Debug().
			Str("plugin", pluginName).
			Str("hook_type", hookType).
			Msg("Executing data collection plugin")
		
		// Execute plugin based on hook type
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := pm.executeDataCollectionPlugin(ctx, globalPlugin, hookType, data)
		cancel()
		
		if err != nil {
			log.Error().
				Str("plugin", pluginName).
				Str("hook_type", hookType).
				Err(err).
				Msg("Data collection plugin execution failed")
			
			// Mark plugin as unhealthy and attempt restart after failures
			pm.mu.Lock()
			globalPlugin.IsHealthy = false
			pm.mu.Unlock()
			
			log.Warn().
				Str("plugin", pluginName).
				Msg("Marking global data collection plugin as unhealthy due to execution failure")
			
			// Note: Global plugin restart would require reloading from configuration
			// This is more complex than regular plugin restart and would need
			// access to the original plugin configuration
		} else {
			log.Debug().
				Str("plugin", pluginName).
				Str("hook_type", hookType).
				Msg("Data collection plugin executed successfully")
			executedCount++
		}
	}
	
	log.Debug().
		Str("hook_type", hookType).
		Int("executed_count", executedCount).
		Int("total_plugins", len(pm.globalDataPlugins)).
		Msg("Data collection plugin execution summary")
	
	return nil
}

// executeDataCollectionPlugin executes a specific plugin for the given data type
func (pm *PluginManager) executeDataCollectionPlugin(ctx context.Context, plugin *GlobalPlugin, hookType string, data interface{}) error {
	switch hookType {
	case "proxy_log":
		if proxyData, ok := data.(*interfaces.ProxyLogData); ok {
			return pm.executeProxyLogPlugin(ctx, plugin, proxyData)
		}
	case "analytics":
		if analyticsData, ok := data.(*interfaces.AnalyticsData); ok {
			return pm.executeAnalyticsPlugin(ctx, plugin, analyticsData)
		}
	case "budget":
		if budgetData, ok := data.(*interfaces.BudgetUsageData); ok {
			return pm.executeBudgetUsagePlugin(ctx, plugin, budgetData)
		}
	}
	
	return fmt.Errorf("unsupported hook type: %s", hookType)
}

// executeProxyLogPlugin executes proxy log data collection
func (pm *PluginManager) executeProxyLogPlugin(ctx context.Context, plugin *GlobalPlugin, data *interfaces.ProxyLogData) error {
	// Convert to protobuf request
	req := &pb.ProxyLogRequest{
		AppId:        uint32(data.AppID),
		UserId:       uint32(data.UserID),
		Vendor:       data.Vendor,
		RequestBody:  data.RequestBody,
		ResponseBody: data.ResponseBody,
		ResponseCode: int32(data.ResponseCode),
		Timestamp:    data.Timestamp.Unix(),
		RequestId:    data.RequestID,
		Context: &pb.PluginContext{
			RequestId: data.RequestID,
			AppId:     uint32(data.AppID),
			UserId:    uint32(data.UserID),
		},
	}
	
	resp, err := plugin.GRPCClient.HandleProxyLog(ctx, req)
	if err != nil {
		return err
	}
	
	if !resp.Success {
		return fmt.Errorf("plugin execution failed: %s", resp.ErrorMessage)
	}
	
	return nil
}

// executeAnalyticsPlugin executes analytics data collection
func (pm *PluginManager) executeAnalyticsPlugin(ctx context.Context, plugin *GlobalPlugin, data *interfaces.AnalyticsData) error {
	log.Debug().
		Str("plugin", plugin.Config.Name).
		Int("total_tokens", data.TotalTokens).
		Float64("cost", data.Cost).
		Str("model", data.ModelName).
		Msg("Executing analytics plugin with data")

	// Convert to protobuf request
	req := &pb.AnalyticsRequest{
		LlmId:                   uint32(data.LLMID),
		ModelName:              data.ModelName,
		Vendor:                 data.Vendor,
		PromptTokens:           int32(data.PromptTokens),
		ResponseTokens:         int32(data.ResponseTokens),
		CacheWritePromptTokens: int32(data.CacheWritePromptTokens),
		CacheReadPromptTokens:  int32(data.CacheReadPromptTokens),
		TotalTokens:            int32(data.TotalTokens),
		Cost:                   data.Cost,
		Currency:               data.Currency,
		AppId:                  uint32(data.AppID),
		UserId:                 uint32(data.UserID),
		Timestamp:              data.Timestamp.Unix(),
		ToolCalls:              int32(data.ToolCalls),
		Choices:                int32(data.Choices),
		RequestId:              data.RequestID,
		Context: &pb.PluginContext{
			RequestId: data.RequestID,
			LlmId:     uint32(data.LLMID),
			AppId:     uint32(data.AppID),
			UserId:    uint32(data.UserID),
		},
	}
	
	resp, err := plugin.GRPCClient.HandleAnalytics(ctx, req)
	if err != nil {
		return err
	}
	
	if !resp.Success {
		return fmt.Errorf("plugin execution failed: %s", resp.ErrorMessage)
	}
	
	return nil
}

// executeBudgetUsagePlugin executes budget usage data collection
func (pm *PluginManager) executeBudgetUsagePlugin(ctx context.Context, plugin *GlobalPlugin, data *interfaces.BudgetUsageData) error {
	log.Debug().
		Str("plugin", plugin.Config.Name).
		Int64("tokens_used", data.TokensUsed).
		Float64("cost", data.Cost).
		Uint("app_id", data.AppID).
		Msg("Executing budget usage plugin with data")

	// Convert to protobuf request
	req := &pb.BudgetUsageRequest{
		AppId:            uint32(data.AppID),
		LlmId:            uint32(data.LLMID),
		TokensUsed:       data.TokensUsed,
		Cost:             data.Cost,
		RequestsCount:    int32(data.RequestsCount),
		PromptTokens:     data.PromptTokens,
		CompletionTokens: data.CompletionTokens,
		PeriodStart:      data.PeriodStart.Unix(),
		PeriodEnd:        data.PeriodEnd.Unix(),
		Timestamp:        data.Timestamp.Unix(),
		RequestId:        data.RequestID,
		Context: &pb.PluginContext{
			RequestId: data.RequestID,
			AppId:     uint32(data.AppID),
		},
	}
	
	resp, err := plugin.GRPCClient.HandleBudgetUsage(ctx, req)
	if err != nil {
		return err
	}
	
	if !resp.Success {
		return fmt.Errorf("plugin execution failed: %s", resp.ErrorMessage)
	}
	
	return nil
}

// pluginHandlesHookType checks if a plugin handles the specified hook type
func (pm *PluginManager) pluginHandlesHookType(hookTypes []string, hookType string) bool {
	for _, ht := range hookTypes {
		if ht == hookType {
			return true
		}
	}
	return false
}

// UnloadGlobalPlugins unloads all global data collection plugins
func (pm *PluginManager) UnloadGlobalPlugins() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	for name, plugin := range pm.globalDataPlugins {
		if plugin.Client != nil {
			plugin.Client.Kill()
		}
		log.Info().Str("plugin", name).Msg("Unloaded global data collection plugin")
	}
	
	pm.globalDataPlugins = make(map[string]*GlobalPlugin)
	pm.dataCollectionHookTypes = make(map[string][]string)
}


// ShouldReplaceDatabaseStorage checks if any plugin is configured to replace database storage for the given hook type
func (pm *PluginManager) ShouldReplaceDatabaseStorage(hookType string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	for pluginName, hookTypes := range pm.dataCollectionHookTypes {
		if !pm.pluginHandlesHookType(hookTypes, hookType) {
			continue
		}
		
		globalPlugin, exists := pm.globalDataPlugins[pluginName]
		if !exists || !globalPlugin.IsHealthy {
			continue
		}
		
		// Check if this plugin is configured to replace database storage
		if globalPlugin.Config.ReplaceDatabase {
			return true
		}
	}
	
	return false
}

// GetGlobalPluginsForHookType returns all global plugins that handle the specified hook type
func (pm *PluginManager) GetGlobalPluginsForHookType(hookType string) []*GlobalPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	var plugins []*GlobalPlugin
	for pluginName, hookTypes := range pm.dataCollectionHookTypes {
		if !pm.pluginHandlesHookType(hookTypes, hookType) {
			continue
		}
		
		globalPlugin, exists := pm.globalDataPlugins[pluginName]
		if exists && globalPlugin.IsHealthy {
			plugins = append(plugins, globalPlugin)
		}
	}

	return plugins
}

// GetOCIClient returns the OCI client if available
func (pm *PluginManager) GetOCIClient() *ociplugins.OCIPluginClient {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.ociClient
}

// GetOCIStats returns OCI plugin statistics
func (pm *PluginManager) GetOCIStats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]interface{})

	if pm.ociClient == nil {
		stats["enabled"] = false
		return stats
	}

	stats["enabled"] = true

	// Get cache size
	if cacheSize, err := pm.ociClient.GetCacheSize(); err == nil {
		stats["cache_size_bytes"] = cacheSize
	}

	// Get cached plugin count
	if cachedPlugins, err := pm.ociClient.ListCached(); err == nil {
		stats["cached_plugins_count"] = len(cachedPlugins)

		// Group by registry
		registryStats := make(map[string]int)
		archStats := make(map[string]int)

		for _, plugin := range cachedPlugins {
			registryStats[plugin.Reference.Registry]++
			archStats[plugin.Params.Architecture]++
		}

		stats["plugins_by_registry"] = registryStats
		stats["plugins_by_architecture"] = archStats
	}

	return stats
}

// ListCachedOCIPlugins returns all cached OCI plugins
func (pm *PluginManager) ListCachedOCIPlugins() ([]*ociplugins.LocalPlugin, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.ociClient == nil {
		return nil, fmt.Errorf("OCI client not available")
	}

	return pm.ociClient.ListCached()
}

// PreFetchOCIPlugin pre-fetches an OCI plugin without loading it
func (pm *PluginManager) PreFetchOCIPlugin(command string) error {
	pm.mu.RLock()
	ociClient := pm.ociClient
	pm.mu.RUnlock()

	if ociClient == nil {
		return fmt.Errorf("OCI client not available")
	}

	// Parse OCI reference
	ref, params, err := ociplugins.ParseOCICommand(command)
	if err != nil {
		return fmt.Errorf("failed to parse OCI command: %w", err)
	}

	// Fetch plugin (this will cache it)
	_, err = ociClient.FetchPlugin(context.Background(), ref, params)
	if err != nil {
		return fmt.Errorf("failed to pre-fetch OCI plugin: %w", err)
	}

	log.Info().
		Str("reference", ref.FullReference()).
		Msg("OCI plugin pre-fetched successfully")

	return nil
}

