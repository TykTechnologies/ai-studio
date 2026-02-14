// plugins/manager.go
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	configpb "github.com/TykTechnologies/midsommar/v2/proto"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
)

// PluginServiceInterface defines minimal interface needed by plugin manager
// This avoids circular dependency with services package
type PluginServiceInterface interface {
	GetPlugin(id uint) (PluginData, error)
	GetPluginsByLLMID(llmID uint) ([]PluginData, error)
	GetPluginsForLLM(llmID uint) ([]PluginData, error)
	GetAllPlugins() ([]PluginData, error)
	GetAllLLMIDs() ([]uint, error)                  // Get all LLM IDs for pre-warming plugins
	GetAllActiveGatewayPlugins() ([]PluginData, error) // Get all plugins that should run on a gateway (LLM-associated + standalone endpoint)
}

// PluginData represents plugin data from database (minimal interface)
type PluginData struct {
	ID        uint
	Name      string
	HookType  string
	HookTypes []string // All hook types this plugin supports
	Command   string
	Config    []byte // JSON-encoded config (matches datatypes.JSON from database)
	Checksum  string
	IsActive  bool
}

// SupportsHookType checks if plugin data supports a specific hook type
func (pd *PluginData) SupportsHookType(hookType string) bool {
	// Check primary hook
	if pd.HookType == hookType {
		return true
	}
	// Check additional hooks
	for _, ht := range pd.HookTypes {
		if ht == hookType {
			return true
		}
	}
	return false
}

// HasAnySupportedGatewayHook returns true if this plugin declares at least one
// hook type that the microgateway supports. Plugins that only declare Studio-only
// hooks (studio_ui, agent, object_hooks) return false.
func (pd *PluginData) HasAnySupportedGatewayHook() bool {
	if interfaces.IsSupportedGatewayHookType(pd.HookType) {
		return true
	}
	for _, ht := range pd.HookTypes {
		if interfaces.IsSupportedGatewayHookType(ht) {
			return true
		}
	}
	return false
}

// LoadedPlugin represents a loaded plugin instance
type LoadedPlugin struct {
	ID            uint
	Name          string
	HookType      interfaces.HookType
	Client        *plugin.Client
	GRPCClient    pb.PluginServiceClient
	Config        map[string]interface{}
	Checksum      string
	IsHealthy     bool
	IsGlobal      bool                            // True for global plugins (vs per-LLM plugins)
	BuiltinPlugin interfaces.DataCollectionPlugin // For built-in plugins
}

// GlobalPlugin represents a global data collection plugin instance
type GlobalPlugin struct {
	Config       DataCollectionPluginConfig
	Client       *plugin.Client
	GRPCClient   pb.PluginServiceClient
	LoadedPlugin *LoadedPlugin
	IsHealthy    bool
}

// PluginStatus represents the current status of a plugin
type PluginStatus string

const (
	PluginStatusReady   PluginStatus = "ready"
	PluginStatusLoading PluginStatus = "loading"
	PluginStatusFailed  PluginStatus = "failed"
	PluginStatusUnknown PluginStatus = "unknown"
)

// PluginHealthStatus tracks the health and readiness of a plugin
type PluginHealthStatus struct {
	ID           uint          `json:"id"`
	Name         string        `json:"name"`
	Command      string        `json:"command"`
	HookType     string        `json:"hook_type"`
	Status       PluginStatus  `json:"status"`
	LastAttempt  time.Time     `json:"last_attempt"`
	ErrorMessage string        `json:"error_message,omitempty"`
	IsOCI        bool          `json:"is_oci"`
	IsCached     bool          `json:"is_cached,omitempty"` // For OCI plugins
	LoadTime     time.Duration `json:"load_time,omitempty"` // Time to load/pre-warm
}

// EndpointRoute stores the mapping from an HTTP route to a specific plugin endpoint
type EndpointRoute struct {
	PluginID       uint
	PluginName     string
	PluginSlug     string   // URL slug used in /plugins/{slug}/ (from config["slug"])
	Path           string   // Registration path pattern (e.g., "/*", "/.well-known/openid-configuration")
	Methods        []string // Allowed HTTP methods
	RequireAuth    bool
	StreamResponse bool
	Description    string
	Metadata       map[string]string
}

// PluginManager manages the lifecycle of plugins
type PluginManager struct {
	mu              sync.RWMutex
	loadedPlugins   map[uint]*LoadedPlugin          // Plugin ID -> loaded plugin
	llmPluginMap    map[uint][]uint                 // LLM ID -> Plugin IDs (ordered)
	pluginClients   map[uint]*plugin.Client         // Plugin ID -> go-plugin client
	reattachConfigs map[uint]*plugin.ReattachConfig // For reconnection
	service         PluginServiceInterface          // Database service
	handshakeConfig plugin.HandshakeConfig
	pluginMap       map[string]plugin.Plugin

	// Global data collection plugins
	globalDataPlugins       map[string]*GlobalPlugin // Plugin name -> global plugin
	dataCollectionHookTypes map[string][]string      // Plugin name -> hook types it handles

	// OCI plugin support
	ociClient *ociplugins.OCIPluginClient // OCI plugin client for fetching

	// Plugin health tracking
	pluginHealth         map[uint]*PluginHealthStatus // Plugin ID -> health status
	preWarmingInProgress map[uint]bool                // Plugin ID -> pre-warming status

	// Built-in plugin support
	edgeClient interface{} // Edge client for built-in plugins (interface to avoid import cycle)

	// Service broker for bidirectional plugin communication
	managementServer interface{} // MicrogatewayManagementServiceServer (interface to avoid import cycle)
	eventServer      interface{} // PluginEventServiceServer for plugin pub/sub (interface to avoid import cycle)

	// Session management for long-lived broker connections
	pluginSessions map[uint]context.CancelFunc // Plugin ID -> session cancel function

	// Custom endpoint routing
	endpointRoutes  map[string]*EndpointRoute // "METHOD:pluginName/subpath" -> route
	pluginEndpoints map[uint][]*EndpointRoute // Plugin ID -> registered routes (for cleanup)

	// Serializes concurrent ReconcilePlugins calls
	reconcileMu sync.Mutex
}

// HandshakeConfig is used to do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
// Updated to use unified AI_STUDIO_PLUGIN handshake for polyglot plugin support
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "AI_STUDIO_PLUGIN",
	MagicCookieValue: "v1",
}

// pluginLogger creates an hclog logger for go-plugin
// Using Debug level to see plugin output including OnSessionReady and event subscription logs
func pluginLogger() hclog.Logger {
	return hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Level:  hclog.Debug, // Show Debug level to see plugin logs
		Output: os.Stderr,   // Output to stderr so we can see plugin logs
	})
}

// NewPluginManager creates a new plugin manager instance
func NewPluginManager(pluginService PluginServiceInterface) *PluginManager {
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
		globalDataPlugins:       make(map[string]*GlobalPlugin),
		dataCollectionHookTypes: make(map[string][]string),
		ociClient:               nil, // Will be initialized when needed
		pluginHealth:            make(map[uint]*PluginHealthStatus),
		preWarmingInProgress:    make(map[uint]bool),
		pluginSessions:          make(map[uint]context.CancelFunc),
		endpointRoutes:          make(map[string]*EndpointRoute),
		pluginEndpoints:         make(map[uint][]*EndpointRoute),
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

// SetSecurityService sets the enterprise security service for OCI signature verification
func (pm *PluginManager) SetSecurityService(securityService ociplugins.SecurityService) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.ociClient != nil {
		pm.ociClient.SetSecurityService(securityService)
	}
}

