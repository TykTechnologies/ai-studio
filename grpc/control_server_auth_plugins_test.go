package grpc

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plugin attached to several LLMs goes out once, listing them all; each
// LLM carries its plugins in execution order; the auth plugin lists of other
// endpoints go out on them, without plugins the edge does not receive.
func TestControlServer_SnapshotPluginAttachments(t *testing.T) {
	server, db := setupTestServer(t, nil)
	namespace := "auth-ns"
	llms := createTestLLMs(db, namespace)

	idp := &models.Plugin{Name: "idp", Command: "/bin/idp", HookType: models.HookTypeAuth, IsActive: true,
		Config: map[string]interface{}{"base": true}}
	logger := &models.Plugin{Name: "logger", Command: "/bin/logger", HookType: models.HookTypePostAuth, IsActive: true}
	inactive := &models.Plugin{Name: "old-idp", Command: "/bin/old", HookType: models.HookTypeAuth, IsActive: true}
	endpoint := &models.Plugin{Name: "endpoint", Command: "/bin/ep", HookType: models.HookTypeCustomEndpoint, IsActive: true}
	for _, p := range []*models.Plugin{idp, logger, inactive, endpoint} {
		require.NoError(t, db.Create(p).Error)
	}
	// Created active (the bool default), then switched off.
	require.NoError(t, db.Model(inactive).Update("is_active", false).Error)

	// LLM 0 runs logger then idp; LLM 1 runs idp only, with an override.
	require.NoError(t, models.UpdatePluginOrder(db, llms[0].ID, []uint{logger.ID, idp.ID}))
	require.NoError(t, models.UpdatePluginOrder(db, llms[1].ID, []uint{idp.ID}))
	setOverride(t, db, llms[1].ID, idp.ID)

	ds := &models.Datasource{Name: "Docs", Active: true, Namespace: namespace}
	require.NoError(t, db.Create(ds).Error)
	require.NoError(t, models.SetEndpointAuthPlugins(db, models.EndpointTypeDatasource, ds.ID, []uint{inactive.ID, idp.ID}))
	require.NoError(t, models.SetEndpointAuthPlugins(db, models.EndpointTypePlugin, endpoint.ID, []uint{idp.ID}))

	snapshot, err := server.getConfigurationSnapshot(namespace)
	require.NoError(t, err)

	byID := map[uint32][]*pb.PluginConfig{}
	for _, p := range snapshot.Plugins {
		byID[p.Id] = append(byID[p.Id], p)
	}
	require.Len(t, byID[uint32(idp.ID)], 1, "one row per plugin")
	idpRow := byID[uint32(idp.ID)][0]
	assert.ElementsMatch(t, []uint32{uint32(llms[0].ID), uint32(llms[1].ID)}, idpRow.LlmIds)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(idpRow.Config), &cfg))
	assert.Equal(t, map[string]interface{}{"base": true}, cfg, "on several LLMs the base config goes out")
	assert.Empty(t, byID[uint32(inactive.ID)], "an inactive plugin is not sent")
	require.Len(t, byID[uint32(endpoint.ID)], 1)
	assert.Equal(t, []uint32{uint32(idp.ID)}, byID[uint32(endpoint.ID)][0].AuthPluginIds)

	for _, l := range snapshot.Llms {
		switch l.Id {
		case uint32(llms[0].ID):
			assert.Equal(t, []uint32{uint32(logger.ID), uint32(idp.ID)}, l.PluginIds)
		case uint32(llms[1].ID):
			assert.Equal(t, []uint32{uint32(idp.ID)}, l.PluginIds)
		}
	}

	var dsRow *pb.DatasourceConfig
	for _, d := range snapshot.Datasources {
		if d.Id == uint32(ds.ID) {
			dsRow = d
		}
	}
	require.NotNil(t, dsRow)
	assert.Equal(t, []uint32{uint32(idp.ID)}, dsRow.AuthPluginIds, "the inactive plugin drops off the list")
}

func setOverride(t *testing.T, db *gorm.DB, llmID, pluginID uint) {
	t.Helper()
	var lp models.LLMPlugin
	require.NoError(t, lp.Get(db, llmID, pluginID))
	require.NoError(t, lp.UpdateConfig(db, map[string]interface{}{"override": true}))
}

// A plugin on one LLM keeps that LLM's config override, as before.
func TestControlServer_SnapshotSingleLLMOverride(t *testing.T) {
	server, db := setupTestServer(t, nil)
	namespace := "auth-ns-single"
	llms := createTestLLMs(db, namespace)
	idp := &models.Plugin{Name: "idp", Command: "/bin/idp", HookType: models.HookTypeAuth, IsActive: true,
		Config: map[string]interface{}{"base": true}}
	require.NoError(t, db.Create(idp).Error)
	require.NoError(t, models.UpdatePluginOrder(db, llms[0].ID, []uint{idp.ID}))
	setOverride(t, db, llms[0].ID, idp.ID)

	snapshot, err := server.getConfigurationSnapshot(namespace)
	require.NoError(t, err)
	var row *pb.PluginConfig
	for _, p := range snapshot.Plugins {
		if p.Id == uint32(idp.ID) {
			row = p
		}
	}
	require.NotNil(t, row)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(row.Config), &cfg))
	assert.Equal(t, true, cfg["override"])
	assert.Equal(t, true, cfg["base"])
}
