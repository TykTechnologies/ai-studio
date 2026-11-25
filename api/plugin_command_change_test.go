package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Changing a plugin's command points it at another binary. The approval of
// its service scopes was given for the old one, and Studio kept running the
// old process: a PATCH with a new command left service_access_authorized
// true and reloaded nothing (midsommar-clean-6bg5.97). Now any change of
// command withdraws the approval and stops the old process; the new binary
// is loaded and its scopes wait for approval.

func storedPlugin(t *testing.T, db *gorm.DB, id uint) models.Plugin {
	t.Helper()
	var p models.Plugin
	require.NoError(t, db.First(&p, id).Error)
	return p
}

// allowedPluginDir is a directory plugin commands may name in these tests.
// Nothing is in it, so no command there can be loaded.
func allowedPluginDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AI_STUDIO_PLUGIN_ALLOWED_DIRS", dir)
	return "file://" + dir
}

func TestPluginAPI_CommandChangeWithdrawsApprovalAndStopsTheOldProcess(t *testing.T) {
	pluginDir := allowedPluginDir(t)
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"command only", map[string]any{"command": pluginDir + "/limiter-v2"}},
		{"command and config in one request", map[string]any{"command": pluginDir + "/limiter-v2",
			"config": map[string]any{"window": "10s"}}},
		{"command with load_immediately", map[string]any{"command": pluginDir + "/limiter-v2", "load_immediately": true}},
		{"command as the form sends it, with every other field", map[string]any{"command": pluginDir + "/limiter-v2",
			"name": "limiter", "description": "", "hook_type": models.HookTypeStudioUI, "is_active": true,
			"namespace": "", "oci_reference": "", "allow_internal_host": false, "config": map[string]any{"window": "1m"}}},
		{"the OCI reference", map[string]any{"oci_reference": "registry.example.com/plugins/limiter:2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, db := setupTestAPI(t)
			router := internalHostRouter(a, true)
			mgr := services.NewAIStudioPluginManager(db, nil)
			a.service.AIStudioPluginManager = mgr

			plugin := &models.Plugin{Name: "limiter", Command: pluginDir + "/limiter-v1",
				HookType: models.HookTypeStudioUI, HookTypes: []string{models.HookTypeStudioUI}, IsActive: true,
				Config:        map[string]interface{}{"window": "1m"},
				ServiceScopes: []string{"kv.readwrite"}, ServiceAccessAuthorized: true}
			require.NoError(t, db.Create(plugin).Error)
			running := &shutdownCountingClient{}
			mgr.InjectTestPlugin(plugin.ID, plugin.Name, running)
			t.Cleanup(func() { mgr.RemoveTestPlugin(plugin.ID) })

			w := performRequest(router, "PATCH", fmt.Sprintf("/plugins/%d", plugin.ID), tc.body)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			got := storedPlugin(t, db, plugin.ID)
			assert.False(t, got.ServiceAccessAuthorized, "the approval given for the old binary is kept")
			assert.Equal(t, int32(1), running.shutdowns.Load(), "the process of the old binary keeps serving")
			// The new command does not exist here, so nothing could be
			// loaded: the plugin is left not running, never on the old binary.
			assert.False(t, mgr.IsPluginLoaded(plugin.ID))
		})
	}
}

func TestPluginAPI_UpdateWithoutACommandChangeKeepsApprovalAndTheProcess(t *testing.T) {
	pluginDir := allowedPluginDir(t)
	a, db := setupTestAPI(t)
	router := internalHostRouter(a, true)
	mgr := services.NewAIStudioPluginManager(db, nil)
	a.service.AIStudioPluginManager = mgr

	plugin := &models.Plugin{Name: "limiter", Command: pluginDir + "/limiter-v1",
		HookType: models.HookTypeStudioUI, HookTypes: []string{models.HookTypeStudioUI}, IsActive: true,
		Config:        map[string]interface{}{"window": "1m"},
		ServiceScopes: []string{"kv.readwrite"}, ServiceAccessAuthorized: true}
	require.NoError(t, db.Create(plugin).Error)
	running := &shutdownCountingClient{}
	mgr.InjectTestPlugin(plugin.ID, plugin.Name, running)
	t.Cleanup(func() { mgr.RemoveTestPlugin(plugin.ID) })
	path := fmt.Sprintf("/plugins/%d", plugin.ID)

	for name, body := range map[string]map[string]any{
		"name only": {"name": "limiter-renamed"},
		// The edit form always sends the command it loaded.
		"the same command sent back": {"command": pluginDir + "/limiter-v1", "oci_reference": "",
			"name": "limiter", "config": map[string]any{"window": "1m"}},
	} {
		w := performRequest(router, "PATCH", path, body)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", name, w.Body.String())
		got := storedPlugin(t, db, plugin.ID)
		assert.True(t, got.ServiceAccessAuthorized, "%s: approval withdrawn without a command change", name)
		assert.Equal(t, []string{"kv.readwrite"}, []string(got.ServiceScopes), name)
		assert.Equal(t, int32(0), running.shutdowns.Load(), "%s: the plugin was restarted", name)
		assert.True(t, mgr.IsPluginLoaded(plugin.ID), name)
	}
}

// A gateway-only plugin is not run by Studio; its approval is withdrawn all
// the same, and nothing is loaded.
func TestPluginAPI_CommandChangeOnAGatewayOnlyPlugin(t *testing.T) {
	pluginDir := allowedPluginDir(t)
	a, db := setupTestAPI(t)
	router := internalHostRouter(a, true)
	mgr := services.NewAIStudioPluginManager(db, nil)
	a.service.AIStudioPluginManager = mgr

	plugin := &models.Plugin{Name: "edge-limiter", Command: pluginDir + "/edge-v1",
		HookType: models.HookTypePostAuth, HookTypes: []string{models.HookTypePostAuth, models.HookTypeOnResponse}, IsActive: true,
		ServiceScopes: []string{"kv.readwrite"}, ServiceAccessAuthorized: true}
	require.NoError(t, db.Create(plugin).Error)

	w := performRequest(router, "PATCH", fmt.Sprintf("/plugins/%d", plugin.ID), map[string]any{"command": pluginDir + "/edge-v2"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got := storedPlugin(t, db, plugin.ID)
	assert.Equal(t, pluginDir+"/edge-v2", got.Command)
	assert.False(t, got.ServiceAccessAuthorized)
	assert.False(t, mgr.IsPluginLoaded(plugin.ID))
}
