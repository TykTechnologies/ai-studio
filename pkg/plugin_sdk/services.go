package plugin_sdk

import (
	"context"
	"fmt"
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	mgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

// defaultServiceBroker provides the standard service broker implementation.
// It wraps the ai_studio_sdk to provide access to host services.
type defaultServiceBroker struct {
	runtime  RuntimeType
	pluginID uint32
	kv       KVService
	logger   LogService
	appMgr   AppManagerService
}

// newServiceBroker creates a service broker for the given runtime
func newServiceBroker(runtime RuntimeType, pluginID uint32) ServiceBroker {
	broker := &defaultServiceBroker{
		runtime:  runtime,
		pluginID: pluginID,
	}

	// Create services
	broker.kv = &defaultKVService{runtime: runtime}
	broker.logger = &defaultLogService{runtime: runtime, pluginID: pluginID}
	broker.appMgr = &defaultAppManager{runtime: runtime}

	return broker
}

func (b *defaultServiceBroker) KV() KVService {
	return b.kv
}

func (b *defaultServiceBroker) Logger() LogService {
	return b.logger
}

func (b *defaultServiceBroker) AppManager() AppManagerService {
	return b.appMgr
}

// defaultKVService implements KVService using ai_studio_sdk
type defaultKVService struct {
	runtime RuntimeType
}

func (kv *defaultKVService) Read(ctx context.Context, key string) ([]byte, error) {
	return ai_studio_sdk.ReadPluginKV(ctx, key)
}

func (kv *defaultKVService) Write(ctx context.Context, key string, value []byte) (bool, error) {
	return ai_studio_sdk.WritePluginKV(ctx, key, value)
}

func (kv *defaultKVService) Delete(ctx context.Context, key string) (bool, error) {
	return ai_studio_sdk.DeletePluginKV(ctx, key)
}

func (kv *defaultKVService) List(ctx context.Context, prefix string) ([]string, error) {
	// Note: The ai_studio_sdk doesn't have a List function yet
	// This is a limitation we'll need to work around
	return nil, fmt.Errorf("list operation not yet supported")
}

// defaultLogService implements LogService
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

// defaultAppManager implements AppManagerService using ai_studio_sdk
type defaultAppManager struct {
	runtime RuntimeType
}

func (a *defaultAppManager) GetApp(ctx context.Context, appID uint32) (*mgmt.GetAppResponse, error) {
	return ai_studio_sdk.GetApp(ctx, appID)
}

func (a *defaultAppManager) ListApps(ctx context.Context, page, limit int32) (*mgmt.ListAppsResponse, error) {
	return ai_studio_sdk.ListApps(ctx, page, limit)
}

func (a *defaultAppManager) UpdateApp(ctx context.Context, req *mgmt.UpdateAppRequest) (*mgmt.UpdateAppResponse, error) {
	// Delegate to the full update function
	return ai_studio_sdk.UpdateAppWithMetadata(
		ctx,
		req.AppId,
		req.Name,
		req.Description,
		req.IsActive,
		req.LlmIds,
		req.ToolIds,
		req.DatasourceIds,
		req.MonthlyBudget,
		req.Metadata,
	)
}

func (a *defaultAppManager) ListLLMs(ctx context.Context, page, limit int32) (*mgmt.ListLLMsResponse, error) {
	return ai_studio_sdk.ListLLMs(ctx, page, limit)
}
