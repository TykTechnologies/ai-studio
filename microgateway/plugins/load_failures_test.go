package plugins

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
	"github.com/TykTechnologies/midsommar/v2/pkg/pluginenv"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

// loadCountingService counts the load attempts the manager makes: each one
// starts by reading the plugin.
type loadCountingService struct {
	*mockPluginService
	loads atomic.Int32
}

func (s *loadCountingService) GetPlugin(id uint) (PluginData, error) {
	s.loads.Add(1)
	return s.mockPluginService.GetPlugin(id)
}

func requirePluginLoadError(t *testing.T, err error, pluginID uint) *PluginLoadError {
	t.Helper()
	var loadErr *PluginLoadError
	require.True(t, errors.As(err, &loadErr), "want a *PluginLoadError, got %v", err)
	assert.Equal(t, pluginID, loadErr.PluginID)
	return loadErr
}

// One plugin of an LLM that cannot be loaded is an error for the LLM's whole
// list, and the failure is remembered: later requests are refused without a
// new load attempt each, and the cause is logged once, not per request.
func TestGetPluginsForLLM_LoadFailureIsRememberedAndLoggedOnce(t *testing.T) {
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")
	logs := captureLogs(t)

	broken := PluginData{ID: 7, Name: "broken-firewall", HookType: "post_auth", Command: "/nonexistent/plugin-binary", IsActive: true}
	svc := &loadCountingService{mockPluginService: &mockPluginService{
		pluginsByLLM:  map[uint][]PluginData{1: {broken}},
		getPluginByID: map[uint]PluginData{7: broken},
	}}
	pm := NewPluginManager(svc)
	pm.loadRetryInterval = time.Hour

	_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	first := requirePluginLoadError(t, err, 7)
	assert.False(t, first.Remembered, "the first request tried the load")
	assert.Equal(t, "broken-firewall", first.PluginName)
	require.EqualValues(t, 1, svc.loads.Load())

	for i := 0; i < 25; i++ {
		_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
		assert.True(t, requirePluginLoadError(t, err, 7).Remembered)
	}
	// The chain is refused the same way: it never runs the plugins that did load.
	_, err = pm.ExecutePluginChain(1, interfaces.HookTypePostAuth, &interfaces.EnrichedRequest{}, &interfaces.PluginContext{})
	requirePluginLoadError(t, err, 7)

	assert.EqualValues(t, 1, svc.loads.Load(), "no load attempt per refused request")
	assert.Equal(t, 1, strings.Count(logs.String(), "Plugin could not be loaded"),
		"the failure is logged once:\n%s", logs.String())
	assert.Contains(t, logs.String(), `"plugin_id":7`)
	assert.Contains(t, logs.String(), `"plugin_name":"broken-firewall"`)

	// A plugin of another hook type on the same LLM does not hide it, and an
	// LLM without the broken plugin is unaffected.
	list, err := pm.GetPluginsForLLM(2, interfaces.HookTypePostAuth)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// After the retry interval the next request starts a new attempt in the
// background and is itself refused at once; the same cause is not logged a
// second time.
func TestGetPluginsForLLM_RetriesAfterTheInterval(t *testing.T) {
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")
	logs := captureLogs(t)

	broken := PluginData{ID: 7, Name: "broken", HookType: "pre_auth", Command: "/nonexistent/plugin-binary", IsActive: true}
	svc := &loadCountingService{mockPluginService: &mockPluginService{
		pluginsByLLM:  map[uint][]PluginData{1: {broken}},
		getPluginByID: map[uint]PluginData{7: broken},
	}}
	pm := NewPluginManager(svc)
	pm.loadRetryInterval = 20 * time.Millisecond

	_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePreAuth)
	requirePluginLoadError(t, err, 7)
	time.Sleep(40 * time.Millisecond)
	_, err = pm.GetPluginsForLLM(1, interfaces.HookTypePreAuth)
	assert.True(t, requirePluginLoadError(t, err, 7).Remembered, "refused with the last failure, not held for the retry")

	require.Eventually(t, func() bool { return svc.loads.Load() >= 2 }, 2*time.Second, 5*time.Millisecond,
		"the interval has passed: a new attempt")
	// Let the attempt finish before the logs are read.
	require.Eventually(t, func() bool {
		_, fresh := pm.lastLoadFailure(7)
		return fresh
	}, 2*time.Second, 5*time.Millisecond)
	pm.loadRetryInterval = time.Hour
	assert.Equal(t, 1, strings.Count(logs.String(), "Plugin could not be loaded"), "same cause, logged once:\n%s", logs.String())
}

// Concurrent requests for a plugin that cannot be loaded share one attempt.
func TestGetPluginsForLLM_ConcurrentRequestsShareOneLoadAttempt(t *testing.T) {
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")

	broken := PluginData{ID: 7, Name: "broken", HookType: "post_auth", Command: "/nonexistent/plugin-binary", IsActive: true}
	svc := &loadCountingService{mockPluginService: &mockPluginService{
		pluginsByLLM:  map[uint][]PluginData{1: {broken}},
		getPluginByID: map[uint]PluginData{7: broken},
	}}
	pm := NewPluginManager(svc)
	pm.loadRetryInterval = time.Hour

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
			assert.Error(t, err)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, svc.loads.Load())
}

