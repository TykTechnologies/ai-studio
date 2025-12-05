package plugin_sdk

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
)

// defaultServiceBroker provides the service broker implementation with runtime-aware services
type defaultServiceBroker struct {
	runtime        RuntimeType
	pluginID       uint32
	kv             KVService
	logger         LogService
	gatewayService GatewayServices
	studioService  StudioServices
	eventService   EventService
}

// newServiceBroker creates a service broker for the given runtime
func newServiceBroker(runtime RuntimeType, pluginID uint32) ServiceBroker {
	broker := &defaultServiceBroker{
		runtime:  runtime,
		pluginID: pluginID,
	}

	// Create universal services (work in both contexts)
	broker.kv = &defaultKVService{runtime: runtime}
	broker.logger = &defaultLogService{runtime: runtime, pluginID: pluginID}

	// Create runtime-specific services
	if runtime == RuntimeGateway {
		broker.gatewayService = &gatewayServicesImpl{}
		broker.studioService = nil
	} else {
		broker.gatewayService = nil
		broker.studioService = &studioServicesImpl{}
	}

	// Event service is lazily initialized when first accessed
	// It needs the gRPC broker connection which is set up during plugin startup
	broker.eventService = &lazyEventService{
		runtime:  runtime,
		pluginID: fmt.Sprintf("%d", pluginID),
	}

	return broker
}

func (b *defaultServiceBroker) KV() KVService {
	return b.kv
}

func (b *defaultServiceBroker) Logger() LogService {
	return b.logger
}

func (b *defaultServiceBroker) Gateway() GatewayServices {
	return b.gatewayService
}

func (b *defaultServiceBroker) Studio() StudioServices {
	return b.studioService
}

func (b *defaultServiceBroker) Events() EventService {
	return b.eventService
}

// Cleanup releases all resources held by the service broker.
// This should be called during plugin shutdown.
func (b *defaultServiceBroker) Cleanup() {
	// Cleanup event service subscriptions
	if lazy, ok := b.eventService.(*lazyEventService); ok {
		lazy.Cleanup()
	}
}

// ===== KV Service (Runtime-Aware) =====

type defaultKVService struct {
	runtime RuntimeType
}

func (kv *defaultKVService) Read(ctx context.Context, key string) ([]byte, error) {
	if kv.runtime == RuntimeGateway {
		return readKVGateway(ctx, key)
	}
	return ai_studio_sdk.ReadPluginKV(ctx, key)
}

func (kv *defaultKVService) Write(ctx context.Context, key string, value []byte, expireAt *time.Time) (bool, error) {
	if kv.runtime == RuntimeGateway {
		return writeKVGateway(ctx, key, value, expireAt)
	}
	return ai_studio_sdk.WritePluginKV(ctx, key, value, expireAt)
}

func (kv *defaultKVService) WriteWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	expireAt := time.Now().Add(ttl)
	return kv.Write(ctx, key, value, &expireAt)
}

func (kv *defaultKVService) Delete(ctx context.Context, key string) (bool, error) {
	if kv.runtime == RuntimeGateway {
		return deleteKVGateway(ctx, key)
	}
	return ai_studio_sdk.DeletePluginKV(ctx, key)
}

func (kv *defaultKVService) List(ctx context.Context, prefix string) ([]string, error) {
	return nil, fmt.Errorf("list operation not yet supported")
}

// ===== Log Service (Universal) =====

type defaultLogService struct {
	runtime  RuntimeType
	pluginID uint32
}

func (l *defaultLogService) Debug(msg string, fields ...interface{}) {
	log.Printf("[DEBUG] [Plugin %d] %s %v", l.pluginID, msg, fields)
}

func (l *defaultLogService) Info(msg string, fields ...interface{}) {
	log.Printf("[INFO] [Plugin %d] %s %v", l.pluginID, msg, fields)
}

func (l *defaultLogService) Warn(msg string, fields ...interface{}) {
	log.Printf("[WARN] [Plugin %d] %s %v", l.pluginID, msg, fields)
}

func (l *defaultLogService) Error(msg string, fields ...interface{}) {
	log.Printf("[ERROR] [Plugin %d] %s %v", l.pluginID, msg, fields)
}

