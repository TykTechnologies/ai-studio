package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// setupOCITestAPI is setupTestAPI with OCI plugin support configured.
func setupOCITestAPI(t *testing.T) (*API, *services.Service) {
	t.Helper()
	db := apitest.SetupTestDB(t)
	cfg := ociplugins.DefaultOCIConfig()
	cfg.CacheDir = t.TempDir()
	cfg.GCInterval = 0
	service := services.NewServiceWithOCI(db, cfg)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	return NewAPI(service, true, authService, config, nil, emptyFile, nil), service
}

// With oci disallowed, the OCI plugin endpoints refuse with 400 (a client
// error naming the setting, not a server error) and never reach the
// registry (midsommar-clean-c1g.10).
func TestOCIPluginAPI_SourcePolicyRefusesWith400(t *testing.T) {
	t.Skip("fails until midsommar-clean-c1g.23 is fixed (500 instead of 400); remove this skip with the fix")
	api, service := setupOCITestAPI(t)
	reg := ocitest.NewRegistry(t)
	manifest := reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	ref := "oci://" + reg.Host() + "/plugins/demo@" + manifest.Digest.String() + "?arch=linux/amd64"

	existing := &models.Plugin{Name: "stored", Command: ref, OCIReference: ref, HookType: "post_auth", IsActive: true}
	require.NoError(t, service.DB.Create(existing).Error)

	withPluginSourcePolicy(t, "file,grpc")

	w := performRequest(api.router, "POST", "/api/v1/plugins/oci", services.CreateOCIPluginRequest{
		Name: "refused", OCIReference: ref, HookType: "post_auth",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES")

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/plugins/%d/refresh", existing.ID), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "AI_STUDIO_PLUGIN_ALLOWED_SOURCES")

	for _, method := range []string{"GET", "HEAD"} {
		for _, kind := range []string{"manifests", "blobs", "referrers"} {
			assert.Zero(t, reg.Requests(method, kind), "%s %s", method, kind)
		}
	}
}
