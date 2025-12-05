package ai_studio_sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Global SDK state for service API access
var (
	serviceClient   mgmtpb.AIStudioManagementServiceClient
	pluginID        uint32
	serviceBrokerID uint32
	initialized     bool
	initMutex       sync.Mutex
	grpcBroker      *goplugin.GRPCBroker
	brokerConn      *grpc.ClientConn // Shared connection for all services on this broker
)

// Initialize sets up the SDK with broker access
// This is called from the plugin's GRPCServer method
func Initialize(server *grpc.Server, broker *goplugin.GRPCBroker, pluginIDVal uint32) error {
	initMutex.Lock()
	defer initMutex.Unlock()

	if initialized {
		log.Debug().Msg("SDK already initialized")
		return nil
	}

	// Store broker for service API access
	grpcBroker = broker
	pluginID = pluginIDVal // May be 0 initially, updated later
	initialized = true

	log.Info().Uint32("plugin_id", pluginID).Msg("✅ AI Studio SDK initialized with broker access")
	return nil
}

// SetServiceBrokerID stores the broker ID for dialing back to host services
// This is called when the plugin receives the broker ID from request payload
func SetServiceBrokerID(brokerID uint32) {
	initMutex.Lock()
	defer initMutex.Unlock()

	serviceBrokerID = brokerID
	log.Info().
		Uint32("broker_id", brokerID).
		Bool("sdk_initialized", initialized).
		Bool("has_grpc_broker", grpcBroker != nil).
		Str("broker_ptr", fmt.Sprintf("%p", grpcBroker)).
		Msg("✅ AI Studio SDK: Service broker ID set")
}

// ExtractBrokerIDFromPayload extracts the broker ID from RPC request payload
// This should be called by plugins that need service API access
func ExtractBrokerIDFromPayload(payload []byte) uint32 {
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		return 0
	}

	if brokerID, ok := payloadMap["_service_broker_id"].(float64); ok {
		return uint32(brokerID)
	}

	return 0
}

// SetPluginID updates the plugin ID after it's received from config
// This is called from the plugin's Initialize method after config is parsed
func SetPluginID(id uint32) {
	initMutex.Lock()
	defer initMutex.Unlock()

	pluginID = id
	log.Info().Uint32("plugin_id", pluginID).Msg("✅ Plugin ID updated in SDK")
}

// GetPluginID returns the current plugin ID
// This can be used by plugins to get their own ID for API calls
func GetPluginID() uint32 {
	initMutex.Lock()
	defer initMutex.Unlock()
	return pluginID
}

// getServiceClient creates and returns the service client, creating it if necessary
// Includes retry logic to handle race conditions where the broker server may not be ready yet
func getServiceClient(ctx context.Context) (mgmtpb.AIStudioManagementServiceClient, error) {
	if serviceClient != nil {
		return serviceClient, nil
	}

	log.Debug().
		Bool("initialized", initialized).
		Bool("has_broker", grpcBroker != nil).
		Uint32("broker_id", serviceBrokerID).
		Str("broker_ptr", fmt.Sprintf("%p", grpcBroker)).
		Msg("getServiceClient: checking prerequisites (AI STUDIO SDK dial attempt)")

	if !initialized || grpcBroker == nil {
		return nil, fmt.Errorf("SDK not initialized - call ai_studio_sdk.Initialize() first (initialized=%v, broker=%v)", initialized, grpcBroker != nil)
	}

	if serviceBrokerID == 0 {
		return nil, fmt.Errorf("service broker ID not set - call ai_studio_sdk.SetServiceBrokerID() with broker ID from config")
	}

	// Dial the brokered server where AI Studio management services are registered
	// Retry with backoff to handle race conditions where server may not be ready yet
	var conn *grpc.ClientConn
	var err error
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		conn, err = grpcBroker.Dial(serviceBrokerID)
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			backoff := time.Duration(50*(i+1)) * time.Millisecond
			log.Debug().
				Int("attempt", i+1).
				Dur("backoff", backoff).
				Err(err).
				Msg("Broker dial failed, retrying...")
			time.Sleep(backoff)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to dial service broker ID %d after %d attempts: %w", serviceBrokerID, maxRetries, err)
	}

	// Store connection for sharing with other services (e.g., EventService)
	brokerConn = conn

	// Create service client from the brokered connection
	serviceClient = mgmtpb.NewAIStudioManagementServiceClient(conn)

	log.Info().
		Uint32("plugin_id", pluginID).
		Uint32("broker_id", serviceBrokerID).
		Msg("✅ Service client created via broker dial - plugin can now call host services")

	return serviceClient, nil
}

// GetSharedBrokerConnection returns the shared gRPC connection to the host's brokered server.
// This allows other services (like EventService) to create clients on the same connection
// without dialing again. Returns nil if no connection has been established yet.
func GetSharedBrokerConnection() *grpc.ClientConn {
	initMutex.Lock()
	defer initMutex.Unlock()
	return brokerConn
}

// SetSharedBrokerConnection stores a pre-dialed gRPC connection for shared use.
// This is called by the event service when it dials first, so ai_studio_sdk can reuse
// the same connection instead of trying to dial again (which would fail since
// go-plugin broker only accepts ONE connection per broker ID).
func SetSharedBrokerConnection(conn *grpc.ClientConn) {
	initMutex.Lock()
	defer initMutex.Unlock()
	if brokerConn == nil && conn != nil {
		brokerConn = conn
		// Also create the service client immediately since we have the connection
		serviceClient = mgmtpb.NewAIStudioManagementServiceClient(conn)
		log.Info().Msg("✅ AI Studio SDK: Shared broker connection set from external source")
	}
}

