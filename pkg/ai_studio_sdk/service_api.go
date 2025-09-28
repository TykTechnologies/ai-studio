package ai_studio_sdk

import (
	"context"
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
// This is called when the plugin receives the broker ID from config
func SetServiceBrokerID(brokerID uint32) {
	initMutex.Lock()
	defer initMutex.Unlock()

	serviceBrokerID = brokerID
	log.Info().Uint32("broker_id", brokerID).Msg("✅ Service broker ID set for host service access")
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