// ===== License Service (Runtime-Aware) =====

// LicenseInfo contains information about the host's license status.
// This is a unified type that works in both AI Studio and Microgateway contexts.
type LicenseInfo struct {
	// Valid indicates whether a valid enterprise license is present
	Valid bool

	// DaysRemaining is the number of days until license expires (-1 for community/never expires)
	DaysRemaining int

	// Type is the license type: "community" or "enterprise"
	Type string

	// Entitlements is a list of enabled features/entitlements
	Entitlements []string

	// Organization is the licensed organization name (enterprise only)
	Organization string

	// ExpiresAt is the license expiration timestamp (zero for community licenses)
	ExpiresAt time.Time
}

// IsEnterprise returns true if this is an enterprise license
func (l *LicenseInfo) IsEnterprise() bool {
	return l.Type == "enterprise" && l.Valid
}

// HasEntitlement checks if a specific entitlement is enabled
func (l *LicenseInfo) HasEntitlement(entitlement string) bool {
	for _, e := range l.Entitlements {
		if e == entitlement {
			return true
		}
	}
	return false
}

// GetLicenseInfo retrieves license information from the host.
// This is a runtime-aware function that works in both AI Studio and Microgateway contexts.
// All plugins can call this without requiring special scopes.
func GetLicenseInfo(ctx context.Context, runtime RuntimeType) (*LicenseInfo, error) {
	if runtime == RuntimeGateway {
		return getLicenseInfoGateway(ctx)
	}
	return getLicenseInfoStudio(ctx)
}

// getLicenseInfoStudio retrieves license info from AI Studio
func getLicenseInfoStudio(ctx context.Context) (*LicenseInfo, error) {
	info, err := ai_studio_sdk.GetLicenseInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &LicenseInfo{
		Valid:         info.LicenseValid,
		DaysRemaining: info.DaysRemaining,
		Type:          info.LicenseType,
		Entitlements:  info.Entitlements,
		Organization:  info.Organization,
		ExpiresAt:     info.ExpiresAt,
	}, nil
}

// ===== Gateway Services Implementation =====









// ===== Studio Services Implementation =====

type studioServicesImpl struct{}

func (s *studioServicesImpl) GetApp(ctx context.Context, appID uint32) (interface{}, error) {
	return ai_studio_sdk.GetApp(ctx, appID)
}

func (s *studioServicesImpl) ListApps(ctx context.Context, page, limit int32) (interface{}, error) {
	return ai_studio_sdk.ListApps(ctx, page, limit)
}

func (s *studioServicesImpl) UpdateAppWithMetadata(ctx context.Context, appID uint32, name, description string, isActive bool, llmIDs, toolIDs, datasourceIDs []uint32, monthlyBudget *float64, metadata string) (interface{}, error) {
	return ai_studio_sdk.UpdateAppWithMetadata(ctx, appID, name, description, isActive, llmIDs, toolIDs, datasourceIDs, monthlyBudget, metadata)
}

func (s *studioServicesImpl) GetLLM(ctx context.Context, llmID uint32) (interface{}, error) {
	return ai_studio_sdk.GetLLM(ctx, llmID)
}

func (s *studioServicesImpl) ListLLMs(ctx context.Context, page, limit int32) (interface{}, error) {
	return ai_studio_sdk.ListLLMs(ctx, page, limit)
}

func (s *studioServicesImpl) ListTools(ctx context.Context, page, limit int32) (interface{}, error) {
	return ai_studio_sdk.ListTools(ctx, page, limit)
}

func (s *studioServicesImpl) CallLLM(ctx context.Context, llmID uint32, model string, messages interface{}, temperature float64, maxTokens int32) (interface{}, error) {
	// TODO: Implement CallLLM wrapper when needed for agent plugins
	return nil, fmt.Errorf("CallLLM not yet implemented in unified SDK")
}

func (s *studioServicesImpl) UpdatePluginConfig(ctx context.Context, pluginID uint32, configJSON string) (bool, string, error) {
	resp, err := ai_studio_sdk.UpdatePluginConfig(ctx, pluginID, configJSON)
	if err != nil {
		return false, "", err
	}
	return resp.Success, resp.Message, nil
}