// createPluginContext creates the authentication context for service API calls
func createPluginContext(methodScope string) *mgmtpb.PluginContext {
	return &mgmtpb.PluginContext{
		PluginId:    pluginID,
		MethodScope: methodScope,
	}
}

// ListPlugins returns a list of plugins from the AI Studio host
func ListPlugins(ctx context.Context, page, limit int32) (*mgmtpb.ListPluginsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListPlugins(ctx, &mgmtpb.ListPluginsRequest{
		Context: createPluginContext(AvailableScopes.PluginsRead),
		Page:    page,
		Limit:   limit,
	})
}

// GetPlugin returns details for a specific plugin
func GetPlugin(ctx context.Context, pluginID uint32) (*mgmtpb.GetPluginResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetPlugin(ctx, &mgmtpb.GetPluginRequest{
		Context:  createPluginContext(AvailableScopes.PluginsRead),
		PluginId: pluginID,
	})
}

// UpdatePluginConfig updates the configuration of a specific plugin
// The configJSON parameter should be a valid JSON string containing the full configuration
func UpdatePluginConfig(ctx context.Context, pluginID uint32, configJSON string) (*mgmtpb.UpdatePluginConfigResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdatePluginConfig(ctx, &mgmtpb.UpdatePluginConfigRequest{
		Context:    createPluginContext(AvailableScopes.PluginsWrite),
		PluginId:   pluginID,
		ConfigJson: configJSON,
	})
}

// ListLLMs returns a list of LLMs from the AI Studio host
func ListLLMs(ctx context.Context, page, limit int32) (*mgmtpb.ListLLMsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListLLMs(ctx, &mgmtpb.ListLLMsRequest{
		Context: createPluginContext(AvailableScopes.LLMsRead),
		Page:    page,
		Limit:   limit,
	})
}

// GetLLM returns details for a specific LLM
func GetLLM(ctx context.Context, llmID uint32) (*mgmtpb.GetLLMResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetLLM(ctx, &mgmtpb.GetLLMRequest{
		Context: createPluginContext(AvailableScopes.LLMsRead),
		LlmId:   llmID,
	})
}

// GetAnalyticsSummary returns analytics data from the AI Studio host
func GetAnalyticsSummary(ctx context.Context, timeRange string) (*mgmtpb.GetAnalyticsSummaryResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetAnalyticsSummary(ctx, &mgmtpb.GetAnalyticsSummaryRequest{
		Context:   createPluginContext("analytics.read"),
		TimeRange: timeRange,
	})
}

// ListTools returns a list of tools from the AI Studio host
func ListTools(ctx context.Context, page, limit int32) (*mgmtpb.ListToolsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListTools(ctx, &mgmtpb.ListToolsRequest{
		Context: createPluginContext(AvailableScopes.ToolsRead),
		Page:    page,
		Limit:   limit,
	})
}

// ListApps returns a list of applications from the AI Studio host
func ListApps(ctx context.Context, page, limit int32) (*mgmtpb.ListAppsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListApps(ctx, &mgmtpb.ListAppsRequest{
		Context: createPluginContext(AvailableScopes.AppsRead),
		Page:    page,
		Limit:   limit,
	})
}

// GetPluginsCount returns the total number of plugins (helper function)
func GetPluginsCount(ctx context.Context) (int, error) {
	resp, err := ListPlugins(ctx, 1, 1)
	if err != nil {
		return 0, err
	}
	return int(resp.TotalCount), nil
}

// GetLLMsCount returns the total number of LLMs (helper function)
func GetLLMsCount(ctx context.Context) (int, error) {
	resp, err := ListLLMs(ctx, 1, 1)
	if err != nil {
		return 0, err
	}
	return int(resp.TotalCount), nil
}

// GetToolsCount returns the total number of tools (helper function)
func GetToolsCount(ctx context.Context) (int, error) {
	resp, err := ListTools(ctx, 1, 1)
	if err != nil {
		return 0, err
	}
	return int(resp.TotalCount), nil
}

// IsInitialized returns whether the SDK has been initialized AND has a broker ID set.
// Both conditions must be true for service API calls to work.
func IsInitialized() bool {
	initMutex.Lock()
	defer initMutex.Unlock()
	return initialized && serviceBrokerID != 0
}

// Reset clears the SDK state (for testing)
func Reset() {
	initMutex.Lock()
	defer initMutex.Unlock()

	serviceClient = nil
	pluginID = 0
	initialized = false
}

// === Plugin KV Storage Functions ===

// WritePluginKV writes a key-value entry for the calling plugin
// Returns true if a new entry was created, false if an existing entry was updated
// expireAt is optional - pass nil for no expiration
func WritePluginKV(ctx context.Context, key string, value []byte, expireAt *time.Time) (bool, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return false, fmt.Errorf("service client unavailable: %w", err)
	}

	req := &mgmtpb.WritePluginKVRequest{
		Context: createPluginContext(AvailableScopes.KVReadWrite),
		Key:     key,
		Value:   value,
	}

	// Set expiration if provided
	if expireAt != nil {
		req.ExpireAt = timestamppb.New(*expireAt)
	}

	resp, err := client.WritePluginKV(ctx, req)
	if err != nil {
		return false, fmt.Errorf("failed to write KV data: %w", err)
	}

	return resp.Created, nil
}

