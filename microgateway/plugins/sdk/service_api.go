package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
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

	// Always update broker reference - handles both initial setup and plugin reloads
	// If broker changed, invalidate any cached client
	if grpcBroker != broker {
		if serviceClient != nil {
			log.Info().Msg("Broker changed, invalidating cached service client")
			serviceClient = nil
		}
		serviceBrokerID = 0 // Reset broker ID so it must be set again
	}

	// Store broker for service API access
	grpcBroker = broker
	pluginID = pluginIDVal // May be 0 initially, updated later via SetPluginID
	initialized = true

	log.Info().
		Uint32("plugin_id", pluginID).
		Bool("broker_is_nil", broker == nil).
		Str("broker_ptr", fmt.Sprintf("%p", broker)).
		Msg("✅ Microgateway service API SDK initialized with broker access (PLUGIN SIDE)")
	return nil
}

// SetServiceBrokerID stores the broker ID for dialing back to host services
// This is called when the plugin receives the broker ID from config
func SetServiceBrokerID(brokerID uint32) {
	initMutex.Lock()
	defer initMutex.Unlock()

	// If broker ID changed, invalidate the cached service client
	// This handles plugin reloads where a new broker session is established
	if serviceBrokerID != brokerID && serviceClient != nil {
		log.Info().
			Uint32("old_broker_id", serviceBrokerID).
			Uint32("new_broker_id", brokerID).
			Msg("Broker ID changed, invalidating cached service client")
		serviceClient = nil
	}

	serviceBrokerID = brokerID
	log.Info().
		Uint32("broker_id", brokerID).
		Bool("initialized", initialized).
		Bool("has_broker", grpcBroker != nil).
		Msg("✅ Service broker ID set for host service access")
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
// Includes retry logic to handle race conditions where the broker server may not be ready yet
func getServiceClient(ctx context.Context) (pb.MicrogatewayManagementServiceClient, error) {
	if serviceClient != nil {
		return serviceClient, nil
	}

	log.Debug().
		Bool("initialized", initialized).
		Bool("has_broker", grpcBroker != nil).
		Uint32("broker_id", serviceBrokerID).
		Str("broker_ptr", fmt.Sprintf("%p", grpcBroker)).
		Msg("getServiceClient: checking prerequisites (PLUGIN SIDE dial attempt)")

	if !initialized || grpcBroker == nil {
		return nil, fmt.Errorf("SDK not initialized - call InitializeServiceAPI() first (initialized=%v, broker=%v)", initialized, grpcBroker != nil)
	}

	if serviceBrokerID == 0 {
		return nil, fmt.Errorf("service broker ID not set - call SetServiceBrokerID() with broker ID from config")
	}

	// Dial the brokered server where microgateway management services are registered
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
// expireAt is optional - pass nil for no expiration
func WritePluginKV(ctx context.Context, key string, value []byte, expireAt *time.Time) (bool, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return false, fmt.Errorf("service client unavailable: %w", err)
	}

	req := &pb.WritePluginKVRequest{
		Context: createPluginContext("kv.readwrite"),
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

// WritePluginKVJSON writes a JSON-encodable value to KV storage without expiration
func WritePluginKVJSON(ctx context.Context, key string, value interface{}) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("failed to marshal value: %w", err)
	}
	return WritePluginKV(ctx, key, data, nil)
}

// WritePluginKVJSONWithTTL writes a JSON-encodable value to KV storage with a TTL
func WritePluginKVJSONWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("failed to marshal value: %w", err)
	}
	return WritePluginKVWithTTL(ctx, key, data, ttl)
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

// Control Payload Functions (for edge-to-control plugin communication)

// SendToControl queues a payload to be sent to the AI Studio control plane
// This is used by plugins running on edge (microgateway) instances to send
// arbitrary data back to the control plane where it will be routed to the
// corresponding plugin via AcceptEdgePayload.
//
// Parameters:
//   - ctx: Context for the RPC call
//   - payload: Arbitrary binary data (max 1MB)
//   - correlationID: Optional correlation ID for tracking (can be empty)
//   - metadata: Optional key-value metadata (can be nil)
//
// Returns:
//   - pendingCount: Number of payloads currently pending in the queue
//   - error: Error if the payload could not be queued
func SendToControl(ctx context.Context, payload []byte, correlationID string, metadata map[string]string) (int64, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return 0, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.QueueControlPayload(ctx, &pb.QueueControlPayloadRequest{
		Context:       createPluginContext("control.send"),
		Payload:       payload,
		CorrelationId: correlationID,
		Metadata:      metadata,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to queue control payload: %w", err)
	}

	if !resp.Success {
		return resp.PendingCount, fmt.Errorf("control payload rejected: %s", resp.ErrorMessage)
	}

	return resp.PendingCount, nil
}

// SendToControlJSON is a convenience function that JSON-encodes a value and sends it to control
func SendToControlJSON(ctx context.Context, value interface{}, correlationID string, metadata map[string]string) (int64, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return SendToControl(ctx, data, correlationID, metadata)
}
