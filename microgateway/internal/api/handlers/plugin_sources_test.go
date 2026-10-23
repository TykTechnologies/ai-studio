package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

func jsonRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// With PLUGINS_ALLOWED_SOURCES=oci the management API refuses file and grpc
// plugins (create, update and pre-warm) with 400.
func TestPluginHandlers_SourcePolicy(t *testing.T) {
	container, router := setupTestHandlers(t)
	policy, err := plugin_security.ParseSourcePolicy("oci")
	require.NoError(t, err)
	container.PluginManager.SetSourcePolicy(policy.WithSetting("PLUGINS_ALLOWED_SOURCES"))

	router.POST("/plugins", CreatePlugin(container))
	router.PUT("/plugins/:id", UpdatePlugin(container))
	router.POST("/plugins/prewarm", TriggerPluginPreWarm(container))

	for _, command := range []string{"/usr/local/bin/demo", "file:///usr/local/bin/demo", "grpc://plugin.example.com:50051"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, jsonRequest(t, "POST", "/plugins", services.CreatePluginRequest{
			Name: "refused", Command: command, HookType: "post_auth",
		}))
		assert.Equal(t, http.StatusBadRequest, w.Code, command)
		assert.Contains(t, w.Body.String(), "PLUGINS_ALLOWED_SOURCES", command)
	}

	// An existing plugin cannot be switched to a disallowed command.
	plugin, err := container.PluginService.CreatePlugin(&services.CreatePluginRequest{
		Name: "existing", Command: "oci://registry.example.com/plugins/demo:1", HookType: "post_auth",
	})
	require.NoError(t, err)
	command := "/usr/local/bin/demo"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(t, "PUT", "/plugins/"+strconv.Itoa(int(plugin.ID)), services.UpdatePluginRequest{Command: &command}))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// Pre-warm refuses a source the gateway does not allow.
	policy, err = plugin_security.ParseSourcePolicy("file")
	require.NoError(t, err)
	container.PluginManager.SetSourcePolicy(policy)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(t, "POST", "/plugins/prewarm", map[string]string{"command": "oci://registry.example.com/plugins/demo:1"}))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}
