package plugins

import (
	"context"
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
)

// mockPluginService implements PluginServiceInterface for testing reconciliation.
type mockPluginService struct {
	plugins            []PluginData
	pluginsByLLM       map[uint][]PluginData
	llmIDs             []uint
	allActiveGwPlugins []PluginData
	getPluginByID      map[uint]PluginData
	errGetAllActive    error
	errGetPlugin       error
}

func (m *mockPluginService) GetPlugin(id uint) (PluginData, error) {
	if m.errGetPlugin != nil {
		return PluginData{}, m.errGetPlugin
	}
	if p, ok := m.getPluginByID[id]; ok {
		return p, nil
	}
	return PluginData{}, fmt.Errorf("plugin %d not found", id)
}

func (m *mockPluginService) GetPluginsByLLMID(llmID uint) ([]PluginData, error) {
	return m.GetPluginsForLLM(llmID)
}

func (m *mockPluginService) GetPluginsForLLM(llmID uint) ([]PluginData, error) {
	return m.pluginsByLLM[llmID], nil
}

func (m *mockPluginService) GetAllPlugins() ([]PluginData, error) {
	return m.plugins, nil
}

func (m *mockPluginService) GetAllLLMIDs() ([]uint, error) {
	return m.llmIDs, nil
}

func (m *mockPluginService) GetAllActiveGatewayPlugins() ([]PluginData, error) {
	if m.errGetAllActive != nil {
		return nil, m.errGetAllActive
	}
	return m.allActiveGwPlugins, nil
}

// noopDataCollector satisfies interfaces.DataCollectionPlugin for builtin detection tests.
type noopDataCollector struct{}

func (n *noopDataCollector) Initialize(config map[string]interface{}) error { return nil }
func (n *noopDataCollector) GetHookType() interfaces.HookType              { return "data_collection" }
func (n *noopDataCollector) GetName() string                               { return "noop" }
func (n *noopDataCollector) GetVersion() string                            { return "0.0.0" }
func (n *noopDataCollector) Shutdown() error                               { return nil }
func (n *noopDataCollector) HandleProxyLog(_ context.Context, _ *interfaces.ProxyLogData, _ *interfaces.PluginContext) (*interfaces.DataCollectionResponse, error) {
	return &interfaces.DataCollectionResponse{Success: true}, nil
}
func (n *noopDataCollector) HandleAnalytics(_ context.Context, _ *interfaces.AnalyticsData, _ *interfaces.PluginContext) (*interfaces.DataCollectionResponse, error) {
	return &interfaces.DataCollectionResponse{Success: true}, nil
}
func (n *noopDataCollector) HandleBudgetUsage(_ context.Context, _ *interfaces.BudgetUsageData, _ *interfaces.PluginContext) (*interfaces.DataCollectionResponse, error) {
	return &interfaces.DataCollectionResponse{Success: true}, nil
}

// helper to create a PluginManager with pre-loaded plugins for testing.
func newTestManager(svc *mockPluginService, loaded map[uint]*LoadedPlugin) *PluginManager {
	pm := NewPluginManager(svc)
	if loaded != nil {
		pm.loadedPlugins = loaded
	}
	return pm
}

