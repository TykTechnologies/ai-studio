package ai_studio_sdk

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"google.golang.org/grpc"
)

// AIStudioPluginImplementation defines the interface that plugin developers must implement
// The SDK handles all the gRPC plumbing and provides the ServiceAPI client automatically
type AIStudioPluginImplementation interface {
	// OnInitialize is called when the plugin is initialized
	// Parameters:
	// - serviceAPI: Client for calling AI Studio services (LLMs, tools, etc.)
	// - pluginID: This plugin's ID in the database
	// - config: Plugin configuration as map[string]string (from database)
	// Plugin developers can parse config and store the reference to start using service APIs
	OnInitialize(serviceAPI mgmtpb.AIStudioManagementServiceClient, pluginID uint32, config map[string]string) error

	// OnShutdown is called when the plugin is being shut down
	// Plugin should clean up any resources and complete any in-flight operations
	OnShutdown() error

	// GetAsset serves static assets for the plugin UI
	// Returns (content, mimeType, error)
	// Used for serving JavaScript, CSS, images, etc.
	GetAsset(assetPath string) ([]byte, string, error)

	// GetManifest returns the plugin manifest as JSON bytes
	// The manifest declares UI components, permissions, and other metadata
	GetManifest() ([]byte, error)

	// HandleCall processes custom RPC method calls from the UI
	// method: The RPC method name (e.g., "get_statistics", "get_settings")
	// payload: JSON payload as bytes from the frontend
	// Returns: JSON response as bytes
	HandleCall(method string, payload []byte) ([]byte, error)

	// GetConfigSchema returns the JSON Schema for plugin configuration
	// Used by the admin UI to generate configuration forms
	// Returns: JSON Schema as bytes
	GetConfigSchema() ([]byte, error)
}

// AgentPluginImplementation defines the interface that agent plugin developers must implement
// Agent plugins process user messages and stream responses back
// The SDK handles service API injection and broker setup automatically
type AgentPluginImplementation interface {
	// OnInitialize is called when the agent plugin is initialized
	// Parameters:
	// - serviceAPI: Client for calling AI Studio services (e.g., CallLLM for proxying requests)
	// - pluginID: This plugin's ID in the database
	// - config: Plugin configuration as map[string]string (from database)
	// The agent can parse config for settings like default LLM, system prompts, etc.
	OnInitialize(serviceAPI mgmtpb.AIStudioManagementServiceClient, pluginID uint32, config map[string]string) error

	// OnShutdown is called when the agent plugin is being shut down
	// Agent should clean up any resources and complete any in-flight operations
	OnShutdown() error

	// HandleAgentMessage processes a user message and streams responses back
	// req contains: user message, available LLMs/tools/datasources, conversation history
	// stream is used to send back content chunks, tool calls, thinking, errors, etc.
	// The agent should call stream.Send() for each chunk and end with a DONE chunk
	HandleAgentMessage(req *pb.AgentMessageRequest, stream grpc.ServerStreamingServer[pb.AgentMessageChunk]) error

	// GetManifest returns the plugin manifest as JSON bytes
	// The manifest declares permissions, scopes, and metadata
	GetManifest() ([]byte, error)

	// GetConfigSchema returns the JSON Schema for plugin configuration
	// Used by the admin UI to generate agent configuration forms
	// Returns: JSON Schema as bytes
	GetConfigSchema() ([]byte, error)
}

// ServiceAPIScope defines the available service API scope constants
// These match the scopes declared in plugin manifests
type ServiceAPIScope struct {
	// Plugin management scopes
	PluginsRead   string
	PluginsWrite  string
	PluginsConfig string

	// LLM management scopes
	LLMsRead  string
	LLMsWrite string
	LLMsProxy string // For agent plugins to call LLM proxy

	// Tool management scopes
	ToolsRead       string
	ToolsWrite      string
	ToolsOperations string

	// App management scopes
	AppsRead  string
	AppsWrite string

	// Plugin KV storage scopes
	KVReadWrite string

	// Datasource management scopes
	DatasourcesRead       string
	DatasourcesWrite      string
	DatasourcesQuery      string // Query datasources (for agent plugins)
	DatasourcesEmbeddings string // Generate embeddings and store documents

	// Tag management scopes
	TagsRead  string
	TagsWrite string

	// Filter management scopes
	FiltersRead  string
	FiltersWrite string

	// Pricing scopes
	PricingRead  string
	PricingWrite string

	// Data catalogue scopes
	DataCataloguesRead  string
	DataCataloguesWrite string

	// Scheduler management scopes
	SchedulerManage string
}

// AvailableScopes provides constants for common service API scopes
var AvailableScopes = ServiceAPIScope{
	PluginsRead:         "plugins.read",
	PluginsWrite:        "plugins.write",
	PluginsConfig:       "plugins.config",
	LLMsRead:            "llms.read",
	LLMsWrite:           "llms.write",
	LLMsProxy:           "llms.proxy",
	ToolsRead:           "tools.read",
	ToolsWrite:          "tools.write",
	ToolsOperations:     "tools.operations",
	AppsRead:            "apps.read",
	AppsWrite:           "apps.write",
	KVReadWrite:           "kv.readwrite",
	DatasourcesRead:       "datasources.read",
	DatasourcesWrite:      "datasources.write",
	DatasourcesQuery:      "datasources.query",
	DatasourcesEmbeddings: "datasources.embeddings",
	TagsRead:              "tags.read",
	TagsWrite:           "tags.write",
	FiltersRead:         "filters.read",
	FiltersWrite:        "filters.write",
	PricingRead:         "pricing.read",
	PricingWrite:        "pricing.write",
	DataCataloguesRead:  "data-catalogues.read",
	DataCataloguesWrite: "data-catalogues.write",
	SchedulerManage:     "scheduler.manage",
}

// PluginContext provides context information for service API calls
type PluginContext struct {
	PluginID    uint32
	MethodScope string
}

// ServiceAPIHelper provides helper methods for common service API operations
type ServiceAPIHelper struct {
	serviceAPI mgmtpb.AIStudioManagementServiceClient
	pluginID   uint32
}

// NewServiceAPIHelper creates a helper for common service API operations
func NewServiceAPIHelper(serviceAPI mgmtpb.AIStudioManagementServiceClient, pluginID uint32) *ServiceAPIHelper {
	return &ServiceAPIHelper{
		serviceAPI: serviceAPI,
		pluginID:   pluginID,
	}
}

// GetPluginsCount returns the total number of plugins
func (h *ServiceAPIHelper) GetPluginsCount(ctx context.Context) (int, error) {
	resp, err := h.serviceAPI.ListPlugins(ctx, &mgmtpb.ListPluginsRequest{
		Context: &mgmtpb.PluginContext{
			PluginId:    h.pluginID,
			MethodScope: AvailableScopes.PluginsRead,
		},
		Page:  1,
		Limit: 1,
	})
	if err != nil {
		return 0, err
	}
	return int(resp.TotalCount), nil
}

// GetLLMsCount returns the total number of LLMs
func (h *ServiceAPIHelper) GetLLMsCount(ctx context.Context) (int, error) {
	resp, err := h.serviceAPI.ListLLMs(ctx, &mgmtpb.ListLLMsRequest{
		Context: &mgmtpb.PluginContext{
			PluginId:    h.pluginID,
			MethodScope: AvailableScopes.LLMsRead,
		},
		Page:  1,
		Limit: 1,
	})
	if err != nil {
		return 0, err
	}
	return int(resp.TotalCount), nil
}