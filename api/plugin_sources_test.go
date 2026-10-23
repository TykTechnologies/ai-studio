package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

// withPluginSourcePolicy installs a configuration whose
// AI_STUDIO_PLUGIN_ALLOWED_SOURCES is csv for the rest of the test.
func withPluginSourcePolicy(t *testing.T, csv string) {
	t.Helper()
	policy, err := plugin_security.ParseSourcePolicy(csv)
	require.NoError(t, err)
	saved := appconfig.Get("")
	conf := *saved
	conf.PluginSourcePolicy = policy.WithSetting("AI_STUDIO_PLUGIN_ALLOWED_SOURCES")
	appconfig.Set(&conf)
	t.Cleanup(func() { appconfig.Set(saved) })
}

func TestPluginAPI_SourcePolicy(t *testing.T) {
	api, db := setupTestAPI(t)
	// A plugin stored before the policy was restricted.
	existing := &models.Plugin{Name: "legacy", Command: "/usr/local/bin/legacy", HookType: "post_auth", IsActive: true}
	require.NoError(t, db.Create(existing).Error)

	withPluginSourcePolicy(t, "oci")

	for _, command := range []string{"/usr/local/bin/test-plugin", "grpc://plugin.example.com:50051"} {
		w := performRequest(api.router, "POST", "/api/v1/plugins", services.CreatePluginRequest{
			Name: "refused", Command: command, HookType: "post_auth",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code, command)
		assert.Contains(t, w.Body.String(), "is not allowed by this installation", command)
		assert.Contains(t, w.Body.String(), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES", command)
	}

	// The registry host does not resolve, which the internal-network check
	// would refuse; this test is about the source policy.
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	w := performRequest(api.router, "POST", "/api/v1/plugins", services.CreatePluginRequest{
		Name: "oci plugin", Command: "oci://registry.example.com/plugins/demo:1.0.0", HookType: "post_auth",
	})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// The existing plugin can still be read and deleted, but not saved with
	// its disallowed command.
	path := fmt.Sprintf("/api/v1/plugins/%d", existing.ID)
	w = performRequest(api.router, "GET", path, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	command := existing.Command
	w = performRequest(api.router, "PATCH", path, services.UpdatePluginRequest{Command: &command})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	w = performRequest(api.router, "DELETE", path, nil)
	assert.Contains(t, []int{http.StatusOK, http.StatusNoContent}, w.Code, w.Body.String())
	var count int64
	require.NoError(t, db.Model(&models.Plugin{}).Where("id = ?", existing.ID).Count(&count).Error)
	assert.Zero(t, count)
}
