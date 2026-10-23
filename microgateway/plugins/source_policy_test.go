package plugins

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
	"github.com/TykTechnologies/midsommar/v2/pkg/pluginenv"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

func mustSourcePolicy(t *testing.T, csv string) plugin_security.SourcePolicy {
	t.Helper()
	policy, err := plugin_security.ParseSourcePolicy(csv)
	require.NoError(t, err)
	return policy.WithSetting("PLUGINS_ALLOWED_SOURCES")
}

func requireSourceNotAllowed(t *testing.T, err error) {
	t.Helper()
	var notAllowed *plugin_security.ErrPluginSourceNotAllowed
	require.True(t, errors.As(err, &notAllowed), "want ErrPluginSourceNotAllowed, got %v", err)
}

// syncBuffer is a bytes.Buffer safe for the plugin goroutines that keep
// logging while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs sends the global logger to a buffer for the rest of the test.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	logs := &syncBuffer{}
	saved := log.Logger
	log.Logger = zerolog.New(logs)
	t.Cleanup(func() { log.Logger = saved })
	return logs
}

// acceptingVerifier is a security service that accepts every signature.
type acceptingVerifier struct{}

func (acceptingVerifier) VerifySignature(context.Context, *ociplugins.OCIReference, string) error {
	return nil
}
func (acceptingVerifier) VerifyBundle(context.Context, *ociplugins.OCIReference, string, string) error {
	return nil
}
func (acceptingVerifier) VerifyWithPolicy(context.Context, *ociplugins.OCIReference, string) error {
	return nil
}

// The policy is checked before any exec, reattach or OCI fetch.
func TestCreatePluginClient_RefusesDisallowedSource(t *testing.T) {
	pm := NewPluginManager(&mockPluginService{})
	pm.SetSourcePolicy(mustSourcePolicy(t, "oci"))

	for _, command := range []string{"/bin/true", "file:///bin/true", "grpc://127.0.0.1:1"} {
		client, err := pm.createPluginClient(command)
		requireSourceNotAllowed(t, err)
		assert.Nil(t, client, command)
	}
}

// With oci disallowed, pre-fetch and pre-warm make no registry request.
func TestOCIPreFetchAndPreWarm_OCIDisallowed(t *testing.T) {
	reg := ocitest.NewRegistry(t)
	manifest := reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	command := "oci://" + reg.Host() + "/plugins/demo@" + manifest.Digest.String() + "?arch=linux/amd64"

	cfg := ociplugins.DefaultOCIConfig()
	cfg.CacheDir = t.TempDir()
	cfg.GCInterval = 0
	svc := &mockPluginService{plugins: []PluginData{{ID: 1, Name: "demo", HookType: "post_auth", Command: command, IsActive: true}}}
	pm, err := NewPluginManagerWithOCI(svc, cfg, acceptingVerifier{})
	require.NoError(t, err)
	pm.SetSourcePolicy(mustSourcePolicy(t, "file,grpc"))

	requireSourceNotAllowed(t, pm.PreFetchOCIPlugin(command))
	require.NoError(t, pm.PreWarmOCIPlugins(context.Background()))
	assert.Empty(t, pm.GetPluginHealth(), "pre-warming skipped every plugin")

	for _, method := range []string{"GET", "HEAD"} {
		for _, kind := range []string{"manifests", "blobs", "referrers"} {
			assert.Zero(t, reg.Requests(method, kind), "%s %s", method, kind)
		}
	}

	// Allowed again, the same pre-fetch reaches the registry.
	pm.SetSourcePolicy(plugin_security.AllowAllSources())
	require.NoError(t, pm.PreFetchOCIPlugin(command))
	assert.NotZero(t, reg.Requests("GET", "manifests"))
}

// A config the hub pushes with a plugin of a disallowed source leaves that
// plugin unloaded (logged with its ID, failed in plugin health) while the
// allowed plugins in the same config load.
func TestReconcile_HubPushedDisallowedSourceNotLoaded(t *testing.T) {
	self, err := os.Executable()
	require.NoError(t, err)
	// The allowed plugin is this test binary serving a gateway plugin.
	t.Setenv(msgSizeHelperEnv, "gatewayplugin_sdk")
	t.Setenv("PLUGIN_RUNTIME", "gateway")
	t.Setenv(pluginenv.EnvPassthrough, msgSizeHelperEnv+",PLUGIN_RUNTIME")
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")
	logs := captureLogs(t)

	allowed := PluginData{ID: 1, Name: "allowed", HookType: "post_auth", Command: self, IsActive: true}
	refused := PluginData{ID: 2, Name: "refused", HookType: "post_auth", Command: "grpc://127.0.0.1:1", IsActive: true}
	svc := &mockPluginService{
		allActiveGwPlugins: []PluginData{allowed, refused},
		getPluginByID:      map[uint]PluginData{1: allowed, 2: refused},
	}
	pm := NewPluginManager(svc)
	pm.SetSourcePolicy(mustSourcePolicy(t, "file"))
	t.Cleanup(func() { _ = pm.UnloadPlugin(1) })

	require.NoError(t, pm.ReconcilePlugins(context.Background()))

	assert.True(t, pm.IsPluginLoaded(1), "the allowed plugin loads")
	assert.False(t, pm.IsPluginLoaded(2), "the disallowed plugin does not")
	health := pm.GetPluginHealth()[2]
	require.NotNil(t, health)
	assert.Equal(t, PluginStatusFailed, health.Status)
	assert.True(t, strings.Contains(logs.String(), `"plugin_id":2`) && strings.Contains(logs.String(), "PLUGINS_ALLOWED_SOURCES"),
		"the refusal is logged with the plugin ID:\n%s", logs.String())
}

// Global data-collection plugins (PLUGINS_CONFIG_PATH) get the same check.
func TestLoadGlobalDataCollectionPlugins_RefusesDisallowedSource(t *testing.T) {
	logs := captureLogs(t)
	pm := NewPluginManager(&mockPluginService{})
	pm.SetSourcePolicy(mustSourcePolicy(t, "oci"))

	require.NoError(t, pm.LoadGlobalDataCollectionPlugins([]DataCollectionPluginConfig{
		{Name: "collector", Path: "/bin/true", Enabled: true, HookTypes: []string{"analytics"}},
	}))
	assert.Empty(t, pm.globalDataPlugins)
	assert.Contains(t, logs.String(), "PLUGINS_ALLOWED_SOURCES")
}
