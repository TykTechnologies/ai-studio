package ai_studio_sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// Global SDK state for service API access
var (
	serviceClient   mgmtpb.AIStudioManagementServiceClient
	pluginID        uint32
	serviceBrokerID uint32
	initialized     bool
	initMutex       sync.Mutex
	grpcBroker      *goplugin.GRPCBroker
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
	log.Info().Uint32("broker_id", brokerID).Msg("✅ Service broker ID set for host service access")
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

// getServiceClient creates and returns the service client, creating it if necessary
func getServiceClient(ctx context.Context) (mgmtpb.AIStudioManagementServiceClient, error) {
	if serviceClient != nil {
		return serviceClient, nil
	}

	if !initialized || grpcBroker == nil {
		return nil, fmt.Errorf("SDK not initialized - call ai_studio_sdk.Initialize() first")
	}

	if serviceBrokerID == 0 {
		return nil, fmt.Errorf("service broker ID not set - call ai_studio_sdk.SetServiceBrokerID() with broker ID from config")
	}

	// Dial the brokered server where AI Studio management services are registered
	// This follows the go-plugin bidirectional pattern
	conn, err := grpcBroker.Dial(serviceBrokerID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial service broker ID %d: %w", serviceBrokerID, err)
	}

	// Create service client from the brokered connection
	serviceClient = mgmtpb.NewAIStudioManagementServiceClient(conn)

	log.Info().
		Uint32("plugin_id", pluginID).
		Uint32("broker_id", serviceBrokerID).
		Msg("✅ Service client created via broker dial - plugin can now call host services")

	return serviceClient, nil
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

// IsInitialized returns whether the SDK has been initialized
func IsInitialized() bool {
	initMutex.Lock()
	defer initMutex.Unlock()
	return initialized
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
func WritePluginKV(ctx context.Context, key string, value []byte) (bool, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return false, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.WritePluginKV(ctx, &mgmtpb.WritePluginKVRequest{
		Context: createPluginContext(AvailableScopes.KVReadWrite),
		Key:     key,
		Value:   value,
	})
	if err != nil {
		return false, fmt.Errorf("failed to write KV data: %w", err)
	}

	return resp.Created, nil
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

// CreateDatasource creates a new datasource
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