func TestGetDesiredPluginState(t *testing.T) {
	t.Run("returns plugins from service", func(t *testing.T) {
		svc := &mockPluginService{
			allActiveGwPlugins: []PluginData{
				{ID: 1, Name: "p1", Checksum: "aaa", IsActive: true, HookType: "post_auth"},
				{ID: 2, Name: "p2", Checksum: "bbb", IsActive: true, HookType: "custom_endpoint"},
			},
		}
		pm := newTestManager(svc, nil)

		state, err := pm.getDesiredPluginState()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(state) != 2 {
			t.Fatalf("expected 2 plugins, got %d", len(state))
		}
		if state[1].Checksum != "aaa" {
			t.Errorf("plugin 1 checksum = %q, want %q", state[1].Checksum, "aaa")
		}
		if state[2].Checksum != "bbb" {
			t.Errorf("plugin 2 checksum = %q, want %q", state[2].Checksum, "bbb")
		}
	})

	t.Run("returns error on service failure", func(t *testing.T) {
		svc := &mockPluginService{
			errGetAllActive: fmt.Errorf("db connection lost"),
		}
		pm := newTestManager(svc, nil)

		_, err := pm.getDesiredPluginState()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestReconcilePlugins_AbortsOnDBError(t *testing.T) {
	svc := &mockPluginService{
		errGetAllActive: fmt.Errorf("db connection lost"),
	}
	pm := newTestManager(svc, map[uint]*LoadedPlugin{
		1: {ID: 1, Name: "p1", Checksum: "aaa"},
	})

	err := pm.ReconcilePlugins(context.Background())
	if err == nil {
		t.Fatal("expected error when DB fails, got nil")
	}

	// Verify plugin was NOT unloaded (should still be loaded)
	pm.mu.RLock()
	_, stillLoaded := pm.loadedPlugins[1]
	pm.mu.RUnlock()
	if !stillLoaded {
		t.Error("plugin 1 should still be loaded after aborted reconciliation")
	}
}

func TestReconcilePlugins_NoChanges(t *testing.T) {
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{
			{ID: 1, Name: "p1", Checksum: "aaa", IsActive: true, HookType: "post_auth"},
		},
		getPluginByID: map[uint]PluginData{
			1: {ID: 1, Name: "p1", Checksum: "aaa", IsActive: true, HookType: "post_auth"},
		},
	}
	pm := newTestManager(svc, map[uint]*LoadedPlugin{
		1: {ID: 1, Name: "p1", Checksum: "aaa"},
	})

	err := pm.ReconcilePlugins(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Plugin should still be loaded
	pm.mu.RLock()
	_, stillLoaded := pm.loadedPlugins[1]
	pm.mu.RUnlock()
	if !stillLoaded {
		t.Error("plugin 1 should still be loaded when no changes")
	}
}

func TestReconcilePlugins_SkipsGlobalAndBuiltin(t *testing.T) {
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{}, // empty desired state
	}
	pm := newTestManager(svc, map[uint]*LoadedPlugin{
		1: {ID: 1, Name: "global-plugin", Checksum: "aaa", IsGlobal: true},
		2: {ID: 2, Name: "builtin-plugin", Checksum: "bbb", BuiltinPlugin: &noopDataCollector{}},
	})

	err := pm.ReconcilePlugins(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Global and builtin plugins should NOT be unloaded even though they're not in desired state
	pm.mu.RLock()
	_, p1Loaded := pm.loadedPlugins[1]
	_, p2Loaded := pm.loadedPlugins[2]
	pm.mu.RUnlock()

	if !p1Loaded {
		t.Error("global plugin should not be unloaded by reconciliation")
	}
	if !p2Loaded {
		t.Error("builtin plugin should not be unloaded by reconciliation")
	}
}

func TestReconcilePlugins_DetectsRemovedPlugins(t *testing.T) {
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{
			// Only plugin 1 is desired; plugin 2 is not
			{ID: 1, Name: "p1", Checksum: "aaa", IsActive: true, HookType: "post_auth"},
		},
		getPluginByID: map[uint]PluginData{
			1: {ID: 1, Name: "p1", Checksum: "aaa", IsActive: true, HookType: "post_auth"},
		},
	}
	pm := newTestManager(svc, map[uint]*LoadedPlugin{
		1: {ID: 1, Name: "p1", Checksum: "aaa"},
		2: {ID: 2, Name: "p2-removed", Checksum: "bbb"},
	})

	err := pm.ReconcilePlugins(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pm.mu.RLock()
	_, p1Loaded := pm.loadedPlugins[1]
	_, p2Loaded := pm.loadedPlugins[2]
	pm.mu.RUnlock()

	if !p1Loaded {
		t.Error("plugin 1 should still be loaded")
	}
	if p2Loaded {
		t.Error("plugin 2 should have been unloaded (removed from desired state)")
	}
}

func TestReconcilePlugins_DetectsChangedChecksum(t *testing.T) {
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{
			{ID: 1, Name: "p1", Checksum: "new-checksum", IsActive: true, HookType: "post_auth"},
		},
		getPluginByID: map[uint]PluginData{
			1: {ID: 1, Name: "p1", Checksum: "new-checksum", IsActive: true, HookType: "post_auth"},
		},
	}
	pm := newTestManager(svc, map[uint]*LoadedPlugin{
		1: {ID: 1, Name: "p1", Checksum: "old-checksum"},
	})

	// ReloadPlugin will call UnloadPlugin (succeeds) then LoadPlugin (will fail
	// since there's no actual gRPC process). That's expected in unit tests.
	_ = pm.ReconcilePlugins(context.Background())

	// After the reload attempt, the old plugin should be gone (UnloadPlugin
	// succeeds as part of ReloadPlugin) and LoadPlugin fails since there's no
	// real process. The key assertion is that the stale checksum is gone.
	pm.mu.RLock()
	lp, exists := pm.loadedPlugins[1]
	pm.mu.RUnlock()

	if exists && lp.Checksum == "old-checksum" {
		t.Error("plugin 1 should have been reloaded (old checksum should be gone)")
	}
}

func TestReconcilePlugins_SerializesConcurrentCalls(t *testing.T) {
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{},
	}
	pm := newTestManager(svc, nil)

	// Run two reconciliations concurrently - should not panic or race
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			done <- pm.ReconcilePlugins(context.Background())
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Errorf("reconciliation error: %v", err)
		}
	}
}