// LoadPlugin loads a plugin by ID
func (pm *PluginManager) LoadPlugin(pluginID uint) (*LoadedPlugin, error) {
	startTime := time.Now()

	pm.mu.Lock()

	// Check if plugin is already loaded
	if existingPlugin, exists := pm.loadedPlugins[pluginID]; exists {
		pm.mu.Unlock()
		// Update health status for already loaded plugin (after releasing lock)
		pm.updatePluginHealthSafe(pluginID, PluginData{
			ID: pluginID, Name: existingPlugin.Name,
			HookType: string(existingPlugin.HookType),
		}, PluginStatusReady, nil, time.Since(startTime))
		return existingPlugin, nil
	}

	pm.mu.Unlock()

	// Get plugin from database (without holding lock)
	pluginData, err := pm.service.GetPlugin(pluginID)
	if err != nil {
		pm.updatePluginHealthSafe(pluginID, PluginData{ID: pluginID}, PluginStatusFailed, err, time.Since(startTime))
		return nil, fmt.Errorf("failed to get plugin from database: %w", err)
	}

	if !pluginData.IsActive {
		err := fmt.Errorf("plugin %d is not active", pluginID)
		pm.updatePluginHealthSafe(pluginID, pluginData, PluginStatusFailed, err, time.Since(startTime))
		return nil, err
	}

	// Mark as loading
	pm.updatePluginHealthSafe(pluginID, pluginData, PluginStatusLoading, nil, 0)

	// Reacquire lock for the rest of the operation
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check if another goroutine loaded it while we were unlocked
	if existingPlugin, exists := pm.loadedPlugins[pluginID]; exists {
		pm.updatePluginHealth(pluginID, pluginData, PluginStatusReady, nil, time.Since(startTime))
		return existingPlugin, nil
	}

	// Create plugin client based on command scheme
	client, err := pm.createPluginClient(pluginData.Command)
	if err != nil {
		pm.updatePluginHealth(pluginID, pluginData, PluginStatusFailed, err, time.Since(startTime))
		return nil, fmt.Errorf("failed to create plugin client: %w", err)
	}

	// Connect via gRPC with timeout and retry for external services
	rpcClient, err := pm.connectWithRetry(client, pluginData.Command)
	if err != nil {
		client.Kill()
		pm.updatePluginHealth(pluginID, pluginData, PluginStatusFailed, err, time.Since(startTime))
		return nil, fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// Request the plugin interface
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		client.Kill()
		pm.updatePluginHealth(pluginID, pluginData, PluginStatusFailed, err, time.Since(startTime))
		return nil, fmt.Errorf("failed to dispense plugin: %w", err)
	}

	// Cast to gRPC client - handle both wrapper and direct client
	var grpcClient pb.PluginServiceClient
	if wrapper, ok := raw.(*MicrogatewayPluginClient); ok {
		// New wrapper type that supports service broker
		grpcClient = wrapper
	} else if directClient, ok := raw.(pb.PluginServiceClient); ok {
		// Direct client (backward compatibility)
		grpcClient = directClient
	} else {
		client.Kill()
		err := fmt.Errorf("plugin does not implement PluginServiceClient interface")
		pm.updatePluginHealth(pluginID, pluginData, PluginStatusFailed, err, time.Since(startTime))
		return nil, err
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

	// Convert config values to strings for gRPC transport
	// For complex types (arrays, objects), JSON-encode them so plugins can parse them
	configStrings := make(map[string]string)
	for k, v := range config {
		switch val := v.(type) {
		case string:
			configStrings[k] = val
		case int, int64, uint, uint64, float64, bool:
			configStrings[k] = fmt.Sprintf("%v", val)
		default:
			// Complex types (arrays, maps) - JSON encode
			jsonBytes, err := json.Marshal(val)
			if err != nil {
				log.Warn().
					Str("key", k).
					Err(err).
					Msg("Failed to JSON encode config value, using string representation")
				configStrings[k] = fmt.Sprintf("%v", val)
			} else {
				configStrings[k] = string(jsonBytes)
			}
		}
	}

	// Add plugin ID to config
	configStrings["_plugin_id"] = fmt.Sprintf("%d", pluginID)

	// Setup service broker BEFORE Initialize (like AI Studio does)
	// This way the broker ID is available in the first and only Initialize call
	if pm.managementServer != nil {
		if clientWrapper, ok := raw.(*MicrogatewayPluginClient); ok {
			// Use SetupServiceBrokerWithEvents if event server is available
			var setupBrokerID uint32
			var err error
			log.Debug().
				Uint("plugin_id", pluginID).
				Bool("has_event_server", pm.eventServer != nil).
				Msg("Setting up service broker for plugin")
			if pm.eventServer != nil {
				setupBrokerID, err = clientWrapper.SetupServiceBrokerWithEvents(pm.managementServer, pm.eventServer)
			} else {
				log.Warn().
					Uint("plugin_id", pluginID).
					Msg("eventServer is nil - plugin will NOT be able to subscribe to events")
				setupBrokerID, err = clientWrapper.SetupServiceBroker(pm.managementServer)
			}
			if err != nil {
				log.Warn().
					Uint("plugin_id", pluginID).
					Err(err).
					Msg("Failed to setup service broker for plugin - service API will not be available")
			} else {
				log.Debug().
					Uint("plugin_id", pluginID).
					Uint32("broker_id", setupBrokerID).
					Bool("has_event_server", pm.eventServer != nil).
					Msg("Service broker setup complete")

				// Add broker ID to config so plugin receives it in Initialize
				configStrings["_service_broker_id"] = fmt.Sprintf("%d", setupBrokerID)
			}
		}
	} else {
		log.Warn().
			Uint("plugin_id", pluginID).
			Msg("managementServer is nil - skipping service broker setup")
	}

	// Initialize plugin with config (including broker ID if available)
	initResp, err := grpcClient.Initialize(context.Background(), &pb.InitRequest{
		Config: configStrings,
	})
	if err != nil {
		client.Kill()
		pm.updatePluginHealth(pluginID, pluginData, PluginStatusFailed, err, time.Since(startTime))
		return nil, fmt.Errorf("failed to initialize plugin: %w", err)
	}

	if !initResp.Success {
		client.Kill()
		err := fmt.Errorf("plugin initialization failed: %s", initResp.ErrorMessage)
		pm.updatePluginHealth(pluginID, pluginData, PluginStatusFailed, err, time.Since(startTime))
		return nil, err
	}

	// Create loaded plugin instance
	loadedPlugin := &LoadedPlugin{
		ID:         pluginData.ID,
		Name:       pluginData.Name,
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

	// Start session loop for long-lived broker access
	// Only if we have a client wrapper with session support
	if clientWrapper, ok := raw.(*MicrogatewayPluginClient); ok {
		brokerIDUint, _ := strconv.ParseUint(configStrings["_service_broker_id"], 10, 32)
		log.Debug().
			Uint("plugin_id", pluginID).
			Uint64("broker_id", brokerIDUint).
			Msg("Starting session loop for plugin")
		go pm.runPluginSessionLoop(pluginID, clientWrapper, uint32(brokerIDUint))
	} else {
		log.Warn().
			Uint("plugin_id", pluginID).
			Str("raw_type", fmt.Sprintf("%T", raw)).
			Msg("⚠️ Plugin does NOT support session loop (not a MicrogatewayPluginClient)")
	}

	// Fetch custom endpoint registrations outside the write lock (blocking gRPC call).
	// Store the plugin reference before releasing the lock so we can use it.
	pm.mu.Unlock()
	endpointRoutes := pm.fetchPluginEndpoints(pluginID, loadedPlugin)
	pm.updatePluginHealthSafe(pluginID, pluginData, PluginStatusReady, nil, time.Since(startTime))
	pm.mu.Lock()

	// Store routes under write lock (fast, no I/O)
	if len(endpointRoutes) > 0 {
		pm.storePluginEndpoints(pluginID, endpointRoutes)
	}

	log.Debug().
		Uint("plugin_id", pluginID).
		Str("plugin_name", pluginData.Name).
		Str("hook_type", pluginData.HookType).
		Dur("load_time", time.Since(startTime)).
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

	// Close session first (before Shutdown) to allow cleanup
	// Try to get client wrapper for session close
	if loadedPlugin.GRPCClient != nil {
		if clientWrapper, ok := loadedPlugin.GRPCClient.(*MicrogatewayPluginClient); ok {
			pm.mu.Unlock() // Release lock for session close
			pm.closePluginSession(pluginID, clientWrapper)
			pm.mu.Lock() // Re-acquire lock
		}
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

	// Unregister custom endpoints for this plugin
	pm.unregisterPluginEndpoints(pluginID)

	log.Debug().
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
		// Check if plugin supports this hook type
		if !pluginData.SupportsHookType(string(hookType)) {
			log.Debug().
				Uint("plugin_id", pluginData.ID).
				Str("plugin_name", pluginData.Name).
				Str("plugin_hook", pluginData.HookType).
				Strs("plugin_hooks", pluginData.HookTypes).
				Str("required_hook", string(hookType)).
				Msg("Plugin does not support required hook type (expected)")
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
				return nil, fmt.Errorf("failed to load plugin %d: %w", pluginData.ID, err)
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
				return nil, fmt.Errorf("invalid input type for post-auth hook: got %T, expected *interfaces.EnrichedRequest", result)
			}

			log.Debug().
				Str("plugin_name", plugin.Name).
				Int("input_body_len", len(enrichedReq.PluginRequest.Body)).
				Msg("🔗 Executing post-auth plugin in chain")

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
				// For post-auth hooks, update the enriched request with modifications
				// Don't convert to PluginResponse - keep it as EnrichedRequest for the next plugin in the chain
				log.Debug().
					Str("plugin_name", plugin.Name).
					Bool("resp_modified", resp.Modified).
					Int("original_body_len", len(enrichedReq.PluginRequest.Body)).
					Int("modified_body_len", len(resp.Body)).
					Msg("🔄 Post-auth plugin modified request, updating for next plugin")

				// Update the request fields with modifications
				if len(resp.Headers) > 0 {
					enrichedReq.PluginRequest.Headers = resp.Headers
				}
				if len(resp.Body) > 0 {
					enrichedReq.PluginRequest.Body = resp.Body
				}
				// result stays as enrichedReq for the next plugin
			}

			// Apply context updates from plugin (e.g., upstream_override for DLB)
			if len(resp.ContextUpdates) > 0 {
				if pluginCtx.Metadata == nil {
					pluginCtx.Metadata = make(map[string]interface{})
				}
				for key, value := range resp.ContextUpdates {
					pluginCtx.Metadata[key] = value
					log.Debug().
						Str("plugin_name", plugin.Name).
						Str("key", key).
						Str("value", value).
						Msg("📝 Plugin set context update")
				}
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
			log.Debug().
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
				log.Debug().
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
	pm.pluginHealth = make(map[uint]*PluginHealthStatus)
	pm.preWarmingInProgress = make(map[uint]bool)
	pm.endpointRoutes = make(map[string]*EndpointRoute)
	pm.pluginEndpoints = make(map[uint][]*EndpointRoute)

	// Shutdown OCI client if available
	if pm.ociClient != nil {
		log.Debug().Msg("Shutting down OCI plugin client...")
		if err := pm.ociClient.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown OCI plugin client")
			errors = append(errors, fmt.Errorf("failed to shutdown OCI client: %w", err))
		} else {
			log.Debug().Msg("OCI plugin client shutdown completed")
		}
	}

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

	log.Debug().
		Uint("plugin_id", pluginID).
		Msg("Reattach config saved for plugin")

	return nil
}

// Security validation functions for plugin paths
var (
	// Pattern to detect shell metacharacters that could be used for command injection
	shellMetacharPattern = regexp.MustCompile(`[;&|$(){}[\]<>?*!~\x60\\]`)

	// Default allowed plugin directories (can be configured)
	defaultAllowedDirs = []string{
		"/opt/microgateway/plugins",
		"./plugins",
		"plugins/",
	}
)

// validatePluginPath performs security validation on plugin executable paths
func validatePluginPath(cmdPath string) error {
	// Security: Check for shell metacharacters that could indicate command injection
	if shellMetacharPattern.MatchString(cmdPath) {
		return fmt.Errorf("🔒 SECURITY: Plugin command contains shell metacharacters: %s", cmdPath)
	}

	// Security: Resolve to absolute path and check for directory traversal
	absPath, err := filepath.Abs(cmdPath)
	if err != nil {
		return fmt.Errorf("🔒 SECURITY: Cannot resolve plugin path: %w", err)
	}

	// Security: Check if resolved path is within allowed directories
	if !isPathInAllowedDirectories(absPath) {
		return fmt.Errorf("🔒 SECURITY: Plugin path '%s' is not in allowed directories. Use oci:// or grpc:// schemes for external plugins", absPath)
	}

	// Security: Check that the file exists and is a regular file (not a symlink to dangerous locations)
	fileInfo, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("🔒 SECURITY: Plugin executable not found: %s", absPath)
		}
		return fmt.Errorf("🔒 SECURITY: Cannot stat plugin file: %w", err)
	}

	// Security: Reject symbolic links (could point to dangerous locations)
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("🔒 SECURITY: Plugin cannot be a symbolic link for security reasons: %s", absPath)
	}

	// Security: Verify file is executable
	if fileInfo.Mode()&0111 == 0 {
		return fmt.Errorf("🔒 SECURITY: Plugin file is not executable: %s", absPath)
	}

	log.Debug().
		Str("plugin_path", absPath).
		Msg("✅ Plugin path security validation passed")

	return nil
}

