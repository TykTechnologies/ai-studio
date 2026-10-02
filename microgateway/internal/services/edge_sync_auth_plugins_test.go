package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func authPluginTestLLM(id uint32, slug string, now time.Time) *pb.LLMConfig {
	return &pb.LLMConfig{
		Id: id, Name: slug, Slug: slug, Vendor: "openai", IsActive: true,
		CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now),
	}
}

func authPluginTestPlugin(id uint32, llmIDs []uint32, now time.Time) *pb.PluginConfig {
	return &pb.PluginConfig{
		Id: id, Name: "auth", Command: "file:///auth", HookType: "auth",
		HookTypes: []string{"auth"}, IsActive: true, LlmIds: llmIDs,
		CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now),
	}
}

// A plugin attached to two LLMs used to arrive as two PluginConfig rows with
// the same id, and the second insert failed the whole sync. Studio now sends
// one row listing both LLMs; an older Studio may still send two, and the edge
// takes both.
func TestEdgeSync_PluginAttachedToTwoLLMs(t *testing.T) {
	now := time.Now()
	for name, plugins := range map[string][]*pb.PluginConfig{
		"one row":            {authPluginTestPlugin(7, []uint32{1, 2}, now)},
		"row per attachment": {authPluginTestPlugin(7, []uint32{1}, now), authPluginTestPlugin(7, []uint32{2}, now)},
	} {
		t.Run(name, func(t *testing.T) {
			db := setupEdgeSyncTestDB(t)
			svc := NewEdgeSyncService(db, "")
			err := svc.SyncConfiguration(&pb.ConfigurationSnapshot{
				Version:      "1",
				SnapshotTime: timestamppb.Now(),
				Llms:         []*pb.LLMConfig{authPluginTestLLM(1, "a", now), authPluginTestLLM(2, "b", now)},
				Plugins:      plugins,
			})
			require.NoError(t, err)

			var count int64
			require.NoError(t, db.Model(&database.Plugin{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
			for _, llmID := range []uint{1, 2} {
				var lp database.LLMPlugin
				assert.NoError(t, db.Where("llm_id = ? AND plugin_id = ?", llmID, 7).First(&lp).Error, "llm %d", llmID)
			}
		})
	}
}

// The auth plugin lists of datasources, tools, routers and custom-endpoint
// plugins are stored in order, and a later snapshot without them clears them.
func TestEdgeSync_EndpointAuthPlugins(t *testing.T) {
	now := time.Now()
	db := setupEdgeSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")

	endpointPlugin := authPluginTestPlugin(20, nil, now)
	endpointPlugin.HookType = "custom_endpoint"
	endpointPlugin.AuthPluginIds = []uint32{8}
	snapshot := func(withLists bool) *pb.ConfigurationSnapshot {
		ds := &pb.DatasourceConfig{Id: 3, Name: "docs", IsActive: true, CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now)}
		tool := &pb.ToolConfig{Id: 4, Name: "crm", Slug: "crm", IsActive: true, CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now)}
		router := &pb.ModelRouterConfig{Id: 5, Name: "r", Slug: "r", IsActive: true, CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now)}
		ep := proto.Clone(endpointPlugin).(*pb.PluginConfig)
		if withLists {
			ds.AuthPluginIds = []uint32{9, 8}
			tool.AuthPluginIds = []uint32{8}
			router.AuthPluginIds = []uint32{9}
		} else {
			ep.AuthPluginIds = nil
		}
		return &pb.ConfigurationSnapshot{
			Version:      "1",
			SnapshotTime: timestamppb.Now(),
			Plugins:      []*pb.PluginConfig{authPluginTestPlugin(8, nil, now), authPluginTestPlugin(9, nil, now), ep},
			Datasources:  []*pb.DatasourceConfig{ds},
			Tools:        []*pb.ToolConfig{tool},
			ModelRouters: []*pb.ModelRouterConfig{router},
		}
	}

	require.NoError(t, svc.SyncConfiguration(snapshot(true)))
	list := func(objectType string, id uint) []uint {
		var ids []uint
		require.NoError(t, db.Model(&database.EndpointAuthPlugin{}).
			Where("object_type = ? AND object_id = ?", objectType, id).
			Order("order_index ASC").Pluck("plugin_id", &ids).Error)
		return ids
	}
	assert.Equal(t, []uint{9, 8}, list(database.EndpointTypeDatasource, 3))
	assert.Equal(t, []uint{8}, list(database.EndpointTypeTool, 4))
	assert.Equal(t, []uint{9}, list(database.EndpointTypeModelRouter, 5))
	assert.Equal(t, []uint{8}, list(database.EndpointTypePlugin, 20))

	require.NoError(t, svc.SyncConfiguration(snapshot(false)))
	var count int64
	require.NoError(t, db.Model(&database.EndpointAuthPlugin{}).Count(&count).Error)
	assert.Zero(t, count)
}

// A plugin on an endpoint's auth list is loaded on the edge even when no LLM
// has it, and the edge serves the list in order.
func TestEndpointAuthPlugins_LoadedAndListed(t *testing.T) {
	now := time.Now()
	db := setupEdgeSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	ds := &pb.DatasourceConfig{Id: 3, Name: "docs", IsActive: true, AuthPluginIds: []uint32{9, 8},
		CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now)}
	require.NoError(t, svc.SyncConfiguration(&pb.ConfigurationSnapshot{
		Version:      "1",
		SnapshotTime: timestamppb.Now(),
		Plugins:      []*pb.PluginConfig{authPluginTestPlugin(8, nil, now), authPluginTestPlugin(9, nil, now)},
		Datasources:  []*pb.DatasourceConfig{ds},
	}))

	pluginService := NewPluginService(db, database.NewRepository(db))
	adapter := NewPluginServiceAdapter(pluginService)

	active, err := adapter.GetAllActiveGatewayPlugins()
	require.NoError(t, err)
	var ids []uint
	for _, p := range active {
		ids = append(ids, p.ID)
	}
	assert.ElementsMatch(t, []uint{8, 9}, ids)

	list, err := adapter.GetAuthPluginsForEndpoint(database.EndpointTypeDatasource, 3)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.EqualValues(t, 9, list[0].ID)
	assert.EqualValues(t, 8, list[1].ID)

	none, err := adapter.GetAuthPluginsForEndpoint(database.EndpointTypeTool, 3)
	require.NoError(t, err)
	assert.Empty(t, none)
}

// LLMConfig.plugin_ids carries the order Studio runs an LLM's plugins in.
func TestEdgeSync_LLMPluginOrder(t *testing.T) {
	now := time.Now()
	db := setupEdgeSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	llm := authPluginTestLLM(1, "a", now)
	llm.PluginIds = []uint32{9, 8}
	err := svc.SyncConfiguration(&pb.ConfigurationSnapshot{
		Version:      "1",
		SnapshotTime: timestamppb.Now(),
		Llms:         []*pb.LLMConfig{llm},
		Plugins:      []*pb.PluginConfig{authPluginTestPlugin(8, []uint32{1}, now), authPluginTestPlugin(9, []uint32{1}, now)},
	})
	require.NoError(t, err)

	var rows []database.LLMPlugin
	require.NoError(t, db.Where("llm_id = ?", 1).Order("order_index ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 9, rows[0].PluginID)
	assert.EqualValues(t, 8, rows[1].PluginID)
}
