package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	pb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// Global SDK state for service API access
var (
	serviceClient   pb.MicrogatewayManagementServiceClient
	pluginID        uint32
	serviceBrokerID uint32
	initialized     bool
	initMutex       sync.Mutex
	grpcBroker      *goplugin.GRPCBroker
)

// Initialize sets up the SDK with broker access
// This is called from the plugin's GRPCServer method
func InitializeServiceAPI(server *grpc.Server, broker *goplugin.GRPCBroker, pluginIDVal uint32) error {
	initMutex.Lock()
	defer initMutex.Unlock()

	if initialized {
		log.Debug().Msg("Service API SDK already initialized")
		return nil
	}

	// Store broker for service API access
	grpcBroker = broker
	pluginID = pluginIDVal // May be 0 initially, updated later
	initialized = true

	log.Info().Uint32("plugin_id", pluginID).Msg("✅ Microgateway service API SDK initialized with broker access")
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

// ExtractBrokerIDFromConfig extracts the broker ID from plugin config
// Call this in your Initialize method
func ExtractBrokerIDFromConfig(config map[string]string) uint32 {
	if brokerIDStr, ok := config["_service_broker_id"]; ok {
		var brokerID uint32
		fmt.Sscanf(brokerIDStr, "%d", &brokerID)
		return brokerID
	}
	return 0
}

// ExtractPluginIDFromConfig extracts the plugin ID from config
func ExtractPluginIDFromConfig(config map[string]string) uint32 {
	if pluginIDStr, ok := config["_plugin_id"]; ok {
		var id uint32
		fmt.Sscanf(pluginIDStr, "%d", &id)
		return id
	}
	return 0
}

// SetPluginID updates the plugin ID after it's received from config
func SetPluginID(id uint32) {
	initMutex.Lock()
	defer initMutex.Unlock()

	pluginID = id
	log.Info().Uint32("plugin_id", pluginID).Msg("✅ Plugin ID updated in SDK")
}

// getServiceClient creates and returns the service client, creating it if necessary
func getServiceClient(ctx context.Context) (pb.MicrogatewayManagementServiceClient, error) {
	if serviceClient != nil {
		return serviceClient, nil
	}

	if !initialized || grpcBroker == nil {
		return nil, fmt.Errorf("SDK not initialized - call InitializeServiceAPI() first")
	}

	if serviceBrokerID == 0 {
		return nil, fmt.Errorf("service broker ID not set - call SetServiceBrokerID() with broker ID from config")
	}

	// Dial the brokered server where microgateway management services are registered
	conn, err := grpcBroker.Dial(serviceBrokerID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial service broker ID %d: %w", serviceBrokerID, err)
	}

	// Create service client from the brokered connection
	serviceClient = pb.NewMicrogatewayManagementServiceClient(conn)

	log.Info().
		Uint32("plugin_id", pluginID).
		Uint32("broker_id", serviceBrokerID).
		Msg("✅ Service client created via broker dial - plugin can now call host services")

	return serviceClient, nil
}

// createPluginContext creates the authentication context for service API calls
func createPluginContext(methodScope string) *pb.PluginContext {
	return &pb.PluginContext{
		PluginId:    pluginID,
		MethodScope: methodScope,
	}
}

// IsInitialized returns whether the SDK has been initialized
func IsInitialized() bool {
	initMutex.Lock()
	defer initMutex.Unlock()
	return initialized && serviceBrokerID != 0
}

// LLM Management Functions

// ListLLMs returns a list of LLMs from the microgateway
func ListLLMs(ctx context.Context, page, limit int32, vendor string, isActive *bool) (*pb.ListLLMsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListLLMs(ctx, &pb.ListLLMsRequest{
		Context:  createPluginContext("llms.read"),
		Vendor:   vendor,
		IsActive: isActive,
		Page:     page,
		Limit:    limit,
	})
}

// GetLLM returns details for a specific LLM
func GetLLM(ctx context.Context, llmID uint32) (*pb.GetLLMResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetLLM(ctx, &pb.GetLLMRequest{
		Context: createPluginContext("llms.read"),
		LlmId:   llmID,
	})
}

// App Management Functions

// ListApps returns a list of applications
func ListApps(ctx context.Context, page, limit int32, isActive *bool) (*pb.ListAppsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListApps(ctx, &pb.ListAppsRequest{
		Context:  createPluginContext("apps.read"),
		IsActive: isActive,
		Page:     page,
		Limit:    limit,
	})
}

// GetApp returns details for a specific application
func GetApp(ctx context.Context, appID uint32) (*pb.GetAppResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetApp(ctx, &pb.GetAppRequest{
		Context: createPluginContext("apps.read"),
		AppId:   appID,
	})
}

// Budget Functions

// GetBudgetStatus returns budget status for an app
func GetBudgetStatus(ctx context.Context, appID uint32, llmID *uint32) (*pb.GetBudgetStatusResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetBudgetStatus(ctx, &pb.GetBudgetStatusRequest{
		Context: createPluginContext("budget.read"),
		AppId:   appID,
		LlmId:   llmID,
	})
}

// Model Price Functions

// ListModelPrices returns a list of model prices
func ListModelPrices(ctx context.Context, vendor string) (*pb.ListModelPricesResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ListModelPrices(ctx, &pb.ListModelPricesRequest{
		Context: createPluginContext("pricing.read"),
		Vendor:  vendor,
	})
}

// GetModelPrice returns price for a specific model
func GetModelPrice(ctx context.Context, modelName, vendor string) (*pb.GetModelPriceResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.GetModelPrice(ctx, &pb.GetModelPriceRequest{
		Context:   createPluginContext("pricing.read"),
		ModelName: modelName,
		Vendor:    vendor,
	})
}

// Credential Functions

// ValidateCredential validates a credential secret
func ValidateCredential(ctx context.Context, secret string) (*pb.ValidateCredentialResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	return client.ValidateCredential(ctx, &pb.ValidateCredentialRequest{
		Context: createPluginContext("credentials.validate"),
		Secret:  secret,
	})
}

// Plugin KV Storage Functions

// WritePluginKV writes a key-value entry for the calling plugin
// Returns true if a new entry was created, false if an existing entry was updated
func WritePluginKV(ctx context.Context, key string, value []byte) (bool, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return false, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.WritePluginKV(ctx, &pb.WritePluginKVRequest{
		Context: createPluginContext("kv.readwrite"),
		Key:     key,
		Value:   value,
	})
	if err != nil {
		return false, fmt.Errorf("failed to write KV data: %w", err)
	}

	return resp.Created, nil
}

// ReadPluginKV reads a key-value entry for the calling plugin
func ReadPluginKV(ctx context.Context, key string) ([]byte, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.ReadPluginKV(ctx, &pb.ReadPluginKVRequest{
		Context: createPluginContext("kv.readwrite"),
		Key:     key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read KV data: %w", err)
	}

	return resp.Value, nil
}

// DeletePluginKV deletes a key-value entry for the calling plugin
func DeletePluginKV(ctx context.Context, key string) (bool, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return false, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.DeletePluginKV(ctx, &pb.DeletePluginKVRequest{
		Context: createPluginContext("kv.readwrite"),
		Key:     key,
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete KV data: %w", err)
	}

	return resp.Deleted, nil
}

// Helper Functions

// WritePluginKVJSON writes a JSON-encodable value to KV storage
func WritePluginKVJSON(ctx context.Context, key string, value interface{}) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("failed to marshal value: %w", err)
	}
	return WritePluginKV(ctx, key, data)
}

// ReadPluginKVJSON reads and unmarshals a JSON value from KV storage
func ReadPluginKVJSON(ctx context.Context, key string, target interface{}) error {
	data, err := ReadPluginKV(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// GetLLMsCount returns the total number of LLMs
func GetLLMsCount(ctx context.Context) (int, error) {
	resp, err := ListLLMs(ctx, 1, 1, "", nil)
	if err != nil {
		return 0, err
	}
	return int(resp.TotalCount), nil
}

// GetAppsCount returns the total number of apps
func GetAppsCount(ctx context.Context) (int, error) {
	resp, err := ListApps(ctx, 1, 1, nil)
	if err != nil {
		return 0, err
	}
	return int(resp.TotalCount), nil
}