// isPathInAllowedDirectories checks if the given path is within allowed plugin directories
func isPathInAllowedDirectories(absPath string) bool {
	if os.Getenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS") == "1" {
		log.Warn().Msg("⚠️ WARNING: MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS is set - all plugin paths are allowed, disabling security checks")
		return true
	}

	for _, allowedDir := range defaultAllowedDirs {
		allowedAbsDir, err := filepath.Abs(allowedDir)
		if err != nil {
			continue
		}

		// Check if the plugin path is within the allowed directory
		relPath, err := filepath.Rel(allowedAbsDir, absPath)
		if err != nil {
			continue
		}

		// If relative path doesn't start with "..", it's within the allowed directory
		if !strings.HasPrefix(relPath, "..") {
			return true
		}
	}
	return false
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

		log.Debug().
			Str("command", command).
			Str("address", reattachConfig.Addr.String()).
			Msg("Creating client for external gRPC plugin")

		return plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig:  pm.handshakeConfig,
			Plugins:          pm.pluginMap,
			Reattach:         reattachConfig,
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Logger:           pluginLogger(),
		}), nil
	} else {
		// Local executable - use exec.Command with security validation
		cmdPath := command
		if strings.HasPrefix(command, "file://") {
			cmdPath = strings.TrimPrefix(command, "file://")
		}

		// Security: Validate plugin path to prevent command injection
		if err := validatePluginPath(cmdPath); err != nil {
			return nil, fmt.Errorf("plugin security validation failed: %w", err)
		}

		log.Debug().
			Str("command", command).
			Str("path", cmdPath).
			Msg("✅ Creating client for validated local plugin executable")

		return plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig:  pm.handshakeConfig,
			Plugins:          pm.pluginMap,
			Cmd:              exec.Command(cmdPath),
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Logger:           pluginLogger(),
			SyncStdout:       os.Stdout, // Forward plugin stdout to microgateway
			SyncStderr:       os.Stderr, // Forward plugin stderr to microgateway
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

	var localPlugin *ociplugins.LocalPlugin

	// Check if plugin is already cached (hot path optimization)
	if pm.ociClient.HasPlugin(ref.Digest, params.Architecture) {
		log.Debug().
			Str("command", command).
			Str("digest", ref.Digest).
			Str("architecture", params.Architecture).
			Msg("Using cached OCI plugin")

		// Get cached plugin without refetching
		localPlugin, err = pm.ociClient.GetPlugin(ref, params)
		if err != nil {
			log.Warn().
				Str("command", command).
				Err(err).
				Msg("Failed to get cached OCI plugin, will re-fetch")
			// Fall through to fetch logic
		}
	}

	// If not cached or cache retrieval failed, fetch from registry
	if localPlugin == nil {
		log.Debug().
			Str("command", command).
			Str("registry", ref.Registry).
			Str("repository", ref.Repository).
			Str("digest", ref.Digest).
			Str("architecture", params.Architecture).
			Msg("Fetching OCI plugin from registry")

		localPlugin, err = pm.ociClient.FetchPlugin(context.Background(), ref, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch OCI plugin: %w", err)
		}

		log.Debug().
			Str("command", command).
			Str("local_path", localPlugin.ExecutablePath).
			Bool("verified", localPlugin.Verified).
			Msg("OCI plugin fetched successfully from registry")
	} else {
		log.Debug().
			Str("command", command).
			Str("local_path", localPlugin.ExecutablePath).
			Bool("verified", localPlugin.Verified).
			Msg("Using cached OCI plugin")
	}

	// Create go-plugin client with the local executable
	return plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pm.handshakeConfig,
		Plugins:          pm.pluginMap,
		Cmd:              exec.Command(localPlugin.ExecutablePath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           pluginLogger(),
		SyncStdout:       os.Stdout, // Forward plugin stdout to microgateway
		SyncStderr:       os.Stderr, // Forward plugin stderr to microgateway
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
			log.Debug().
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

		log.Debug().
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
		Logger:           pluginLogger(),
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

	log.Debug().
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

		// Check if this is the built-in analytics pulse plugin
		var globalPlugin *GlobalPlugin
		var err error

		if cfg.Name == "analytics_pulse" {
			// Skip built-in analytics pulse plugin during initial load
			// It will be loaded later when edge client is available
			log.Debug().Str("plugin", cfg.Name).Msg("Deferring built-in analytics pulse plugin load until edge client is available")
			continue
		} else {
			// Load external plugin from path
			globalPlugin, err = pm.loadGlobalPluginFromConfig(cfg)
		}

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

		log.Debug().
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

// SetEdgeClient sets the edge client for built-in plugins that need gRPC access
func (pm *PluginManager) SetEdgeClient(edgeClient interface{}) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.edgeClient = edgeClient
	log.Debug().Msg("Edge client set for plugin manager built-in plugins")
}

// SetManagementServer sets the management server for service broker setup
func (pm *PluginManager) SetManagementServer(server interface{}) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.managementServer = server
	log.Debug().Msg("Management server set for plugin manager service broker")
}

