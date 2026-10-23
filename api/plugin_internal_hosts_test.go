package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// The plugin API refuses an OCI plugin whose registry resolves to an
// internal address, naming the host and the setting, and accepts it with
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

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	w = performRequest(api.router, "POST", "/api/v1/plugins", create)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