// An OCI plugin that keeps failing to load costs the registry one fetch
// attempt, not one per client request.
func TestGetPluginsForLLM_FailingOCIPluginNoRegistryRequestPerRequest(t *testing.T) {
	reg := ocitest.NewRegistry(t)
	// The plugin's digest is not in the registry: every fetch fails.
	command := "oci://" + reg.Host() + "/plugins/missing@sha256:" + strings.Repeat("ab", 32) + "?arch=linux/amd64"

	cfg := ociplugins.DefaultOCIConfig()
	cfg.CacheDir = t.TempDir()
	cfg.GCInterval = 0
	oci := PluginData{ID: 3, Name: "oci-unverifiable", HookType: "post_auth", Command: command, IsActive: true}
	svc := &mockPluginService{
		pluginsByLLM:  map[uint][]PluginData{1: {oci}},
		getPluginByID: map[uint]PluginData{3: oci},
	}
	pm, err := NewPluginManagerWithOCI(svc, cfg, acceptingVerifier{})
	require.NoError(t, err)
	pm.loadRetryInterval = time.Hour

	registryRequests := func() int {
		n := 0
		for _, method := range []string{"GET", "HEAD"} {
			for _, kind := range []string{"manifests", "blobs", "referrers"} {
				n += reg.Requests(method, kind)
			}
		}
		return n
	}

	_, err = pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	requirePluginLoadError(t, err, 3)
	afterFirst := registryRequests()
	require.NotZero(t, afterFirst, "the first request reached the registry")

	for i := 0; i < 20; i++ {
		_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
		requirePluginLoadError(t, err, 3)
	}
	assert.Equal(t, afterFirst, registryRequests(), "refused requests make no registry request")
}

// A file plugin the edge refuses (PLUGINS_ALLOWED_SOURCES=oci) is a load
// failure like any other. Once the cause is gone, a configuration sync loads
// the plugin at once, without waiting for the retry interval, and requests
// are served again.
func TestGetPluginsForLLM_RecoversOnReconcile(t *testing.T) {
	self, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(msgSizeHelperEnv, "gatewayplugin_sdk")
	t.Setenv("PLUGIN_RUNTIME", "gateway")
	t.Setenv(pluginenv.EnvPassthrough, msgSizeHelperEnv+",PLUGIN_RUNTIME")
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")
	logs := captureLogs(t)

	p := PluginData{ID: 1, Name: "file-plugin", HookType: "post_auth", Command: self, IsActive: true}
	svc := &mockPluginService{
		pluginsByLLM:       map[uint][]PluginData{1: {p}},
		allActiveGwPlugins: []PluginData{p},
		getPluginByID:      map[uint]PluginData{1: p},
	}
	pm := NewPluginManager(svc)
	pm.loadRetryInterval = time.Hour
	pm.SetSourcePolicy(mustSourcePolicy(t, "oci"))
	t.Cleanup(func() { _ = pm.UnloadPlugin(1) })

	_, err = pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	requireSourceNotAllowed(t, requirePluginLoadError(t, err, 1))

	// The cause is gone, but requests do not retry within the interval.
	pm.SetSourcePolicy(plugin_security.AllowAllSources())
	_, err = pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	assert.True(t, requirePluginLoadError(t, err, 1).Remembered)

	require.NoError(t, pm.ReconcilePlugins(context.Background()))
	list, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Contains(t, logs.String(), "Plugin loaded after earlier failures")
}

// Remembered failures of plugins removed from the gateway are dropped.
func TestReconcile_PrunesRememberedFailuresOfRemovedPlugins(t *testing.T) {
	pm := NewPluginManager(&mockPluginService{})
	pm.rememberLoadFailure(9, "gone", errors.New("boom"))
	require.NotNil(t, pm.rememberedLoadFailure(9))

	require.NoError(t, pm.ReconcilePlugins(context.Background()))
	assert.Nil(t, pm.rememberedLoadFailure(9))
}

// A load that fails before the plugin could be read is still logged with
// the name the caller knew.
func TestLoadFailure_LoggedWithTheNameWhenThePluginCannotBeRead(t *testing.T) {
	logs := captureLogs(t)
	p := PluginData{ID: 4, Name: "unreadable", HookType: "post_auth", Command: "/bin/true", IsActive: true}
	pm := NewPluginManager(&mockPluginService{
		pluginsByLLM: map[uint][]PluginData{1: {p}},
		errGetPlugin: errors.New("database is locked"),
	})

	_, err := pm.GetPluginsForLLM(1, interfaces.HookTypePostAuth)
	assert.Equal(t, "unreadable", requirePluginLoadError(t, err, 4).PluginName)
	assert.Contains(t, logs.String(), `"plugin_name":"unreadable"`)
}
