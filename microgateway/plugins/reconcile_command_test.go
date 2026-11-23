package plugins

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/pkg/pluginenv"
)

// The reconcile diff: a plugin whose command changed is reloaded. It was
// compared by checksum, config and allow_internal_host only, so an edge kept
// running the old binary after a plugin was pointed at a new file or OCI
// digest in AI Studio, until it was restarted (2.3.0-rc4 finding M4).
func TestDiffPluginStates(t *testing.T) {
	const (
		ociV1 = "oci://registry.example.com/plugins/llm-load-balancer@sha256:1111111111111111111111111111111111111111111111111111111111111111?arch=linux/amd64"
		ociV2 = "oci://registry.example.com/plugins/llm-load-balancer@sha256:2222222222222222222222222222222222222222222222222222222222222222?arch=linux/amd64"
	)
	cfg := []byte(`{"a":1}`)

	cases := []struct {
		name       string
		loaded     loadedPluginState
		desired    *PluginData // nil: no longer on this gateway
		wantUnload bool
		wantChange []string // nil: no reload
	}{
		{
			name:    "nothing changed",
			loaded:  loadedPluginState{Command: ociV1, Checksum: "c", RawConfig: cfg, AllowInternalHost: true},
			desired: &PluginData{Command: ociV1, Checksum: "c", Config: cfg, AllowInternalHost: true},
		},
		{
			name:       "new file path",
			loaded:     loadedPluginState{Command: ociV1},
			desired:    &PluginData{Command: "file:///opt/microgateway/plugins/llm-load-balancer"},
			wantChange: []string{"command"},
		},
		{
			name:       "new OCI digest (a marketplace upgrade)",
			loaded:     loadedPluginState{Command: ociV1},
			desired:    &PluginData{Command: ociV2},
			wantChange: []string{"command"},
		},
		{
			name:       "new OCI query parameter (the key that must have signed it)",
			loaded:     loadedPluginState{Command: ociV1},
			desired:    &PluginData{Command: ociV1 + "&pubkey=CI"},
			wantChange: []string{"command"},
		},
		{
			name:       "new grpc address",
			loaded:     loadedPluginState{Command: "grpc://plugins.example.com:9000"},
			desired:    &PluginData{Command: "grpc://plugins.example.com:9001"},
			wantChange: []string{"command"},
		},
		{
			name:       "the same file named another way is a change: the command is compared as written",
			loaded:     loadedPluginState{Command: "/opt/plugins/fw"},
			desired:    &PluginData{Command: "file:///opt/plugins/fw"},
			wantChange: []string{"command"},
		},
		{
			name:       "command and config together",
			loaded:     loadedPluginState{Command: ociV1, RawConfig: cfg},
			desired:    &PluginData{Command: ociV2, Config: []byte(`{"a":2}`)},
			wantChange: []string{"command", "config"},
		},
		{
			name:       "checksum",
			loaded:     loadedPluginState{Command: ociV1, Checksum: "old"},
			desired:    &PluginData{Command: ociV1, Checksum: "new"},
			wantChange: []string{"checksum"},
		},
		{
			name:       "allow internal host switched off",
			loaded:     loadedPluginState{Command: ociV1, AllowInternalHost: true},
			desired:    &PluginData{Command: ociV1},
			wantChange: []string{"allow_internal_host"},
		},
		{
			name:       "removed from the gateway",
			loaded:     loadedPluginState{Command: ociV1},
			wantUnload: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desired := map[uint]PluginData{}
			if tc.desired != nil {
				desired[7] = *tc.desired
			}
			toUnload, toReload := diffPluginStates(map[uint]loadedPluginState{7: tc.loaded}, desired)

			if tc.wantUnload {
				assert.Equal(t, []uint{7}, toUnload)
			} else {
				assert.Empty(t, toUnload)
			}
			if tc.wantChange == nil {
				assert.Empty(t, toReload, "no reload")
				return
			}
			require.Len(t, toReload, 1)
			assert.Equal(t, uint(7), toReload[0].ID)
			assert.Equal(t, tc.wantChange, toReload[0].Changed)
		})
	}

	t.Run("a desired plugin that is not loaded is left to the pre-warm", func(t *testing.T) {
		toUnload, toReload := diffPluginStates(map[uint]loadedPluginState{}, map[uint]PluginData{7: {Command: ociV1}})
		assert.Empty(t, toUnload)
		assert.Empty(t, toReload)
	})
}

