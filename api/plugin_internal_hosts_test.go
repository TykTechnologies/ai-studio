package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// The plugin API refuses an OCI plugin whose registry resolves to an
// internal address, naming the host and the settings, and accepts it when the
// host is named in LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS or with
// ALLOW_INTERNAL_NETWORK_ACCESS=true (midsommar-clean-c1g.32).
func TestPluginAPI_InternalRegistryRefused(t *testing.T) {
	netguardtest.StubLookupIP(t, map[string]string{"registry.corp.internal": "10.0.0.5"})
	api, _ := setupTestAPI(t)
	create := services.CreatePluginRequest{
		Name: "corp", Command: "oci://registry.corp.internal/plugins/demo:1.0.0", HookType: "post_auth",
	}

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	w := performRequest(api.router, "POST", "/api/v1/plugins", create)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "registry.corp.internal")
	assert.Contains(t, w.Body.String(), "ALLOW_INTERNAL_NETWORK_ACCESS")

	assert.Contains(t, w.Body.String(), "LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS")

	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "registry.corp.internal")
	w = performRequest(api.router, "POST", "/api/v1/plugins", create)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "")

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	create.Name = "corp-2"
	w = performRequest(api.router, "POST", "/api/v1/plugins", create)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

// An administrator can turn a plugin's "Allow internal host" switch off while
// its command is internal: the edit saves and the plugin is refused when it
// is loaded or dialled instead (w4e.5). A new internal command with the
// switch off is still refused, and the refusal names the switch.
func TestPluginAPI_InternalHostSwitchOffSaves(t *testing.T) {
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "")
	a, _ := setupTestAPI(t)
	admin := internalHostRouter(a, true)

	create := services.CreatePluginRequest{Name: "grpc-internal", Command: "grpc://10.0.0.5:9001", HookType: "post_auth"}
	w := performRequest(admin, "POST", "/plugins", create)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Allow internal host", "the refusal names the switch")

	create.AllowInternalHost = true
	w = performRequest(admin, "POST", "/plugins", create)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := createdID(t, w.Body.Bytes())
	path := fmt.Sprintf("/plugins/%d", id)

	// The edit form sends the unchanged command along with the switch.
	w = performRequest(admin, "PATCH", path, map[string]any{"command": create.Command, "allow_internal_host": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	stored, err := a.service.PluginService.GetPlugin(uint(id))
	require.NoError(t, err)
	assert.False(t, stored.AllowInternalHost, "the switch was saved off")
	assert.Error(t, netguard.CheckPluginHostWith(stored.Exemption(), "10.0.0.5"), "the stored plugin's host is refused at dial")

	w = performRequest(admin, "PATCH", path, map[string]any{"command": "grpc://10.0.0.6:9001"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Allow internal host")
}
