package plugin_sdk

import (
	"context"
	"fmt"
	"log"

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

func (kv *defaultKVService) Write(ctx context.Context, key string, value []byte) (bool, error) {
	if kv.runtime == RuntimeGateway {
		return writeKVGateway(ctx, key, value)
	}
	return ai_studio_sdk.WritePluginKV(ctx, key, value)
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
