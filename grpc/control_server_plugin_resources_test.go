package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestControlServer_SnapshotSkipsNonAppGrantedPluginResources: only plugin
// resource types an App credential grants access to reach the gateways. A
// binding to a non-app-granted type stays on the App but is not shipped, and
// the association shape for shipped types is unchanged.
func TestControlServer_SnapshotSkipsNonAppGrantedPluginResources(t *testing.T) {
	server, db := setupTestServer(t, nil)
	namespace := "prs-ns"
	llms := createTestLLMs(db, namespace)
	apps := createTestApps(db, namespace, llms)
	app := apps[0]

	plugin := &models.Plugin{Name: "p", Command: "/usr/bin/p", HookType: models.HookTypeResourceProvider, IsActive: true}
	require.NoError(t, db.Create(plugin).Error)
	granted := &models.PluginResourceType{PluginID: plugin.ID, Slug: "mcp_servers", Name: "MCP Servers", AccessGrantedViaApp: true, IsActive: true}
	informational := &models.PluginResourceType{PluginID: plugin.ID, Slug: "agent", Name: "Agent", AccessGrantedViaApp: false, IsActive: true}
	require.NoError(t, db.Create(granted).Error)
	require.NoError(t, db.Create(informational).Error)

	require.NoError(t, db.Create(&models.AppPluginResource{AppID: app.ID, PluginResourceTypeID: granted.ID, InstanceID: "srv-1", InstanceName: "Server One", InstancePrivacyScore: 20, InstanceMetadata: []byte(`{"k":"v"}`)}).Error)
	require.NoError(t, db.Create(&models.AppPluginResource{AppID: app.ID, PluginResourceTypeID: informational.ID, InstanceID: "ast_1", InstanceName: "Agent One"}).Error)

	snapshot, err := server.getConfigurationSnapshot(namespace)
	require.NoError(t, err)

	var found bool
	for _, a := range snapshot.Apps {
		if a.Id != uint32(app.ID) {
			continue
		}
		found = true
		require.Len(t, a.PluginResources, 1, "only the app-granted type is shipped")
		pra := a.PluginResources[0]
		assert.Equal(t, "mcp_servers", pra.ResourceTypeSlug)
		assert.Equal(t, uint32(plugin.ID), pra.PluginId)
		assert.Equal(t, []string{"srv-1"}, pra.InstanceIds)
		require.Len(t, pra.Instances, 1)
		assert.Equal(t, "Server One", pra.Instances[0].Name)
		assert.Equal(t, int32(20), pra.Instances[0].PrivacyScore)
		assert.Equal(t, []byte(`{"k":"v"}`), pra.Instances[0].Metadata)
	}
	assert.True(t, found, "app present in snapshot")

	// The binding itself is untouched.
	var count int64
	require.NoError(t, db.Model(&models.AppPluginResource{}).Where("app_id = ?", app.ID).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}
