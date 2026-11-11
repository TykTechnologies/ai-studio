package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// A plugin changed on another replica: this replica's permission catalogue
// follows the database (without writing it), and a deletion drops the
// plugin's entries.
func TestApplyPluginChangeFromReplica_PermissionCatalogue(t *testing.T) {
	db := setupTestDB(t)
	svc := &Service{DB: db}

	plugin := &models.Plugin{
		Name: "Reports", Command: "file:///bin/true",
		HookType: models.HookTypeStudioUI, HookTypes: []string{models.HookTypeStudioUI},
		IsActive: true,
	}
	require.NoError(t, db.Create(plugin).Error)
	key := plugin.PermissionKey()
	t.Cleanup(func() { authz.UnregisterPlugin(key) })

	// Created elsewhere: this replica learns the permission resource.
	svc.ApplyPluginChangeFromReplica(TopicPluginCreated, plugin.ID)
	_, ok := authz.ResourceByKey(key)
	require.True(t, ok, "the plugin's resource is in this replica's catalogue")
	stored, _ := pluginPermissionKeys.Load(plugin.ID)
	assert.Equal(t, key, stored)

	var rows int64
	db.Model(&models.PluginPermissionResource{}).Where("plugin_id = ?", plugin.ID).Count(&rows)
	assert.Zero(t, rows, "nothing written: the replica that made the change wrote the database")

	// Updated elsewhere so it has no admin surface any more: dropped here.
	require.NoError(t, db.Model(plugin).Updates(map[string]interface{}{"hook_type": models.HookTypePostAuth, "hook_types": `["post_auth"]`}).Error)
	svc.ApplyPluginChangeFromReplica(TopicPluginUpdated, plugin.ID)
	_, ok = authz.ResourceByKey(key)
	assert.False(t, ok)

	// Back, then deleted elsewhere.
	require.NoError(t, db.Model(plugin).Updates(map[string]interface{}{"hook_type": models.HookTypeStudioUI, "hook_types": `["studio_ui"]`}).Error)
	svc.ApplyPluginChangeFromReplica(TopicPluginUpdated, plugin.ID)
	_, ok = authz.ResourceByKey(key)
	require.True(t, ok)
	require.NoError(t, db.Delete(plugin).Error)
	svc.ApplyPluginChangeFromReplica(TopicPluginDeleted, plugin.ID)
	_, ok = authz.ResourceByKey(key)
	assert.False(t, ok)
	_, ok = pluginPermissionKeys.Load(plugin.ID)
	assert.False(t, ok)
}

// Which plugins Studio starts when it starts (and when another replica
// creates or changes them).
func TestLoadsAtStart(t *testing.T) {
	cases := []struct {
		hooks []string
		want  bool
	}{
		{[]string{models.HookTypeStudioUI}, true},
		{[]string{models.HookTypeAgent}, true},
		{[]string{models.HookTypeObjectHooks}, true},
		{nil, true}, // not known yet: marketplace plugin before its manifest is read
		{[]string{models.HookTypePostAuth, models.HookTypeOnResponse}, false}, // gateway-only
		{[]string{models.HookTypePostAuth, models.HookTypeStudioUI}, true},
	}
	for _, tc := range cases {
		p := &models.Plugin{HookTypes: tc.hooks}
		if len(tc.hooks) > 0 {
			p.HookType = tc.hooks[0]
		}
		assert.Equal(t, tc.want, loadsAtStart(p), "%v", tc.hooks)
	}
}

// A replica that missed cluster events for longer than they are kept checks
// its plugins against the database: what changed or was deleted meanwhile is
// applied as its event would have been, and a plugin that did not change is
// left alone.
func TestReconcilePluginsAfterGap(t *testing.T) {
	db := setupTestDB(t)
	svc := &Service{DB: db}
	ui := func(name string) *models.Plugin {
		p := &models.Plugin{
			Name: name, Command: "file:///bin/true",
			HookType: models.HookTypeStudioUI, HookTypes: []string{models.HookTypeStudioUI},
			IsActive: true,
		}
		require.NoError(t, db.Create(p).Error)
		t.Cleanup(func() { authz.UnregisterPlugin(p.PermissionKey()) })
		return p
	}
	old, changed, deleted := ui("Old"), ui("Changed"), ui("Deleted")

	// This replica knew "Deleted" before the gap, and nothing of "Changed"
	// (created while it was away). "Old" was last written long before.
	svc.ApplyPluginChangeFromReplica(TopicPluginCreated, deleted.ID)
	_, ok := authz.ResourceByKey(deleted.PermissionKey())
	require.True(t, ok)
	long := time.Now().Add(-48 * time.Hour)
	require.NoError(t, db.Model(&models.Plugin{}).Where("id = ?", old.ID).UpdateColumn("updated_at", long).Error)
	require.NoError(t, db.Delete(deleted).Error)

	svc.ReconcilePluginsAfterGap(time.Now().Add(-time.Hour))

	_, ok = authz.ResourceByKey(changed.PermissionKey())
	assert.True(t, ok, "a plugin created while the replica was away is in its catalogue")
	_, ok = authz.ResourceByKey(deleted.PermissionKey())
	assert.False(t, ok, "a plugin deleted while it was away is dropped")
	_, ok = authz.ResourceByKey(old.PermissionKey())
	assert.False(t, ok, "a plugin that did not change is not touched")

	// Nothing to do the second time, and no database: no panic.
	svc.ReconcilePluginsAfterGap(time.Now().Add(-time.Hour))
	(&Service{}).ReconcilePluginsAfterGap(time.Now())
}
