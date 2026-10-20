package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/brokerdial"
	pb "github.com/TykTechnologies/midsommar/v2/proto/microgateway_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Global SDK state for service API access
var (
	pluginID        uint32
	serviceBrokerID uint32
	initialized     bool
	initMutex       sync.Mutex
	grpcBroker      *goplugin.GRPCBroker

	// brokerConn is the one connection to the host's brokered server,
	// shared with other services (e.g., the event service).
	brokerConn brokerdial.Conn

	// serviceClient is the management client on brokerConn's connection.
	clientMu      sync.Mutex
	serviceClient pb.MicrogatewayManagementServiceClient
	clientConn    *grpc.ClientConn
)

// Initialize sets up the SDK with broker access
// This is called from the plugin's GRPCServer method
func InitializeServiceAPI(server *grpc.Server, broker *goplugin.GRPCBroker, pluginIDVal uint32) error {
	initMutex.Lock()
	defer initMutex.Unlock()

	// Always update broker reference - handles both initial setup and plugin reloads
	// If broker changed, invalidate any cached client
	if grpcBroker != broker {
		if brokerConn.Current() != nil {
			log.Info().Msg("Broker changed, invalidating cached service client")
		}
		serviceBrokerID = 0 // Reset broker ID so it must be set again
		brokerConn.SetID(0, false)
	}

	// Store broker for service API access
	grpcBroker = broker
	brokerConn.SetBroker(broker)
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
	if serviceBrokerID != brokerID && brokerConn.Current() != nil {
		log.Info().
			Uint32("old_broker_id", serviceBrokerID).
			Uint32("new_broker_id", brokerID).
			Msg("Broker ID changed, invalidating cached service client")
	}

	serviceBrokerID = brokerID
	brokerConn.SetID(brokerID, false)
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

// getServiceClient returns the management client on the shared broker
// connection, dialling it if needed. It waits for the dial at most until ctx
// ends (brokerdial.Conn): a dial the caller gave up on still completes, and
// its connection serves the next call. Only one dial is ever in flight, and
// a failed one is not retried for brokerdial.RetryAfter.
func getServiceClient(ctx context.Context) (pb.MicrogatewayManagementServiceClient, error) {
	initMutex.Lock()
	ready, hasBroker, id := initialized, grpcBroker != nil, serviceBrokerID
	initMutex.Unlock()
	if !ready || !hasBroker {
		return nil, fmt.Errorf("SDK not initialized - call InitializeServiceAPI() first (initialized=%v, broker=%v)", ready, hasBroker)
	}
	if id == 0 {
		return nil, fmt.Errorf("service broker ID not set - call SetServiceBrokerID() with broker ID from config")
	}

	conn, err := brokerConn.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to dial service broker ID %d: %w", id, err)
	}

	clientMu.Lock()
	defer clientMu.Unlock()
	if serviceClient == nil || clientConn != conn {
		serviceClient = pb.NewMicrogatewayManagementServiceClient(conn)
		clientConn = conn
		log.Info().
			Uint32("broker_id", id).
			Msg("✅ Service client created via broker dial - plugin can now call host services")
	}
	return serviceClient, nil
}

// createPluginContext creates the authentication context for service API calls
func createPluginContext(methodScope string) *pb.PluginContext {
	initMutex.Lock()
	id := pluginID
	initMutex.Unlock()
	return &pb.PluginContext{
		PluginId:    id,
		MethodScope: methodScope,
	}
}

// IsInitialized returns whether the SDK has been initialized
func IsInitialized() bool {
	initMutex.Lock()
	defer initMutex.Unlock()
	return initialized && serviceBrokerID != 0
}

// GetSharedBrokerConnection returns the shared gRPC connection to the host's brokered server.
// This allows other services (like the event service) to reuse the same connection.
// The go-plugin broker only allows ONE connection per broker ID.
func GetSharedBrokerConnection() *grpc.ClientConn {
	return brokerConn.Current()
}

// SetSharedBrokerConnection stores a pre-dialed gRPC connection for shared use,
// when none is open yet. This is called by services that dial the broker first.
func SetSharedBrokerConnection(conn *grpc.ClientConn) {
	brokerConn.Share(conn)
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

// StoreApp writes an App into the local gateway database using upsert semantics.
// Used by auth plugins that dynamically provision apps from the control plane.
func StoreApp(ctx context.Context, req *pb.StoreAppRequest) (*pb.StoreAppResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	if req.Context == nil {
		req.Context = createPluginContext("apps.write")
	}

	return client.StoreApp(ctx, req)
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

// License Information Functions

// LicenseInfo contains license information returned by GetLicenseInfo
type LicenseInfo struct {
	Valid        bool     // Whether license is currently valid
	DaysLeft     int      // Days until license expires (-1 = never expires)
	Type         string   // "community" or "enterprise"
	Entitlements []string // List of enabled features/entitlements
	Organization string   // Organization name from license (if any)
}

// GetLicenseInfo returns license information from the host
// This can be used by plugins to check if enterprise features are available
// All plugins can call this without requiring special scopes
func GetLicenseInfo(ctx context.Context) (*LicenseInfo, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}

	resp, err := client.GetLicenseInfo(ctx, &pb.GetLicenseInfoRequest{
		Context: createPluginContext(""), // No scope required
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get license info: %w", err)
	}

	return &LicenseInfo{
		Valid:        resp.LicenseValid,
		DaysLeft:     int(resp.DaysRemaining),
		Type:         resp.LicenseType,
		Entitlements: resp.Entitlements,
		Organization: resp.Organization,
	}, nil
}

// IsEnterprise returns true if the microgateway is running in enterprise mode
// This is a convenience function that checks if license type is "enterprise"
func IsEnterprise(ctx context.Context) (bool, error) {
	info, err := GetLicenseInfo(ctx)
	if err != nil {
		return false, err
	}
	return info.Type == "enterprise", nil
}
