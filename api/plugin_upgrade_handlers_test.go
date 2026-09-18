package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	upgradeTestDigestV1 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	upgradeTestDigestV2 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func setupTestAPIWithMarketplace(t *testing.T) (*API, *gorm.DB) {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	service.MarketplaceService = services.NewMarketplaceService(db, nil, service.PluginService, nil, t.TempDir(), "", 0)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	for version, digest := range map[string]string{"1.0.0": upgradeTestDigestV1, "1.1.0": upgradeTestDigestV2} {
		require.NoError(t, db.Create(&models.MarketplacePlugin{
			PluginID: "com.tyk.cache", Version: version, Name: "Cache",
			OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCIDigest: digest,
			SyncedFromURL: "https://marketplace.example.com/index.yaml",
		}).Error)
	}
	return api, db
}

func TestPluginAPI_ReportsMarketplaceUpdates(t *testing.T) {
	api, db := setupTestAPIWithMarketplace(t)

	// Installed the way the wizard does it: a plain create with an OCI command.
	w := performRequest(api.router, "POST", "/api/v1/plugins", services.CreatePluginRequest{
		Name:     "cache",
		Command:  "oci://ghcr.io/tyk/cache@" + upgradeTestDigestV1,
		HookType: "post_auth",
		IsActive: true,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created struct {
		Data PluginResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotNil(t, created.Data.Marketplace, "the install is linked to its marketplace entry with no extra bookkeeping")
	assert.Equal(t, "com.tyk.cache", created.Data.Marketplace.MarketplaceID)
	assert.Equal(t, "1.0.0", created.Data.Version)
	assert.Equal(t, "1.1.0", created.Data.Marketplace.AvailableVersion)
	assert.True(t, created.Data.Marketplace.UpdateAvailable)

	// A plugin that did not come from the marketplace carries no link.
	w = performRequest(api.router, "POST", "/api/v1/plugins", services.CreatePluginRequest{
		Name: "local", Command: "/usr/local/bin/local-plugin", HookType: "post_auth", IsActive: true,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w = performRequest(api.router, "GET", "/api/v1/plugins", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list PluginListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Equal(t, int64(1), list.UpdatesAvailable)
	require.Len(t, list.Data, 2)
	for _, p := range list.Data {
		if p.Attributes.Name == "cache" {
			require.NotNil(t, p.Marketplace)
			assert.True(t, p.Marketplace.UpdateAvailable)
			assert.Equal(t, "1.0.0", p.Version)
		} else {
			assert.Nil(t, p.Marketplace)
		}
	}

	// The marketplace listing knows the plugin is installed.
	w = performRequest(api.router, "GET", "/api/v1/marketplace/plugins", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var marketplaceList struct {
		Installed map[string][]struct {
			PluginID         uint   `json:"plugin_id"`
			InstalledVersion string `json:"installed_version"`
			UpdateAvailable  bool   `json:"update_available"`
		} `json:"installed"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &marketplaceList))
	require.Len(t, marketplaceList.Installed["com.tyk.cache"], 1)
	assert.Equal(t, "1.0.0", marketplaceList.Installed["com.tyk.cache"][0].InstalledVersion)
	assert.True(t, marketplaceList.Installed["com.tyk.cache"][0].UpdateAvailable)

	// Hand-editing the command to the newer artifact clears the update.
	newCommand := "oci://ghcr.io/tyk/cache@" + upgradeTestDigestV2
	w = performRequest(api.router, "PATCH", "/api/v1/plugins/"+created.Data.ID, services.UpdatePluginRequest{Command: &newCommand})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var patched struct {
		Data PluginResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &patched))
	require.NotNil(t, patched.Data.Marketplace)
	assert.Equal(t, "1.1.0", patched.Data.Version)
	assert.False(t, patched.Data.Marketplace.UpdateAvailable)

	// Deleting the plugin removes its tracking row.
	w = performRequest(api.router, "DELETE", "/api/v1/plugins/"+created.Data.ID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	var rows int64
	require.NoError(t, db.Unscoped().Model(&models.InstalledPluginVersion{}).Count(&rows).Error)
	assert.Equal(t, int64(0), rows)
}

func TestPluginUpgradeEndpoints_Refusals(t *testing.T) {
	api, _ := setupTestAPIWithMarketplace(t)

	w := performRequest(api.router, "POST", "/api/v1/plugins/not-a-number/upgrade", gin.H{})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = performRequest(api.router, "POST", "/api/v1/plugins/999999/upgrade", gin.H{})
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = performRequest(api.router, "POST", "/api/v1/plugins/999999/upgrade/preview", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = performRequest(api.router, "POST", "/api/v1/plugins", services.CreatePluginRequest{
		Name: "local", Command: "/usr/local/bin/local-plugin", HookType: "post_auth", IsActive: true,
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Data PluginResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// A plugin with no marketplace entry has nothing to upgrade to.
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/plugins/%s/upgrade", created.Data.ID), gin.H{})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestRespondPluginUpgradeError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	respond := func(err error) (*httptest.ResponseRecorder, map[string]interface{}) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondPluginUpgradeError(c, err)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return w, body
	}

	w, body := respond(&services.UpgradeScopesNotApprovedError{Missing: []string{"llms.proxy"}})
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, []interface{}{"llms.proxy"}, body["missing_scopes"], "the UI asks for exactly these")

	w, body = respond(&services.UpgradeRolledBackError{Cause: errors.New("init failed")})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, true, body["rolled_back"])

	w, body = respond(&services.UpgradeRolledBackError{Cause: errors.New("init failed"), RollbackErr: errors.New("old version gone")})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, false, body["rolled_back"])

	for err, status := range map[error]int{
		services.ErrUpgradeNotFromMarketplace:                         http.StatusBadRequest,
		services.ErrUpgradeVersionNotFound:                            http.StatusNotFound,
		services.ErrUpgradeSameVersion:                                http.StatusConflict,
		services.ErrUpgradeDowngradeNotAllowed:                        http.StatusConflict,
		fmt.Errorf("%w: a vs b", services.ErrUpgradeManifestMismatch): http.StatusUnprocessableEntity,
		services.ErrUpgradeEnterpriseOnly:                             http.StatusForbidden,
		services.ErrUpgradePluginNotFound:                             http.StatusNotFound,
		// A registry "not found" is about the artifact, never about the plugin.
		fmt.Errorf("%w: manifest not found", services.ErrUpgradeTargetUnavailable): http.StatusUnprocessableEntity,
		errors.New("database is down"):                                             http.StatusInternalServerError,
	} {
		w, _ := respond(err)
		assert.Equal(t, status, w.Code, err.Error())
	}
}