// GetManagementServer returns the management server for external wiring
// This is used to wire the control payload queue from main.go
func (pm *PluginManager) GetManagementServer() interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.managementServer
}

// SetEventServer sets the plugin event server for pub/sub support
func (pm *PluginManager) SetEventServer(server interface{}) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.eventServer = server
	log.Debug().Msg("Event server set for plugin manager service broker")
}

// GetEventServer returns the plugin event server
func (pm *PluginManager) GetEventServer() interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.eventServer
}

// SetEventBus creates a PluginEventServer from the given event bus and node ID.
// This is a convenience method that creates the server internally.
// The bus should implement eventbridge.Bus interface.
func (pm *PluginManager) SetEventBus(bus interface{}, nodeID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if bus == nil {
		log.Warn().Msg("SetEventBus: bus is nil")
		return
	}

	// Cast to eventbridge.Bus
	eventBus, ok := bus.(eventbridge.Bus)
	if !ok {
		log.Warn().Msg("SetEventBus: provided bus does not implement eventbridge.Bus interface")
		return
	}

	pm.eventServer = sdk.NewPluginEventServer(eventBus, nodeID)
	log.Debug().
		Str("node_id", nodeID).
		Msg("Plugin event server created from event bus")
}

// LoadDeferredBuiltinPlugins loads any built-in plugins that were deferred during initial load
func (pm *PluginManager) LoadDeferredBuiltinPlugins(configs []DataCollectionPluginConfig) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		// Only load analytics_pulse plugin that was deferred
		if cfg.Name == "analytics_pulse" {
			log.Debug().Str("plugin", cfg.Name).Msg("Loading deferred built-in analytics pulse plugin")

			globalPlugin, err := pm.loadBuiltinAnalyticsPulsePlugin(cfg)
			if err != nil {
				log.Error().
					Str("plugin", cfg.Name).
					Err(err).
					Msg("Failed to load deferred built-in analytics pulse plugin")
				return err
			}

			// Store global plugin
			pm.globalDataPlugins[cfg.Name] = globalPlugin
			pm.dataCollectionHookTypes[cfg.Name] = cfg.HookTypes

			log.Debug().
				Str("plugin", cfg.Name).
				Strs("hook_types", cfg.HookTypes).
				Msg("Deferred built-in analytics pulse plugin loaded successfully")
		}
	}

	return nil
}