// copyOfTestBinary puts the test binary (which serves a helper plugin) at a
// second path: version B of the plugin.
func copyOfTestBinary(t *testing.T, self string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "plugin-b")
	if os.Link(self, dst) == nil {
		return dst
	}
	in, err := os.Open(self)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o755)
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())
	return dst
}

// reloadFixture is a manager with one running plugin, loaded from the test
// binary, attached to LLM 1.
func reloadFixture(t *testing.T) (pm *PluginManager, svc *mockPluginService, self string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(msgSizeHelperEnv, "gatewayplugin_sdk")
	t.Setenv("PLUGIN_RUNTIME", "gateway")
	t.Setenv(pluginenv.EnvPassthrough, msgSizeHelperEnv+",PLUGIN_RUNTIME")
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")

	svc = &mockPluginService{}
	setPluginCommand(svc, self)
	pm = NewPluginManager(svc)
	t.Cleanup(func() { _ = pm.UnloadPlugin(1) })
	require.NoError(t, pm.ReconcilePlugins(context.Background()))
	require.True(t, pm.IsPluginLoaded(1))
	return pm, svc, self
}

// setPluginCommand is what a configuration push that changed the plugin's
// command leaves in the gateway's database.
func setPluginCommand(svc *mockPluginService, command string) {
	p := PluginData{ID: 1, Name: "llm-load-balancer", HookType: "post_auth", Command: command, IsActive: true}
	svc.allActiveGwPlugins = []PluginData{p}
	svc.getPluginByID = map[uint]PluginData{1: p}
	svc.pluginsByLLM = map[uint][]PluginData{1: {p}}
}

func loadedPlugin(pm *PluginManager, id uint) *LoadedPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.loadedPlugins[id]
}

// A running plugin whose command changes is replaced on the next reconcile,
// with no restart of the gateway: the old process exits and requests get the
// new one. A push that changes nothing reloads nothing.
func TestReconcile_ChangedCommandReplacesTheRunningPlugin(t *testing.T) {
	logs := captureLogs(t)
	pm, svc, self := reloadFixture(t)
	versionA := loadedPlugin(pm, 1)
	require.Equal(t, self, versionA.Command)

	// A push with the same command: the plugin keeps running.
	require.NoError(t, pm.ReconcilePlugins(context.Background()))
	assert.Same(t, versionA, loadedPlugin(pm, 1), "an unchanged command is not a reload")
	assert.False(t, versionA.Client.Exited())
	assert.NotContains(t, logs.String(), "Reloading changed plugin")

	// A push with a new command.
	versionBPath := copyOfTestBinary(t, self)
	setPluginCommand(svc, versionBPath)
	require.NoError(t, pm.ReconcilePlugins(context.Background()))

	versionB := loadedPlugin(pm, 1)
	require.NotNil(t, versionB, "the plugin is loaded again")
	assert.NotSame(t, versionA, versionB)
	assert.Equal(t, versionBPath, versionB.Command)
	assert.True(t, versionA.Client.Exited(), "the old process is gone")
	assert.False(t, versionB.Client.Exited())
	assert.Equal(t, versionBPath, pm.GetPluginHealth()[1].Command, "plugin health names what runs now")

	list, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Same(t, versionB, list[0], "requests get the new process")

	// The log says what changed, not the command (it may hold credentials).
	assert.Contains(t, logs.String(), `"changed":["command"]`)
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "Reloading changed plugin") {
			assert.NotContains(t, line, versionBPath)
			assert.NotContains(t, line, self)
		}
	}
}

// When the new command cannot be loaded, the old process does not keep
// serving as if nothing had changed: it is stopped, the plugin is failed in
// plugin health, and requests for its LLM get an error, not a list without
// the plugin.
func TestReconcile_ChangedCommandThatFailsToLoadStopsTheOldPlugin(t *testing.T) {
	pm, svc, _ := reloadFixture(t)
	versionA := loadedPlugin(pm, 1)

	setPluginCommand(svc, "/nonexistent/llm-load-balancer-1.2.1")
	require.NoError(t, pm.ReconcilePlugins(context.Background()))

	assert.False(t, pm.IsPluginLoaded(1))
	assert.True(t, versionA.Client.Exited(), "the old process is stopped")
	health := pm.GetPluginHealth()[1]
	require.NotNil(t, health)
	assert.Equal(t, PluginStatusFailed, health.Status)
	assert.Equal(t, "/nonexistent/llm-load-balancer-1.2.1", health.Command)

	list, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	require.Error(t, err, "the LLM's plugin list is an error, not an empty list")
	assert.Empty(t, list)
}
