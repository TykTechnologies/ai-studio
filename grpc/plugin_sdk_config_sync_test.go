package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	sgrpc "github.com/TykTechnologies/midsommar/v2/services/grpc"
	"github.com/stretchr/testify/require"
)

// Plugin SDK config writes (UpdatePluginConfig, UpdateLLMPlugins) reach the
// control server checksum: an in-sync edge turns pending on a real change and
// stays in sync on a no-op write (53e.27).
func TestPluginSDKConfigWritesMarkEdgesPending(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	plugin := &models.Plugin{Name: "lb", Command: "/bin/lb", HookType: models.HookTypePostAuth,
		HookTypes: []string{models.HookTypePostAuth}, IsActive: true,
		Config: map[string]interface{}{"pools": []interface{}{map[string]interface{}{"name": "p", "up_b": true}}}}
	require.NoError(t, db.Create(plugin).Error)
	require.NoError(t, db.Create(&models.LLMPlugin{LLMID: llms[0].ID, PluginID: plugin.ID, IsActive: true}).Error)
	edge := models.EdgeInstance{EdgeID: "edge-1", Namespace: "", Status: models.EdgeStatusRegistered, SyncStatus: models.EdgeSyncStatusInSync}
	require.NoError(t, db.Create(&edge).Error)

	service := services.NewService(db)
	mgmt := sgrpc.NewAIStudioManagementServer(service)
	bus := eventbridge.NewBus()
	service.SetEventBus(bus)
	server.eventBus = bus
	server.subscribeToConfigChanges()

	syncOf := func() string {
		var e models.EdgeInstance
		require.NoError(t, db.Where("edge_id = ?", "edge-1").First(&e).Error)
		return e.SyncStatus
	}
	// Baseline checksum, edge in sync.
	server.onConfigurationChanged(topicPluginUpdated, eventbridge.Event{ID: "base"})
	require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("sync_status", models.EdgeSyncStatusInSync).Error)
	require.Equal(t, models.EdgeSyncStatusInSync, syncOf())

	// No-op config write: checksum unchanged, stays in sync.
	ctx := sgrpc.SetPluginIDInContext(context.Background(), plugin.ID)
	_, err := mgmt.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{PluginId: uint32(plugin.ID), ConfigJson: `{"pools":[{"name":"p","up_b":true}]}`})
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, models.EdgeSyncStatusInSync, syncOf(), "no-op write should not churn")

	_, err = mgmt.UpdatePluginConfig(ctx, &pb.UpdatePluginConfigRequest{PluginId: uint32(plugin.ID), ConfigJson: `{"pools":[{"name":"p","up_b":false}]}`})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return syncOf() == models.EdgeSyncStatusPending }, 3*time.Second, 50*time.Millisecond)

	// No-op plugin chain write (the LLM's existing plugin set): stays in sync.
	require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("sync_status", models.EdgeSyncStatusInSync).Error)
	_, err = mgmt.UpdateLLMPlugins(context.Background(), &pb.UpdateLLMPluginsRequest{LlmId: uint32(llms[0].ID), PluginIds: []uint32{uint32(plugin.ID)}})
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, models.EdgeSyncStatusInSync, syncOf(), "no-op plugin chain write should not churn")

	// Attaching the plugin to another LLM through the SDK.
	_, err = mgmt.UpdateLLMPlugins(context.Background(), &pb.UpdateLLMPluginsRequest{LlmId: uint32(llms[1].ID), PluginIds: []uint32{uint32(plugin.ID)}})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return syncOf() == models.EdgeSyncStatusPending }, 3*time.Second, 50*time.Millisecond)
}