// loadBuiltinAnalyticsPulsePlugin loads the built-in analytics pulse plugin
func (pm *PluginManager) loadBuiltinAnalyticsPulsePlugin(cfg DataCollectionPluginConfig) (*GlobalPlugin, error) {
	if pm.edgeClient == nil {
		return nil, fmt.Errorf("edge client not available for built-in analytics pulse plugin")
	}

	// Get gRPC client from edge client
	var grpcClient configpb.ConfigurationSyncServiceClient
	if client, ok := pm.edgeClient.(interface {
		GetGRPCClient() configpb.ConfigurationSyncServiceClient
	}); ok {
		grpcClient = client.GetGRPCClient()
	} else {
		return nil, fmt.Errorf("edge client does not provide gRPC client interface")
	}

	// Extract edge ID and namespace from edge client
	edgeID := "unknown"
	edgeNamespace := ""

	if edgeInfo, ok := pm.edgeClient.(interface {
		GetEdgeID() string
		GetEdgeNamespace() string
	}); ok {
		edgeID = edgeInfo.GetEdgeID()
		edgeNamespace = edgeInfo.GetEdgeNamespace()
	}

	// Create the built-in analytics pulse plugin
	pulsePlugin, err := plugins.NewAnalyticsPulsePlugin(edgeID, edgeNamespace, grpcClient, cfg.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create analytics pulse plugin: %w", err)
	}

	// Initialize the plugin
	if err := pulsePlugin.Initialize(cfg.Config); err != nil {
		return nil, fmt.Errorf("failed to initialize analytics pulse plugin: %w", err)
	}

	// Create global plugin wrapper
	globalPlugin := &GlobalPlugin{
		Config:     cfg,
		Client:     nil, // Built-in plugin doesn't use go-plugin client
		GRPCClient: nil, // Built-in plugin doesn't use external gRPC
		IsHealthy:  true,
		LoadedPlugin: &LoadedPlugin{
			Name:       cfg.Name,
			HookType:   interfaces.HookTypeDataCollection,
			Client:     nil,
			GRPCClient: nil,
			Config:     cfg.Config,
			IsHealthy:  true,
			IsGlobal:   true,
		},
	}

	// Store reference to built-in plugin for execution
	globalPlugin.LoadedPlugin.BuiltinPlugin = pulsePlugin

	log.Debug().
		Str("plugin", cfg.Name).
		Str("edge_id", edgeID).
		Str("edge_namespace", edgeNamespace).
		Msg("Built-in analytics pulse plugin loaded successfully")

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
	// Check if this is a built-in plugin
	if plugin.LoadedPlugin.BuiltinPlugin != nil {
		return pm.executeBuiltinDataCollectionPlugin(ctx, plugin, hookType, data)
	}

	// Handle external plugins via gRPC
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

// executeBuiltinDataCollectionPlugin executes a built-in data collection plugin
func (pm *PluginManager) executeBuiltinDataCollectionPlugin(ctx context.Context, plugin *GlobalPlugin, hookType string, data interface{}) error {
	builtinPlugin := plugin.LoadedPlugin.BuiltinPlugin

	// Create plugin context
	pluginCtx := &interfaces.PluginContext{
		RequestID: "builtin-execution",
	}

	switch hookType {
	case "proxy_log":
		if proxyData, ok := data.(*interfaces.ProxyLogData); ok {
			_, err := builtinPlugin.HandleProxyLog(ctx, proxyData, pluginCtx)
			return err
		}
	case "analytics":
		if analyticsData, ok := data.(*interfaces.AnalyticsData); ok {
			_, err := builtinPlugin.HandleAnalytics(ctx, analyticsData, pluginCtx)
			return err
		}
	case "budget":
		if budgetData, ok := data.(*interfaces.BudgetUsageData); ok {
			_, err := builtinPlugin.HandleBudgetUsage(ctx, budgetData, pluginCtx)
			return err
		}
	}

	return fmt.Errorf("unsupported hook type for built-in plugin: %s", hookType)
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
		LlmId:                  uint32(data.LLMID),
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
		log.Debug().Str("plugin", name).Msg("Unloaded global data collection plugin")
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

	log.Debug().
		Str("reference", ref.FullReference()).
		Msg("OCI plugin pre-fetched successfully")

	return nil
}

// PrewarmAllPlugins loads plugins assigned to LLMs at startup to establish event subscriptions.
// This is called after SetEventBus to ensure plugins can subscribe to events.
// Only plugins that are actually assigned to LLMs on this edge are loaded.
func (pm *PluginManager) PrewarmAllPlugins(ctx context.Context) error {
	log.Debug().Msg("Starting plugin pre-warming for event subscriptions")

	desiredState, err := pm.getDesiredPluginState()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get desired plugin state for pre-warming")
		return fmt.Errorf("failed to get desired plugin state for pre-warming: %w", err)
	}

	pluginsToLoad := make([]PluginData, 0, len(desiredState))
	for _, p := range desiredState {
		pluginsToLoad = append(pluginsToLoad, p)
	}

	log.Debug().Int("unique_plugins", len(pluginsToLoad)).Msg("Found unique plugins to pre-warm")

	var loadedCount int
	var preWarmErrors []error

	for _, plugin := range pluginsToLoad {
		// Skip plugins that are already loaded
		pm.mu.RLock()
		_, exists := pm.loadedPlugins[plugin.ID]
		pm.mu.RUnlock()

		if exists {
			log.Debug().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Msg("Plugin already loaded, skipping pre-warm")
			continue
		}

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("command", plugin.Command).
			Msg("Pre-warming plugin for event subscriptions")

		// Load the plugin - this will start the session loop and trigger OnSessionReady
		startTime := time.Now()
		_, err := pm.LoadPlugin(plugin.ID)
		if err != nil {
			log.Error().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Err(err).
				Msg("Failed to pre-warm plugin")
			preWarmErrors = append(preWarmErrors, fmt.Errorf("plugin %d (%s): %w", plugin.ID, plugin.Name, err))
		} else {
			loadedCount++
			log.Debug().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Dur("load_time", time.Since(startTime)).
				Msg("Plugin pre-warmed successfully")
		}
	}

	if len(preWarmErrors) > 0 {
		log.Warn().
			Int("total_plugins", len(pluginsToLoad)).
			Int("loaded_plugins", loadedCount).
			Int("failed_plugins", len(preWarmErrors)).
			Msg("Plugin pre-warming completed with some errors")
	} else {
		log.Info().
			Int("plugins_prewarmed", loadedCount).
			Int("desired_plugins", len(pluginsToLoad)).
			Msg("Plugin pre-warming completed successfully - event subscriptions should be active")
	}

	return nil
}

// PreWarmOCIPlugins pre-warms all OCI plugins found in the database
func (pm *PluginManager) PreWarmOCIPlugins(ctx context.Context) error {
	log.Debug().Msg("Starting OCI plugin pre-warming")

	if pm.ociClient == nil {
		log.Debug().Msg("OCI client not available, skipping OCI plugin pre-warming")
		return nil
	}

	// Get all plugins from database
	allPlugins, err := pm.service.GetAllPlugins()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get plugins for pre-warming")
		return fmt.Errorf("failed to get plugins for pre-warming: %w", err)
	}

	var ociPluginCount int
	var preWarmErrors []error

	// Pre-warm OCI plugins that have gateway-supported hook types
	for _, plugin := range allPlugins {
		if !strings.HasPrefix(plugin.Command, "oci://") {
			continue
		}

		// Skip plugins that only declare Studio-only hooks (studio_ui, agent, object_hooks)
		if !plugin.HasAnySupportedGatewayHook() {
			log.Debug().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Str("hook_type", plugin.HookType).
				Msg("Skipping OCI pre-warm for non-gateway plugin")
			continue
		}

		ociPluginCount++

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("command", plugin.Command).
			Msg("Pre-warming OCI plugin")

		// Mark as loading during pre-warming
		pm.updatePluginHealthSafe(plugin.ID, plugin, PluginStatusLoading, nil, 0)

		// Pre-fetch the plugin
		startTime := time.Now()
		if err := pm.PreFetchOCIPlugin(plugin.Command); err != nil {
			log.Error().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Err(err).
				Msg("Failed to pre-warm OCI plugin")

			pm.updatePluginHealthSafe(plugin.ID, plugin, PluginStatusFailed, err, time.Since(startTime))
			preWarmErrors = append(preWarmErrors, fmt.Errorf("plugin %d (%s): %w", plugin.ID, plugin.Name, err))
		} else {
			log.Debug().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Dur("pre_warm_time", time.Since(startTime)).
				Msg("OCI plugin pre-warmed successfully")

			pm.updatePluginHealthSafe(plugin.ID, plugin, PluginStatusReady, nil, time.Since(startTime))
		}
	}

	if len(preWarmErrors) > 0 {
		log.Error().
			Int("total_oci_plugins", ociPluginCount).
			Int("failed_plugins", len(preWarmErrors)).
			Msg("OCI plugin pre-warming completed with errors")
		return fmt.Errorf("failed to pre-warm %d out of %d OCI plugins", len(preWarmErrors), ociPluginCount)
	}

	log.Debug().
		Int("oci_plugins_prewarmed", ociPluginCount).
		Msg("OCI plugin pre-warming completed successfully")

	return nil
}

