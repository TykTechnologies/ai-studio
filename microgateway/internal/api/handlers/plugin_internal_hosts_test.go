package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
)

// The management API refuses an OCI plugin whose registry resolves to an
// internal address, on create and on pre-warm, naming the host and the
// setting; ALLOW_INTERNAL_NETWORK_ACCESS=true allows it
// (midsommar-clean-c1g.32).
func TestPluginHandlers_InternalRegistryRefused(t *testing.T) {
	netguardtest.StubLookupIP(t, map[string]string{"registry.corp.internal": "10.0.0.5"})
	container, router := setupTestHandlers(t)
	router.POST("/plugins", CreatePlugin(container))
	router.POST("/plugins/prewarm", TriggerPluginPreWarm(container))
	command := "oci://registry.corp.internal/plugins/demo:1.0.0"

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(t, "POST", "/plugins", services.CreatePluginRequest{
		Name: "corp", Command: command, HookType: "post_auth",
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "registry.corp.internal")
	assert.Contains(t, w.Body.String(), "ALLOW_INTERNAL_NETWORK_ACCESS")

	w = httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(t, "POST", "/plugins/prewarm", map[string]string{"command": command}))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "ALLOW_INTERNAL_NETWORK_ACCESS")

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, jsonRequest(t, "POST", "/plugins", services.CreatePluginRequest{
		Name: "corp", Command: command, HookType: "post_auth",
	}))
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