// WritePluginKVWithTTL is a convenience function that writes a key-value entry with a TTL (time-to-live)
// The expiration time is calculated as time.Now().Add(ttl)
func WritePluginKVWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	expireAt := time.Now().Add(ttl)
	return WritePluginKV(ctx, key, value, &expireAt)
}

// ReadPluginKV reads a key-value entry for the calling plugin
// Returns an error if the key doesn't exist
func ReadPluginKV(ctx context.Context, key string) ([]byte, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.ReadPluginKV(ctx, &mgmtpb.ReadPluginKVRequest{
		Context: createPluginContext(AvailableScopes.KVReadWrite),
		Key:     key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read KV data: %w", err)
	}

	return resp.Value, nil
}

// DeletePluginKV deletes a key-value entry for the calling plugin
// Returns true if the key existed and was deleted, false if the key didn't exist
func DeletePluginKV(ctx context.Context, key string) (bool, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return false, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeletePluginKV(ctx, &mgmtpb.DeletePluginKVRequest{
		Context: createPluginContext(AvailableScopes.KVReadWrite),
		Key:     key,
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete KV data: %w", err)
	}

	return resp.Deleted, nil
}

// === LLM CRUD Operations ===

// CreateLLM creates a new LLM configuration
func CreateLLM(ctx context.Context, name, apiKey, apiEndpoint, vendor, defaultModel string, privacyScore int32, allowedModels []string, monthlyBudget *float64) (*mgmtpb.CreateLLMResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateLLM(ctx, &mgmtpb.CreateLLMRequest{
		Context:         createPluginContext(AvailableScopes.LLMsWrite),
		Name:            name,
		ApiKey:          apiKey,
		ApiEndpoint:     apiEndpoint,
		Vendor:          vendor,
		PrivacyScore:    privacyScore,
		DefaultModel:    defaultModel,
		AllowedModels:   allowedModels,
		MonthlyBudget:   monthlyBudget,
		Active:          true,
	})
}

// UpdateLLM updates an existing LLM configuration
func UpdateLLM(ctx context.Context, llmID uint32, name, apiKey, apiEndpoint, defaultModel string, privacyScore int32, allowedModels []string, active bool, monthlyBudget *float64) (*mgmtpb.UpdateLLMResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateLLM(ctx, &mgmtpb.UpdateLLMRequest{
		Context:       createPluginContext(AvailableScopes.LLMsWrite),
		LlmId:         llmID,
		Name:          name,
		ApiKey:        apiKey,
		ApiEndpoint:   apiEndpoint,
		PrivacyScore:  privacyScore,
		DefaultModel:  defaultModel,
		AllowedModels: allowedModels,
		Active:        active,
		MonthlyBudget: monthlyBudget,
	})
}

// DeleteLLM deletes an LLM configuration
func DeleteLLM(ctx context.Context, llmID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteLLM(ctx, &mgmtpb.DeleteLLMRequest{
		Context: createPluginContext(AvailableScopes.LLMsWrite),
		LlmId:   llmID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete LLM: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete LLM failed: %s", resp.Message)
	}

	return nil
}

// === App CRUD Operations ===

// GetApp retrieves a specific app
func GetApp(ctx context.Context, appID uint32) (*mgmtpb.GetAppResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetApp(ctx, &mgmtpb.GetAppRequest{
		Context: createPluginContext(AvailableScopes.AppsRead),
		AppId:   appID,
	})
}

// CreateApp creates a new application
func CreateApp(ctx context.Context, name, description string, userID uint32, llmIDs, toolIDs []uint32, monthlyBudget *float64) (*mgmtpb.CreateAppResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateApp(ctx, &mgmtpb.CreateAppRequest{
		Context:       createPluginContext(AvailableScopes.AppsWrite),
		Name:          name,
		Description:   description,
		UserId:        userID,
		LlmIds:        llmIDs,
		ToolIds:       toolIDs,
		MonthlyBudget: monthlyBudget,
	})
}

// UpdateApp updates an existing application
func UpdateApp(ctx context.Context, appID uint32, name, description string, isActive bool, llmIDs, toolIDs []uint32, monthlyBudget *float64) (*mgmtpb.UpdateAppResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateApp(ctx, &mgmtpb.UpdateAppRequest{
		Context:       createPluginContext(AvailableScopes.AppsWrite),
		AppId:         appID,
		Name:          name,
		Description:   description,
		IsActive:      isActive,
		LlmIds:        llmIDs,
		ToolIds:       toolIDs,
		MonthlyBudget: monthlyBudget,
	})
}

// UpdateAppWithMetadata updates an application including metadata
func UpdateAppWithMetadata(ctx context.Context, appID uint32, name, description string, isActive bool, llmIDs, toolIDs, datasourceIDs []uint32, monthlyBudget *float64, metadata string) (*mgmtpb.UpdateAppResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateApp(ctx, &mgmtpb.UpdateAppRequest{
		Context:       createPluginContext(AvailableScopes.AppsWrite),
		AppId:         appID,
		Name:          name,
		Description:   description,
		IsActive:      isActive,
		LlmIds:        llmIDs,
		ToolIds:       toolIDs,
		DatasourceIds: datasourceIDs,
		MonthlyBudget: monthlyBudget,
		Metadata:      metadata,
	})
}

// DeleteApp deletes an application
func DeleteApp(ctx context.Context, appID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteApp(ctx, &mgmtpb.DeleteAppRequest{
		Context: createPluginContext(AvailableScopes.AppsWrite),
		AppId:   appID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete app failed: %s", resp.Message)
	}

	return nil
}

// === Tool CRUD Operations ===

// GetTool retrieves a specific tool
func GetTool(ctx context.Context, toolID uint32) (*mgmtpb.GetToolResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetTool(ctx, &mgmtpb.GetToolRequest{
		Context: createPluginContext(AvailableScopes.ToolsRead),
		ToolId:  toolID,
	})
}

// CreateTool creates a new tool
func CreateTool(ctx context.Context, name, description, toolType, oasSpec, authSchemaName, authKey string, privacyScore int32) (*mgmtpb.CreateToolResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateTool(ctx, &mgmtpb.CreateToolRequest{
		Context:        createPluginContext(AvailableScopes.ToolsWrite),
		Name:           name,
		Description:    description,
		ToolType:       toolType,
		OasSpec:        oasSpec,
		PrivacyScore:   privacyScore,
		AuthSchemaName: authSchemaName,
		AuthKey:        authKey,
	})
}

// UpdateTool updates an existing tool
func UpdateTool(ctx context.Context, toolID uint32, name, description, toolType, oasSpec, authSchemaName, authKey string, privacyScore int32) (*mgmtpb.UpdateToolResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateTool(ctx, &mgmtpb.UpdateToolRequest{
		Context:        createPluginContext(AvailableScopes.ToolsWrite),
		ToolId:         toolID,
		Name:           name,
		Description:    description,
		ToolType:       toolType,
		OasSpec:        oasSpec,
		PrivacyScore:   privacyScore,
		AuthSchemaName: authSchemaName,
		AuthKey:        authKey,
	})
}

// DeleteTool deletes a tool
func DeleteTool(ctx context.Context, toolID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteTool(ctx, &mgmtpb.DeleteToolRequest{
		Context: createPluginContext(AvailableScopes.ToolsWrite),
		ToolId:  toolID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete tool: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete tool failed: %s", resp.Message)
	}

	return nil
}

// === Tag CRUD Operations ===

// GetTag retrieves a specific tag
func GetTag(ctx context.Context, tagID uint32) (*mgmtpb.GetTagResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetTag(ctx, &mgmtpb.GetTagRequest{
		Context: createPluginContext(AvailableScopes.TagsRead),
		TagId:   tagID,
	})
}

// CreateTag creates a new tag
func CreateTag(ctx context.Context, name string) (*mgmtpb.CreateTagResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateTag(ctx, &mgmtpb.CreateTagRequest{
		Context: createPluginContext(AvailableScopes.TagsWrite),
		Name:    name,
	})
}

// UpdateTag updates an existing tag
func UpdateTag(ctx context.Context, tagID uint32, name string) (*mgmtpb.UpdateTagResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateTag(ctx, &mgmtpb.UpdateTagRequest{
		Context: createPluginContext(AvailableScopes.TagsWrite),
		TagId:   tagID,
		Name:    name,
	})
}

// DeleteTag deletes a tag
func DeleteTag(ctx context.Context, tagID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteTag(ctx, &mgmtpb.DeleteTagRequest{
		Context: createPluginContext(AvailableScopes.TagsWrite),
		TagId:   tagID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete tag failed: %s", resp.Message)
	}

	return nil
}

// SearchTags searches for tags by query
func SearchTags(ctx context.Context, query string) (*mgmtpb.SearchTagsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.SearchTags(ctx, &mgmtpb.SearchTagsRequest{
		Context: createPluginContext(AvailableScopes.TagsRead),
		Query:   query,
	})
}

// === Filter CRUD Operations ===

// GetFilter retrieves a specific filter
func GetFilter(ctx context.Context, filterID uint32) (*mgmtpb.GetFilterResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetFilter(ctx, &mgmtpb.GetFilterRequest{
		Context:  createPluginContext(AvailableScopes.FiltersRead),
		FilterId: filterID,
	})
}

// CreateFilter creates a new filter
func CreateFilter(ctx context.Context, name, description, script string) (*mgmtpb.CreateFilterResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateFilter(ctx, &mgmtpb.CreateFilterRequest{
		Context:     createPluginContext(AvailableScopes.FiltersWrite),
		Name:        name,
		Description: description,
		Script:      script,
	})
}

// UpdateFilter updates an existing filter
func UpdateFilter(ctx context.Context, filterID uint32, name, description, script string) (*mgmtpb.UpdateFilterResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateFilter(ctx, &mgmtpb.UpdateFilterRequest{
		Context:     createPluginContext(AvailableScopes.FiltersWrite),
		FilterId:    filterID,
		Name:        name,
		Description: description,
		Script:      script,
	})
}

// DeleteFilter deletes a filter
func DeleteFilter(ctx context.Context, filterID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteFilter(ctx, &mgmtpb.DeleteFilterRequest{
		Context:  createPluginContext(AvailableScopes.FiltersWrite),
		FilterId: filterID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete filter: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete filter failed: %s", resp.Message)
	}

	return nil
}

// === Model Price CRUD Operations ===

// GetModelPrice retrieves a specific model price
func GetModelPrice(ctx context.Context, modelPriceID uint32) (*mgmtpb.GetModelPriceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetModelPrice(ctx, &mgmtpb.GetModelPriceRequest{
		Context:      createPluginContext(AvailableScopes.PricingRead),
		ModelPriceId: modelPriceID,
	})
}

// CreateModelPrice creates a new model price entry
func CreateModelPrice(ctx context.Context, modelName, vendor, currency string, cpt, cpit, cacheWritePt, cacheReadPt float64) (*mgmtpb.CreateModelPriceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateModelPrice(ctx, &mgmtpb.CreateModelPriceRequest{
		Context:      createPluginContext(AvailableScopes.PricingWrite),
		ModelName:    modelName,
		Vendor:       vendor,
		Cpt:          cpt,
		Cpit:         cpit,
		CacheWritePt: cacheWritePt,
		CacheReadPt:  cacheReadPt,
		Currency:     currency,
	})
}

// UpdateModelPrice updates an existing model price
func UpdateModelPrice(ctx context.Context, modelPriceID uint32, modelName, vendor, currency string, cpt, cpit, cacheWritePt, cacheReadPt float64) (*mgmtpb.UpdateModelPriceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateModelPrice(ctx, &mgmtpb.UpdateModelPriceRequest{
		Context:      createPluginContext(AvailableScopes.PricingWrite),
		ModelPriceId: modelPriceID,
		ModelName:    modelName,
		Vendor:       vendor,
		Cpt:          cpt,
		Cpit:         cpit,
		CacheWritePt: cacheWritePt,
		CacheReadPt:  cacheReadPt,
		Currency:     currency,
	})
}

// DeleteModelPrice deletes a model price
func DeleteModelPrice(ctx context.Context, modelPriceID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteModelPrice(ctx, &mgmtpb.DeleteModelPriceRequest{
		Context:      createPluginContext(AvailableScopes.PricingWrite),
		ModelPriceId: modelPriceID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete model price: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete model price failed: %s", resp.Message)
	}

	return nil
}

// === Datasource CRUD Operations ===

// ListDatasources retrieves all datasources with optional filtering and pagination
func ListDatasources(ctx context.Context, page, limit int32, isActive *bool, userID string) (*mgmtpb.ListDatasourcesResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListDatasources(ctx, &mgmtpb.ListDatasourcesRequest{
		Context:  createPluginContext(AvailableScopes.DatasourcesRead),
		Page:     page,
		Limit:    limit,
		IsActive: isActive,
		UserId:   userID,
	})
}

// GetDatasource retrieves a specific datasource
func GetDatasource(ctx context.Context, datasourceID uint32) (*mgmtpb.GetDatasourceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetDatasource(ctx, &mgmtpb.GetDatasourceRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesRead),
		DatasourceId: datasourceID,
	})
}

// CreateDatasource creates a new datasource with full configuration
func CreateDatasource(ctx context.Context, name, shortDesc, longDesc, url, dbSourceType string, privacyScore int32, userID uint32, active bool) (*mgmtpb.CreateDatasourceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateDatasource(ctx, &mgmtpb.CreateDatasourceRequest{
		Context:          createPluginContext(AvailableScopes.DatasourcesWrite),
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Url:              url,
		DbSourceType:     dbSourceType,
		PrivacyScore:     privacyScore,
		UserId:           userID,
		Active:           active,
	})
}

// CreateDatasourceWithEmbedder creates a new datasource with full embedder configuration
// This is the complete version that includes vector store and embedder setup for RAG
func CreateDatasourceWithEmbedder(ctx context.Context, name, shortDesc, longDesc, url string,
	dbConnString, dbSourceType, dbConnAPIKey, dbName string,
	embedVendor, embedURL, embedAPIKey, embedModel string,
	privacyScore int32, userID uint32, active bool) (*mgmtpb.CreateDatasourceResponse, error) {

	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateDatasource(ctx, &mgmtpb.CreateDatasourceRequest{
		Context:          createPluginContext(AvailableScopes.DatasourcesWrite),
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Url:              url,
		DbConnString:     dbConnString,
		DbSourceType:     dbSourceType,
		DbConnApiKey:     dbConnAPIKey,
		DbName:           dbName,
		EmbedVendor:      embedVendor,
		EmbedUrl:         embedURL,
		EmbedApiKey:      embedAPIKey,
		EmbedModel:       embedModel,
		PrivacyScore:     privacyScore,
		UserId:           userID,
		Active:           active,
	})
}

// UpdateDatasource updates an existing datasource
func UpdateDatasource(ctx context.Context, datasourceID uint32, name, shortDesc, longDesc, url, dbSourceType string, privacyScore int32, userID uint32, active bool) (*mgmtpb.UpdateDatasourceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateDatasource(ctx, &mgmtpb.UpdateDatasourceRequest{
		Context:          createPluginContext(AvailableScopes.DatasourcesWrite),
		DatasourceId:     datasourceID,
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Url:              url,
		DbSourceType:     dbSourceType,
		PrivacyScore:     privacyScore,
		UserId:           userID,
		Active:           active,
	})
}

// UpdateDatasourceWithEmbedder updates an existing datasource with full embedder configuration
func UpdateDatasourceWithEmbedder(ctx context.Context, datasourceID uint32,
	name, shortDesc, longDesc, url string,
	dbConnString, dbSourceType, dbConnAPIKey, dbName string,
	embedVendor, embedURL, embedAPIKey, embedModel string,
	privacyScore int32, userID uint32, active bool, tagNames []string) (*mgmtpb.UpdateDatasourceResponse, error) {

	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateDatasource(ctx, &mgmtpb.UpdateDatasourceRequest{
		Context:          createPluginContext(AvailableScopes.DatasourcesWrite),
		DatasourceId:     datasourceID,
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Url:              url,
		DbConnString:     dbConnString,
		DbSourceType:     dbSourceType,
		DbConnApiKey:     dbConnAPIKey,
		DbName:           dbName,
		EmbedVendor:      embedVendor,
		EmbedUrl:         embedURL,
		EmbedApiKey:      embedAPIKey,
		EmbedModel:       embedModel,
		PrivacyScore:     privacyScore,
		UserId:           userID,
		Active:           active,
		TagNames:         tagNames,
	})
}

// DeleteDatasource deletes a datasource
func DeleteDatasource(ctx context.Context, datasourceID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteDatasource(ctx, &mgmtpb.DeleteDatasourceRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesWrite),
		DatasourceId: datasourceID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete datasource: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete datasource failed: %s", resp.Message)
	}

	return nil
}

// CloneDatasource clones an existing datasource with all configuration including API keys
func CloneDatasource(ctx context.Context, sourceDatasourceID uint32) (*mgmtpb.CloneDatasourceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CloneDatasource(ctx, &mgmtpb.CloneDatasourceRequest{
		Context:            createPluginContext(AvailableScopes.DatasourcesWrite),
		SourceDatasourceId: sourceDatasourceID,
	})
}

// SearchDatasources searches datasources
func SearchDatasources(ctx context.Context, query string) (*mgmtpb.SearchDatasourcesResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.SearchDatasources(ctx, &mgmtpb.SearchDatasourcesRequest{
		Context: createPluginContext(AvailableScopes.DatasourcesRead),
		Query:   query,
	})
}

// QueryDatasource performs a semantic search on a datasource using a text query
// The query text is automatically converted to an embedding and used to search the vector store
func QueryDatasource(ctx context.Context, datasourceID uint32, query string, maxResults int32, similarityThreshold float64) (*mgmtpb.QueryDatasourceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.QueryDatasource(ctx, &mgmtpb.QueryDatasourceRequest{
		Context:             createPluginContext(AvailableScopes.DatasourcesQuery),
		DatasourceId:        datasourceID,
		Query:               query,
		MaxResults:          maxResults,
		SimilarityThreshold: similarityThreshold,
	})
}

// ProcessDatasourceEmbeddings triggers async processing of all files in a datasource
// This generates embeddings for file content and stores them in the configured vector store
func ProcessDatasourceEmbeddings(ctx context.Context, datasourceID uint32) (*mgmtpb.ProcessEmbeddingsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ProcessDatasourceEmbeddings(ctx, &mgmtpb.ProcessEmbeddingsRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesEmbeddings),
		DatasourceId: datasourceID,
	})
}

// GenerateEmbedding generates embeddings for text using the datasource's embedder configuration
func GenerateEmbedding(ctx context.Context, datasourceID uint32, texts []string) (*mgmtpb.GenerateEmbeddingResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GenerateEmbedding(ctx, &mgmtpb.GenerateEmbeddingRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesEmbeddings),
		DatasourceId: datasourceID,
		Texts:        texts,
	})
}

// StoreDocuments stores pre-vectorized documents in the datasource's vector store
func StoreDocuments(ctx context.Context, datasourceID uint32, documents []*mgmtpb.DocumentWithEmbedding) (*mgmtpb.StoreDocumentsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.StoreDocuments(ctx, &mgmtpb.StoreDocumentsRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesEmbeddings),
		DatasourceId: datasourceID,
		Documents:    documents,
	})
}