// updatePluginHealthSafe updates the health status for a plugin with internal locking
func (pm *PluginManager) updatePluginHealthSafe(pluginID uint, pluginData PluginData, status PluginStatus, err error, loadTime time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.updatePluginHealthUnsafe(pluginID, pluginData, status, err, loadTime)
}

// updatePluginHealth updates the health status for a plugin (assumes lock is held)
func (pm *PluginManager) updatePluginHealth(pluginID uint, pluginData PluginData, status PluginStatus, err error, loadTime time.Duration) {
	pm.updatePluginHealthUnsafe(pluginID, pluginData, status, err, loadTime)
}

// updatePluginHealthUnsafe updates the health status for a plugin (no locking - assumes lock is held)
func (pm *PluginManager) updatePluginHealthUnsafe(pluginID uint, pluginData PluginData, status PluginStatus, err error, loadTime time.Duration) {

	health := &PluginHealthStatus{
		ID:          pluginID,
		Name:        pluginData.Name,
		Command:     pluginData.Command,
		HookType:    pluginData.HookType,
		Status:      status,
		LastAttempt: time.Now(),
		IsOCI:       strings.HasPrefix(pluginData.Command, "oci://"),
		LoadTime:    loadTime,
	}

	if err != nil {
		health.ErrorMessage = err.Error()
	}

	// For OCI plugins, check if cached
	if health.IsOCI && pm.ociClient != nil {
		if ref, params, parseErr := ociplugins.ParseOCICommand(pluginData.Command); parseErr == nil {
			health.IsCached = pm.ociClient.HasPlugin(ref.Digest, params.Architecture)
		}
	}

	pm.pluginHealth[pluginID] = health

	log.Debug().
		Uint("plugin_id", pluginID).
		Str("plugin_name", pluginData.Name).
		Str("status", string(status)).
		Bool("is_oci", health.IsOCI).
		Bool("is_cached", health.IsCached).
		Msg("Plugin health status updated")
}

// GetPluginHealth returns health status for all tracked plugins
func (pm *PluginManager) GetPluginHealth() map[uint]*PluginHealthStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Return a copy to avoid concurrent access issues
	health := make(map[uint]*PluginHealthStatus)
	for id, status := range pm.pluginHealth {
		statusCopy := *status
		health[id] = &statusCopy
	}

	return health
}

// IsAllPluginsReady checks if all tracked plugins are ready
func (pm *PluginManager) IsAllPluginsReady() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, health := range pm.pluginHealth {
		if health.Status == PluginStatusFailed || health.Status == PluginStatusLoading {
			return false
		}
	}

	return true
}

// GetPluginHealthSummary returns a summary of plugin health
func (pm *PluginManager) GetPluginHealthSummary() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	summary := map[string]interface{}{
		"total_plugins":      len(pm.pluginHealth),
		"ready_plugins":      0,
		"loading_plugins":    0,
		"failed_plugins":     0,
		"unknown_plugins":    0,
		"oci_plugins":        0,
		"cached_oci_plugins": 0,
	}

	for _, health := range pm.pluginHealth {
		switch health.Status {
		case PluginStatusReady:
			summary["ready_plugins"] = summary["ready_plugins"].(int) + 1
		case PluginStatusLoading:
			summary["loading_plugins"] = summary["loading_plugins"].(int) + 1
		case PluginStatusFailed:
			summary["failed_plugins"] = summary["failed_plugins"].(int) + 1
		case PluginStatusUnknown:
			summary["unknown_plugins"] = summary["unknown_plugins"].(int) + 1
		}

		if health.IsOCI {
			summary["oci_plugins"] = summary["oci_plugins"].(int) + 1
			if health.IsCached {
				summary["cached_oci_plugins"] = summary["cached_oci_plugins"].(int) + 1
			}
		}
	}

	summary["all_ready"] = summary["failed_plugins"].(int) == 0 && summary["loading_plugins"].(int) == 0

	return summary
}

// runPluginSessionLoop runs the session loop for a plugin.
// This keeps calling OpenSession in a loop to maintain broker connectivity.
// The loop exits when the session is explicitly closed or the plugin is unloaded.
func (pm *PluginManager) runPluginSessionLoop(pluginID uint, client *MicrogatewayPluginClient, brokerID uint32) {
	// Create a cancellable context for this session
	ctx, cancel := context.WithCancel(context.Background())

	// Store cancel function for cleanup during unload
	pm.mu.Lock()
	pm.pluginSessions[pluginID] = cancel
	pm.mu.Unlock()

	defer func() {
		pm.mu.Lock()
		delete(pm.pluginSessions, pluginID)
		pm.mu.Unlock()
	}()

	log.Debug().
		Uint("plugin_id", pluginID).
		Uint32("broker_id", brokerID).
		Msg("Starting plugin session loop")

	sessionTimeoutMs := int32(30000) // 30 second sessions

	for {
		select {
		case <-ctx.Done():
			log.Debug().
				Uint("plugin_id", pluginID).
				Msg("Plugin session loop cancelled")
			return
		default:
		}

		log.Debug().
			Uint("plugin_id", pluginID).
			Uint32("broker_id", brokerID).
			Msg("Calling OpenSession RPC on plugin")

		// Call OpenSession - this blocks until timeout or explicit close
		resp, err := client.OpenSession(ctx, &pb.OpenSessionRequest{
			ServiceBrokerId: brokerID,
			PluginId:        uint32(pluginID),
			TimeoutMs:       sessionTimeoutMs,
		})

		log.Debug().
			Uint("plugin_id", pluginID).
			Bool("resp_nil", resp == nil).
			Bool("err_nil", err == nil).
			Msg("OpenSession RPC returned")

		if err != nil {
			// Check if context was cancelled
			select {
			case <-ctx.Done():
				log.Debug().
					Uint("plugin_id", pluginID).
					Msg("Plugin session loop cancelled during OpenSession")
				return
			default:
			}

			log.Error().
				Uint("plugin_id", pluginID).
				Err(err).
				Msg("❌ OpenSession failed, retrying after delay")

			// Wait before retrying on error
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		// Check close reason
		switch resp.CloseReason {
		case pb.OpenSessionResponse_TIMEOUT:
			// Normal timeout - immediately re-open session
			log.Debug().
				Uint("plugin_id", pluginID).
				Str("session_id", resp.SessionId).
				Msg("Plugin session timed out, renewing")
			continue

		case pb.OpenSessionResponse_EXPLICIT_CLOSE:
			// Session was explicitly closed - exit loop
			log.Debug().
				Uint("plugin_id", pluginID).
				Str("session_id", resp.SessionId).
				Msg("Plugin session explicitly closed, exiting session loop")
			return

		case pb.OpenSessionResponse_PLUGIN_ERROR:
			// Plugin error - log and retry
			log.Warn().
				Uint("plugin_id", pluginID).
				Str("session_id", resp.SessionId).
				Str("error", resp.ErrorMessage).
				Msg("Plugin session error, retrying after delay")

			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue

		default:
			// Unknown reason - log and continue
			log.Debug().
				Uint("plugin_id", pluginID).
				Str("session_id", resp.SessionId).
				Int32("close_reason", int32(resp.CloseReason)).
				Msg("Plugin session closed with unknown reason, renewing")
			continue
		}
	}
}

// closePluginSession closes the session for a plugin during unload
func (pm *PluginManager) closePluginSession(pluginID uint, client *MicrogatewayPluginClient) {
	// Cancel the session loop context
	pm.mu.Lock()
	if cancel, exists := pm.pluginSessions[pluginID]; exists {
		cancel()
	}
	pm.mu.Unlock()

	// Call CloseSession to notify the plugin
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.CloseSession(ctx, &pb.CloseSessionRequest{
		Reason: "unload",
	})
	if err != nil {
		log.Debug().
			Uint("plugin_id", pluginID).
			Err(err).
			Msg("Failed to close plugin session (may have already ended)")
	} else {
		log.Debug().
			Uint("plugin_id", pluginID).
			Msg("Plugin session closed")
	}
}

// --- Custom Endpoint Registration & Dispatch ---

// validHTTPMethods is the set of HTTP methods that plugin endpoints may declare.
var validHTTPMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "OPTIONS": true, "HEAD": true,
}

