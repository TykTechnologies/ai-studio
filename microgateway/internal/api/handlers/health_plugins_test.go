package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readyPlugins is the gateway's plugin configuration for these tests.
type readyPlugins struct {
	plugins.PluginServiceInterface
	byID map[uint]plugins.PluginData
}

func (s readyPlugins) GetPlugin(id uint) (plugins.PluginData, error) {
	p, ok := s.byID[id]
	if !ok {
		return plugins.PluginData{}, fmt.Errorf("plugin %d not found", id)
	}
	return p, nil
}

func readyResponse(t *testing.T, sc *services.ServiceContainer) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	ReadinessCheck(sc)(c)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return w.Code, body
}

func notReady(body map[string]interface{}) []map[string]interface{} {
	list, _ := body["plugins_not_ready"].([]interface{})
	out := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

// A plugin that failed to load does not make the gateway not ready. It
// affects the LLMs it is attached to, whose requests are refused; /ready
// used to answer 503 for it, and since every pod of an edge has the same
// plugins, Kubernetes took the whole edge out of service and every LLM went
// down with the one. /ready is 200 and its body names the plugin.
func TestReadinessWithAFailedPluginIsReadyAndNamesIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	const secretPath = "/nonexistent/secret-dir/llm-firewall"
	pm := plugins.NewPluginManager(readyPlugins{byID: map[uint]plugins.PluginData{
		7: {ID: 7, Name: "llm-firewall", HookType: "post_auth", Command: secretPath, IsActive: true},
	}})
	sc := &services.ServiceContainer{DB: db, PluginManager: pm}

	code, body := readyResponse(t, sc)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, notReady(body), "nothing has failed yet")
	assert.Contains(t, body, "plugins_not_ready", "the list is there also when it is empty")

	_, err = pm.LoadPlugin(7)
	require.Error(t, err, "the plugin cannot be loaded")
	require.False(t, pm.IsAllPluginsReady())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	ReadinessCheck(sc)(c)
	require.Equal(t, http.StatusOK, w.Code, "one failed plugin must not take the gateway out of service: %s", w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assert.Equal(t, "ready", body["status"])
	list := notReady(body)
	require.Len(t, list, 1)
	assert.EqualValues(t, 7, list[0]["id"])
	assert.Equal(t, "llm-firewall", list[0]["name"])
	assert.Equal(t, "failed", list[0]["status"])
	summary, _ := body["plugin_health"].(map[string]interface{})
	assert.EqualValues(t, 1, summary["failed_plugins"])
	assert.Equal(t, false, summary["all_ready"])

	// /ready is unauthenticated: no path, command or cause in it.
	assert.NotContains(t, w.Body.String(), "secret-dir")
	assert.NotContains(t, w.Body.String(), "not found")

	// The plugin's own state is unchanged: it is still a failure for the
	// detailed status, and for requests to its LLMs (plugins.PluginLoadError).
	assert.Error(t, sc.PluginsReady())
	assert.NoError(t, sc.Health())
}

// A plugin that is still loading (a grpc:// host being connected to, a
// process starting: up to 20 seconds at every edge start) does not make the
// gateway not ready either. /ready is 200 and lists it as loading.
func TestReadinessWithALoadingPluginIsReadyAndNamesIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	// A "plugin" that starts and never completes the handshake: the load
	// waits for it, in the loading state, until it exits.
	slow := filepath.Join(t.TempDir(), "slow-plugin")
	require.NoError(t, os.WriteFile(slow, []byte("#!/bin/sh\nsleep 4\n"), 0o755))
	pm := plugins.NewPluginManager(readyPlugins{byID: map[uint]plugins.PluginData{
		3: {ID: 3, Name: "ext-grpc-slow", HookType: "post_auth", Command: slow, IsActive: true},
	}})
	sc := &services.ServiceContainer{DB: db, PluginManager: pm}

	loaded := make(chan struct{})
	go func() {
		defer close(loaded)
		_, _ = pm.LoadPlugin(3)
	}()
	t.Cleanup(func() { <-loaded })

	require.Eventually(t, func() bool {
		health := pm.GetPluginHealth()[3]
		return health != nil && health.Status == plugins.PluginStatusLoading
	}, 3*time.Second, 5*time.Millisecond, "the plugin is loading")

	code, body := readyResponse(t, sc)
	require.Equal(t, http.StatusOK, code, "a loading plugin must not take the gateway out of service: %v", body)
	list := notReady(body)
	require.Len(t, list, 1)
	assert.EqualValues(t, 3, list[0]["id"])
	assert.Equal(t, "ext-grpc-slow", list[0]["name"])
	assert.Equal(t, "loading", list[0]["status"])
}

// A gateway that cannot serve at all is still not ready: without its
// database it can neither authenticate a request nor find an LLM.
func TestReadinessWithoutADatabaseIsNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sc := &services.ServiceContainer{DB: db, PluginManager: plugins.NewPluginManager(readyPlugins{})}

	code, _ := readyResponse(t, sc)
	require.Equal(t, http.StatusOK, code)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	code, body := readyResponse(t, sc)
	require.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "not ready", body["status"])
	assert.Contains(t, body["error"], "database")
}

// The detailed health status keeps counting a failed plugin as unhealthy.
func TestDetailedHealthStillReportsAFailedPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS", "1")
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	pm := plugins.NewPluginManager(readyPlugins{byID: map[uint]plugins.PluginData{
		7: {ID: 7, Name: "llm-firewall", HookType: "post_auth", Command: "/nonexistent/llm-firewall", IsActive: true},
	}})
	_, err = pm.LoadPlugin(7)
	require.Error(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health/detailed", nil)
	GetDetailedHealthStatus(&services.ServiceContainer{DB: db, PluginManager: pm})(c)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "1 plugins failed")
}