// ProcessAndStoreDocuments generates embeddings and stores documents in one step
func ProcessAndStoreDocuments(ctx context.Context, datasourceID uint32, chunks []*mgmtpb.DocumentChunk) (*mgmtpb.ProcessAndStoreResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ProcessAndStoreDocuments(ctx, &mgmtpb.ProcessAndStoreRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesEmbeddings),
		DatasourceId: datasourceID,
		Chunks:       chunks,
	})
}

// QueryDatasourceByVector performs similarity search using a pre-computed embedding vector
func QueryDatasourceByVector(ctx context.Context, datasourceID uint32, embedding []float32, maxResults int32, similarityThreshold float64) (*mgmtpb.QueryDatasourceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.QueryDatasourceByVector(ctx, &mgmtpb.QueryByVectorRequest{
		Context:             createPluginContext(AvailableScopes.DatasourcesQuery),
		DatasourceId:        datasourceID,
		Embedding:           embedding,
		MaxResults:          maxResults,
		SimilarityThreshold: similarityThreshold,
	})
}

// === Advanced Datasource Operations - Metadata and Namespace Management ===

// DeleteDocumentsByMetadata deletes documents matching metadata filter
func DeleteDocumentsByMetadata(ctx context.Context, datasourceID uint32, metadataFilter map[string]string, filterMode string, dryRun bool) (int32, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return 0, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteDocumentsByMetadata(ctx, &mgmtpb.DeleteDocumentsByMetadataRequest{
		Context:        createPluginContext(AvailableScopes.DatasourcesWrite),
		DatasourceId:   datasourceID,
		MetadataFilter: metadataFilter,
		FilterMode:     filterMode,
		DryRun:         dryRun,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to delete documents: %w", err)
	}

	return resp.DeletedCount, nil
}

// QueryByMetadataOnly queries documents using only metadata filters
func QueryByMetadataOnly(ctx context.Context, datasourceID uint32, metadataFilter map[string]string, filterMode string, limit, offset int32) ([]*mgmtpb.DatasourceResult, int32, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.QueryByMetadataOnly(ctx, &mgmtpb.QueryByMetadataOnlyRequest{
		Context:        createPluginContext(AvailableScopes.DatasourcesQuery),
		DatasourceId:   datasourceID,
		MetadataFilter: metadataFilter,
		FilterMode:     filterMode,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query by metadata: %w", err)
	}

	return resp.Results, resp.TotalCount, nil
}

// ListNamespaces lists all namespaces/collections in the vector store
func ListNamespaces(ctx context.Context, datasourceID uint32) ([]*mgmtpb.NamespaceInfo, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.ListNamespaces(ctx, &mgmtpb.ListNamespacesRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesRead),
		DatasourceId: datasourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	return resp.Namespaces, nil
}

// DeleteNamespace deletes an entire namespace/collection
func DeleteNamespace(ctx context.Context, datasourceID uint32, namespace string, confirm bool) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	_, err = client.DeleteNamespace(ctx, &mgmtpb.DeleteNamespaceRequest{
		Context:      createPluginContext(AvailableScopes.DatasourcesWrite),
		DatasourceId: datasourceID,
		Namespace:    namespace,
		Confirm:      confirm,
	})
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	return nil
}

// === Data Catalogue CRUD Operations ===

// GetDataCatalogue retrieves a specific data catalogue
func GetDataCatalogue(ctx context.Context, catalogueID uint32) (*mgmtpb.GetDataCatalogueResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetDataCatalogue(ctx, &mgmtpb.GetDataCatalogueRequest{
		Context:         createPluginContext(AvailableScopes.DataCataloguesRead),
		DataCatalogueId: catalogueID,
	})
}

// CreateDataCatalogue creates a new data catalogue
func CreateDataCatalogue(ctx context.Context, name, shortDesc, longDesc, icon string) (*mgmtpb.CreateDataCatalogueResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.CreateDataCatalogue(ctx, &mgmtpb.CreateDataCatalogueRequest{
		Context:          createPluginContext(AvailableScopes.DataCataloguesWrite),
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Icon:             icon,
	})
}

// UpdateDataCatalogue updates an existing data catalogue
func UpdateDataCatalogue(ctx context.Context, catalogueID uint32, name, shortDesc, longDesc, icon string) (*mgmtpb.UpdateDataCatalogueResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.UpdateDataCatalogue(ctx, &mgmtpb.UpdateDataCatalogueRequest{
		Context:          createPluginContext(AvailableScopes.DataCataloguesWrite),
		DataCatalogueId:  catalogueID,
		Name:             name,
		ShortDescription: shortDesc,
		LongDescription:  longDesc,
		Icon:             icon,
	})
}

// DeleteDataCatalogue deletes a data catalogue
func DeleteDataCatalogue(ctx context.Context, catalogueID uint32) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteDataCatalogue(ctx, &mgmtpb.DeleteDataCatalogueRequest{
		Context:         createPluginContext(AvailableScopes.DataCataloguesWrite),
		DataCatalogueId: catalogueID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete data catalogue: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete data catalogue failed: %s", resp.Message)
	}

	return nil
}

// === LLM Proxy Operations ===

// CallLLM is a helper function for agent plugins to call the LLM proxy
// This is the primary way agent plugins interact with LLMs
// Returns a streaming client for receiving responses
func CallLLM(ctx context.Context, llmID uint32, model string, messages []*mgmtpb.LLMMessage, temperature float64, maxTokens int32, tools []*mgmtpb.LLMTool, stream bool) (mgmtpb.AIStudioManagementService_CallLLMClient, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	req := &mgmtpb.CallLLMRequest{
		Context:     createPluginContext(AvailableScopes.LLMsProxy),
		LlmId:       llmID,
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Tools:       tools,
		Stream:      stream,
	}

	streamClient, err := client.CallLLM(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call LLM: %w", err)
	}

	return streamClient, nil
}

// CallLLMSimple is a simplified helper that waits for the complete non-streaming response
// Use this when you want a simple request-response pattern without streaming
func CallLLMSimple(ctx context.Context, llmID uint32, model string, messages []*mgmtpb.LLMMessage, temperature float64, maxTokens int32) (string, error) {
	stream, err := CallLLM(ctx, llmID, model, messages, temperature, maxTokens, nil, false)
	if err != nil {
		return "", err
	}

	// Read the single response (non-streaming mode)
	resp, err := stream.Recv()
	if err != nil {
		return "", fmt.Errorf("failed to receive LLM response: %w", err)
	}

	if !resp.Success {
		return "", fmt.Errorf("LLM call failed: %s", resp.ErrorMessage)
	}

	return resp.Content, nil
}

// ===========================
// Schedule Management Methods
// ===========================

// CreateSchedule creates a new schedule for the calling plugin
func CreateSchedule(ctx context.Context, scheduleID, name, cronExpr, timezone string, timeoutSeconds int32, config map[string]interface{}, enabled bool) (*mgmtpb.ScheduleInfo, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	// Convert config map to JSON string
	configJSON := "{}"
	if len(config) > 0 {
		if configBytes, err := json.Marshal(config); err == nil {
			configJSON = string(configBytes)
		} else {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
	}

	resp, err := client.CreateSchedule(ctx, &mgmtpb.CreateScheduleRequest{
		Context:        createPluginContext(AvailableScopes.SchedulerManage),
		ScheduleId:     scheduleID,
		Name:           name,
		CronExpr:       cronExpr,
		Timezone:       timezone,
		TimeoutSeconds: timeoutSeconds,
		ConfigJson:     configJSON,
		Enabled:        enabled,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create schedule: %w", err)
	}

	return resp.Schedule, nil
}

// GetSchedule retrieves a specific schedule by manifest_schedule_id
func GetSchedule(ctx context.Context, scheduleID string) (*mgmtpb.ScheduleInfo, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.GetSchedule(ctx, &mgmtpb.GetScheduleRequest{
		Context:    createPluginContext(AvailableScopes.SchedulerManage),
		ScheduleId: scheduleID,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	return resp.Schedule, nil
}

// ListSchedules lists all schedules for the calling plugin
func ListSchedules(ctx context.Context) ([]*mgmtpb.ScheduleInfo, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.ListSchedules(ctx, &mgmtpb.ListSchedulesRequest{
		Context: createPluginContext(AvailableScopes.SchedulerManage),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	return resp.Schedules, nil
}

// UpdateScheduleOptions provides optional fields for updating a schedule
type UpdateScheduleOptions struct {
	Name           *string
	CronExpr       *string
	Timezone       *string
	TimeoutSeconds *int32
	Config         map[string]interface{}
	Enabled        *bool
}

// UpdateSchedule updates an existing schedule with optional fields
func UpdateSchedule(ctx context.Context, scheduleID string, opts UpdateScheduleOptions) (*mgmtpb.ScheduleInfo, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	req := &mgmtpb.UpdateScheduleRequest{
		Context:    createPluginContext(AvailableScopes.SchedulerManage),
		ScheduleId: scheduleID,
	}

	// Set optional fields
	if opts.Name != nil {
		req.Name = opts.Name
	}
	if opts.CronExpr != nil {
		req.CronExpr = opts.CronExpr
	}
	if opts.Timezone != nil {
		req.Timezone = opts.Timezone
	}
	if opts.TimeoutSeconds != nil {
		req.TimeoutSeconds = opts.TimeoutSeconds
	}
	if opts.Enabled != nil {
		req.Enabled = opts.Enabled
	}
	if opts.Config != nil {
		if configBytes, err := json.Marshal(opts.Config); err == nil {
			configJSON := string(configBytes)
			req.ConfigJson = &configJSON
		} else {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
	}

	resp, err := client.UpdateSchedule(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update schedule: %w", err)
	}

	return resp.Schedule, nil
}

// DeleteSchedule deletes a schedule
func DeleteSchedule(ctx context.Context, scheduleID string) error {
	client, err := getServiceClient(ctx)
	if err != nil {
		return fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeleteSchedule(ctx, &mgmtpb.DeleteScheduleRequest{
		Context:    createPluginContext(AvailableScopes.SchedulerManage),
		ScheduleId: scheduleID,
	})

	if err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete schedule failed: %s", resp.Message)
	}

	return nil
}

// ===========================
// License Information Methods
// ===========================

// LicenseInfo represents license information returned from the host
type LicenseInfo struct {
	LicenseValid  bool      // True if a valid enterprise license is present
	DaysRemaining int       // Days until license expires (-1 for community/never expires)
	LicenseType   string    // "community" or "enterprise"
	Entitlements  []string  // List of enabled features/entitlements
	Organization  string    // Licensed organization name (enterprise only)
	ExpiresAt     time.Time // License expiration timestamp (zero for community)
}

// GetLicenseInfo retrieves license information from the AI Studio host
// This allows plugins to check if they're running in enterprise mode and what features are available
// Note: This doesn't require any special scope - all plugins can check license status
func GetLicenseInfo(ctx context.Context) (*LicenseInfo, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.GetLicenseInfo(ctx, &mgmtpb.GetLicenseInfoRequest{
		Context: createPluginContext(""), // No scope required for license check
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get license info: %w", err)
	}

	info := &LicenseInfo{
		LicenseValid:  resp.LicenseValid,
		DaysRemaining: int(resp.DaysRemaining),
		LicenseType:   resp.LicenseType,
		Entitlements:  resp.Entitlements,
		Organization:  resp.Organization,
	}

	// Convert timestamp if present
	if resp.ExpiresAt != nil {
		info.ExpiresAt = resp.ExpiresAt.AsTime()
	}

	return info, nil
}

// IsEnterpriseMode is a helper that checks if the host has an enterprise license
func IsEnterpriseMode(ctx context.Context) (bool, error) {
	info, err := GetLicenseInfo(ctx)
	if err != nil {
		return false, err
	}
	return info.LicenseType == "enterprise" && info.LicenseValid, nil
}

// HasEntitlement checks if a specific feature entitlement is enabled
func HasEntitlement(ctx context.Context, entitlement string) (bool, error) {
	info, err := GetLicenseInfo(ctx)
	if err != nil {
		return false, err
	}

	for _, e := range info.Entitlements {
		if e == entitlement {
			return true, nil
		}
	}
	return false, nil
}