// fetchPluginEndpoints queries a plugin for its custom endpoint registrations via gRPC.
// This performs a blocking network call and must NOT be called while holding pm.mu.
// Returns nil if the plugin does not implement custom endpoints or has no registrations.
func (pm *PluginManager) fetchPluginEndpoints(pluginID uint, lp *LoadedPlugin) []*EndpointRoute {
	if lp.GRPCClient == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := lp.GRPCClient.GetEndpointRegistrations(ctx, &pb.GetEndpointRegistrationsRequest{})
	if err != nil {
		log.Debug().
			Uint("plugin_id", pluginID).
			Str("plugin_name", lp.Name).
			Err(err).
			Msg("Plugin does not provide custom endpoint registrations (OK)")
		return nil
	}

	if len(resp.Registrations) == 0 {
		return nil
	}

	// Resolve slug from plugin config
	slug := ""
	if lp.Config != nil {
		if s, ok := lp.Config["slug"].(string); ok && s != "" {
			slug = s
		}
	}
	if slug == "" {
		log.Warn().
			Uint("plugin_id", pluginID).
			Str("plugin_name", lp.Name).
			Msg("Plugin has endpoint registrations but no 'slug' in config — endpoints will not be reachable. Set config.slug to the desired URL path component.")
		return nil
	}

	log.Debug().
		Uint("plugin_id", pluginID).
		Str("plugin_name", lp.Name).
		Str("slug", slug).
		Int("registrations", len(resp.Registrations)).
		Msg("Fetched custom plugin endpoint registrations")

	var routes []*EndpointRoute
	for _, reg := range resp.Registrations {
		if !isValidEndpointPath(reg.Path) {
			log.Warn().
				Uint("plugin_id", pluginID).
				Str("path", reg.Path).
				Msg("Skipping invalid endpoint path from plugin")
			continue
		}

		methods := validateHTTPMethods(reg.Methods)
		if len(methods) == 0 {
			log.Warn().
				Uint("plugin_id", pluginID).
				Str("path", reg.Path).
				Msg("Skipping endpoint registration with no valid HTTP methods — plugin must explicitly declare methods")
			continue
		}

		routes = append(routes, &EndpointRoute{
			PluginID:       pluginID,
			PluginName:     lp.Name,
			PluginSlug:     slug,
			Path:           reg.Path,
			Methods:        methods,
			RequireAuth:    reg.RequireAuth,
			StreamResponse: reg.StreamResponse,
			Description:    reg.Description,
			Metadata:       reg.Metadata,
		})
	}
	return routes
}

// storePluginEndpoints writes pre-fetched routes into the routing tables.
// Assumes pm.mu write lock is held by the caller.
func (pm *PluginManager) storePluginEndpoints(pluginID uint, routes []*EndpointRoute) {
	for _, route := range routes {
		for _, method := range route.Methods {
			key := endpointRouteKey(method, route.PluginSlug, route.Path)
			pm.endpointRoutes[key] = route
		}
		pm.pluginEndpoints[pluginID] = append(pm.pluginEndpoints[pluginID], route)

		log.Debug().
			Uint("plugin_id", pluginID).
			Str("slug", route.PluginSlug).
			Str("path", route.Path).
			Strs("methods", route.Methods).
			Msg("Registered custom plugin endpoint")
	}
}

// unregisterPluginEndpoints removes all endpoint routes for a given plugin.
// Assumes pm.mu is held by the caller.
func (pm *PluginManager) unregisterPluginEndpoints(pluginID uint) {
	routes, exists := pm.pluginEndpoints[pluginID]
	if !exists {
		return
	}

	pluginName := ""
	if lp, ok := pm.loadedPlugins[pluginID]; ok {
		pluginName = lp.Name
	}

	for _, route := range routes {
		for _, method := range route.Methods {
			key := endpointRouteKey(method, route.PluginSlug, route.Path)
			delete(pm.endpointRoutes, key)
		}
	}
	delete(pm.pluginEndpoints, pluginID)

	log.Debug().
		Uint("plugin_id", pluginID).
		Str("plugin_name", pluginName).
		Int("routes_removed", len(routes)).
		Msg("Unregistered custom plugin endpoints")
}

// RefreshAllEndpoints re-queries every loaded plugin for endpoint registrations.
// This is useful after a bulk config reload.
// gRPC calls are performed outside the lock to avoid blocking request routing.
func (pm *PluginManager) RefreshAllEndpoints() {
	// Phase 1: snapshot loaded plugins under read lock (no I/O)
	pm.mu.RLock()
	type pluginSnapshot struct {
		id     uint
		plugin *LoadedPlugin
	}
	snapshots := make([]pluginSnapshot, 0, len(pm.loadedPlugins))
	for id, lp := range pm.loadedPlugins {
		snapshots = append(snapshots, pluginSnapshot{id: id, plugin: lp})
	}
	pm.mu.RUnlock()

	// Phase 2: fetch registrations outside lock (blocking gRPC, potentially slow)
	type fetchResult struct {
		pluginID uint
		routes   []*EndpointRoute
	}
	var results []fetchResult
	for _, snap := range snapshots {
		routes := pm.fetchPluginEndpoints(snap.id, snap.plugin)
		if len(routes) > 0 {
			results = append(results, fetchResult{pluginID: snap.id, routes: routes})
		}
	}

	// Phase 3: swap route tables under write lock (fast, no I/O)
	pm.mu.Lock()
	pm.endpointRoutes = make(map[string]*EndpointRoute)
	pm.pluginEndpoints = make(map[uint][]*EndpointRoute)
	for _, res := range results {
		pm.storePluginEndpoints(res.pluginID, res.routes)
	}
	pm.mu.Unlock()

	log.Debug().
		Int("total_routes", len(pm.endpointRoutes)).
		Int("plugins_with_endpoints", len(pm.pluginEndpoints)).
		Msg("Refreshed all plugin endpoint registrations")
}

