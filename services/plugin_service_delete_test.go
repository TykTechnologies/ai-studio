package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleting a plugin must take its KV namespace with it. A reinstall gets a
// new id, so rows left under the old id are unreachable through the KV
// service (ClearAllPluginData refuses an id with no plugin row) and would
// otherwise leak forever.
func TestDeletePlugin_RemovesKVData(t *testing.T) {
	kvService, db, pluginID := setupPluginKVTest(t)

	created, err := kvService.WriteKV(pluginID, "settings", []byte(`{"enabled":true}`), nil)
	require.NoError(t, err)
	require.True(t, created)
	created, err = kvService.WriteKV(pluginID, "counter", []byte("42"), nil)
	require.NoError(t, err)
	require.True(t, created)

	before, err := models.CountPluginDataByPluginID(db, pluginID)
	require.NoError(t, err)
	require.Equal(t, int64(2), before)

	require.NoError(t, NewPluginService(db).DeletePlugin(pluginID))

	after, err := models.CountPluginDataByPluginID(db, pluginID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), after, "KV rows must not outlive their plugin")

	// Not just soft-deleted: tombstones under the old id serve nothing and
	// still accumulate across delete + reinstall cycles.
	var remaining int64
	require.NoError(t, db.Unscoped().Model(&models.PluginData{}).Where("plugin_id = ?", pluginID).Count(&remaining).Error)
	assert.Equal(t, int64(0), remaining, "plugin_data rows are removed outright, not tombstoned")
}

// Deleting a plugin must retire its resource types even when the plugin is
// not loaded (only unload used to deactivate them). An active type whose
// plugin is gone broke the App form (500s) and duplicated portal Browse tabs.
func TestDeletePlugin_DeactivatesResourceTypes(t *testing.T) {
	_, db, pluginID := setupPluginKVTest(t)

	prt := &models.PluginResourceType{PluginID: pluginID, Slug: "mcp_servers", Name: "MCP Servers", IsActive: true}
	require.NoError(t, db.Create(prt).Error)

	require.NoError(t, NewPluginService(db).DeletePlugin(pluginID))

	var active models.PluginResourceTypes
	require.NoError(t, active.GetAllActive(db))
	assert.Empty(t, active, "a deleted plugin's resource types must not stay active")
}

// Databases already hold active types of plugins deleted before the fix;
// the startup sweep retires them.
func TestDeactivateOrphanedPluginResourceTypes(t *testing.T) {
	_, db, livePluginID := setupPluginKVTest(t)

	gone := &models.Plugin{Name: "gone", Command: "/usr/bin/gone", HookType: "pre_request", IsActive: true}
	require.NoError(t, db.Create(gone).Error)
	require.NoError(t, db.Create(&models.PluginResourceType{PluginID: livePluginID, Slug: "live", Name: "Live", IsActive: true}).Error)
	require.NoError(t, db.Create(&models.PluginResourceType{PluginID: gone.ID, Slug: "orphan", Name: "Orphan", IsActive: true}).Error)
	require.NoError(t, db.Delete(gone).Error) // soft delete, as DeletePlugin did before

	n, err := models.DeactivateOrphanedPluginResourceTypes(db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	var active models.PluginResourceTypes
	require.NoError(t, active.GetAllActive(db))
	require.Len(t, active, 1)
	assert.Equal(t, "live", active[0].Slug)
}