// getDesiredPluginState queries the database for all plugins that should be active
// on this gateway (LLM-associated + standalone endpoint plugins) and returns a map
// of pluginID -> PluginData. Returns an error if any database query fails, to avoid
// operating on partial state.
func (pm *PluginManager) getDesiredPluginState() (map[uint]PluginData, error) {
	plugins, err := pm.service.GetAllActiveGatewayPlugins()
	if err != nil {
		return nil, fmt.Errorf("failed to get desired plugin state: %w", err)
	}

	result := make(map[uint]PluginData, len(plugins))
	for _, p := range plugins {
		result[p.ID] = p
	}
	return result, nil
}

// ReconcilePlugins compares currently loaded plugins with the database state and
// reconciles differences: unloads removed plugins, reloads changed plugins, loads
// new plugins, and refreshes endpoint registrations. Call this after a configuration
// sync to ensure running plugin processes match the latest snapshot.
func (pm *PluginManager) ReconcilePlugins(ctx context.Context) error {
	pm.reconcileMu.Lock()
	defer pm.reconcileMu.Unlock()

	log.Info().Msg("Starting plugin reconciliation after configuration sync")

	// Phase 1: Snapshot currently loaded (non-global, non-builtin) plugins
	pm.mu.RLock()
	loadedChecksums := make(map[uint]string, len(pm.loadedPlugins))
	for id, lp := range pm.loadedPlugins {
		if lp.IsGlobal || lp.BuiltinPlugin != nil {
			continue
		}
		loadedChecksums[id] = lp.Checksum
	}
	pm.mu.RUnlock()

	// Phase 2: Build desired state from database (abort on any DB error)
	desiredPlugins, err := pm.getDesiredPluginState()
	if err != nil {
		return fmt.Errorf("reconcile aborted: %w", err)
	}

	// Phase 3: Compute diff
	var toUnload []uint
	var toReload []uint

	for id, loadedChecksum := range loadedChecksums {
		desired, exists := desiredPlugins[id]
		if !exists {
			toUnload = append(toUnload, id)
		} else if loadedChecksum != desired.Checksum {
			toReload = append(toReload, id)
		}
	}

	log.Info().
		Int("to_unload", len(toUnload)).
		Int("to_reload", len(toReload)).
		Int("loaded_count", len(loadedChecksums)).
		Int("desired_count", len(desiredPlugins)).
		Msg("Plugin reconciliation diff computed")

	// Phase 4: Apply changes

	for _, id := range toUnload {
		log.Info().Uint("plugin_id", id).Msg("Unloading removed plugin")
		if err := pm.UnloadPlugin(id); err != nil {
			log.Error().Uint("plugin_id", id).Err(err).Msg("Failed to unload removed plugin")
		}
	}

	for _, id := range toReload {
		log.Info().Uint("plugin_id", id).Msg("Reloading changed plugin")
		if err := pm.ReloadPlugin(id); err != nil {
			log.Error().Uint("plugin_id", id).Err(err).Msg("Failed to reload changed plugin")
		}
	}

	// Load new plugins (PrewarmAllPlugins skips already-loaded)
	if err := pm.PrewarmAllPlugins(ctx); err != nil {
		log.Warn().Err(err).Msg("Some new plugins failed to load during reconciliation")
	}

	// Refresh endpoint routing table
	pm.RefreshAllEndpoints()

	log.Info().Msg("Plugin reconciliation completed")
	return nil
}

// GetEndpointRoute looks up a route for the given HTTP method, plugin name, and sub-path.
// Returns nil when no matching route is found.
func (pm *PluginManager) GetEndpointRoute(method, pluginName, subPath string) *EndpointRoute {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Try exact match first
	key := endpointRouteKey(method, pluginName, subPath)
	if route, ok := pm.endpointRoutes[key]; ok {
		return route
	}

	// Try wildcard match: the plugin registered "/*"
	wildcardKey := endpointRouteKey(method, pluginName, "/*")
	if route, ok := pm.endpointRoutes[wildcardKey]; ok {
		return route
	}

	return nil
}

// HandleEndpointRequest dispatches a unary endpoint request to the appropriate plugin.
func (pm *PluginManager) HandleEndpointRequest(ctx context.Context, pluginID uint, req *pb.EndpointRequest) (*pb.EndpointResponse, error) {
	pm.mu.RLock()
	lp, exists := pm.loadedPlugins[pluginID]
	pm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	if lp.GRPCClient == nil {
		return nil, fmt.Errorf("plugin %d has no gRPC client", pluginID)
	}

	return lp.GRPCClient.HandleEndpointRequest(ctx, req)
}

// HandleEndpointRequestStream dispatches a streaming endpoint request to the appropriate plugin.
func (pm *PluginManager) HandleEndpointRequestStream(ctx context.Context, pluginID uint, req *pb.EndpointRequest) (interface{ Recv() (*pb.EndpointResponseChunk, error) }, error) {
	pm.mu.RLock()
	lp, exists := pm.loadedPlugins[pluginID]
	pm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin %d is not loaded", pluginID)
	}

	if lp.GRPCClient == nil {
		return nil, fmt.Errorf("plugin %d has no gRPC client", pluginID)
	}

	return lp.GRPCClient.HandleEndpointRequestStream(ctx, req)
}

// --- Endpoint helper functions ---

// endpointRouteKey builds the map key used for endpoint route lookups.
func endpointRouteKey(method, pluginName, path string) string {
	return strings.ToUpper(method) + ":" + pluginName + path
}

// ParsePluginEndpointPath splits a request path of the form /plugins/{pluginName}/...
// into the plugin name (slug) and the remaining sub-path.
// Returns ("", "", false) when the path does not match the expected prefix.
func ParsePluginEndpointPath(fullPath string) (pluginName string, subPath string, ok bool) {
	trimmed := strings.TrimPrefix(fullPath, "/plugins/")
	if trimmed == fullPath {
		return "", "", false
	}

	slashIdx := strings.Index(trimmed, "/")
	if slashIdx < 0 {
		if trimmed == "" {
			return "", "", false
		}
		return trimmed, "/", true
	}

	pluginName = trimmed[:slashIdx]
	subPath = trimmed[slashIdx:]

	if pluginName == "" {
		return "", "", false
	}

	return pluginName, subPath, true
}

// isValidEndpointPath checks that a registration path is safe and well-formed.
func isValidEndpointPath(path string) bool {
	if path == "" {
		return false
	}
	// Must start with /
	if path[0] != '/' {
		return false
	}
	// Reject directory traversal
	if strings.Contains(path, "..") {
		return false
	}
	return true
}

// validateHTTPMethods filters a list of methods to only those that are valid HTTP methods.
func validateHTTPMethods(methods []string) []string {
	var valid []string
	for _, m := range methods {
		upper := strings.ToUpper(m)
		if validHTTPMethods[upper] {
			valid = append(valid, upper)
		}
	}
	return valid
}

// SplitPathSegments splits a path like "/users/123/profile" into ["users", "123", "profile"].
func SplitPathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
