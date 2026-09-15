package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestPluginResourceTypes_AccessGrantedViaApp proves the admin and portal
// type listings expose the resolved access class and the portal path, and
// that the resolution needs no plugin change: a plugin shaped like the MCP
// registry (custom endpoints + resources) resolves true, one shaped like the
// asset catalog (resources only) resolves false.
func TestPluginResourceTypes_AccessGrantedViaApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	service := services.NewService(db)
	api := &API{service: service}

	mcp := &models.Plugin{Name: "mcp-registry", Command: "/usr/bin/mcp", HookType: models.HookTypeCustomEndpoint,
		HookTypes: []string{models.HookTypeCustomEndpoint, models.HookTypeStudioUI, models.HookTypeResourceProvider}, IsActive: true}
	catalog := &models.Plugin{Name: "asset-catalog", Command: "/usr/bin/catalog", HookType: models.HookTypeResourceProvider,
		HookTypes: []string{models.HookTypeResourceProvider, models.HookTypeStudioUI, models.HookTypePortalUI}, IsActive: true}
	require.NoError(t, db.Create(mcp).Error)
	require.NoError(t, db.Create(catalog).Error)

	require.NoError(t, service.RegisterPluginResourceTypes(mcp.ID, []models.PluginResourceType{
		{Slug: "mcp_servers", Name: "MCP Servers", SupportsSubmissions: true},
	}))
	require.NoError(t, service.RegisterPluginResourceTypes(catalog.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent", SupportsSubmissions: true, PortalDetailPath: "/portal/plugins/asset-catalog#/assets/{id}"},
	}))

	user := &models.User{Email: "admin@test.com", Name: "Admin", EmailVerified: true}
	require.NoError(t, user.Create(db))

	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user", user); c.Next() })
	router.GET("/api/v1/plugin-resource-types", api.listPluginResourceTypes)
	router.GET("/common/plugin-resource-types", api.listSubmittablePluginResourceTypes)

	for _, path := range []string{"/api/v1/plugin-resource-types", "/common/plugin-resource-types"} {
		t.Run(path, func(t *testing.T) {
			req, _ := http.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var body struct {
				Data []map[string]interface{} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			bySlug := map[string]map[string]interface{}{}
			for _, entry := range body.Data {
				bySlug[entry["slug"].(string)] = entry
			}
			require.Contains(t, bySlug, "mcp_servers")
			require.Contains(t, bySlug, "agent")

			assert.Equal(t, true, bySlug["mcp_servers"]["access_granted_via_app"], "proxying provider resolves true")
			assert.Equal(t, "", bySlug["mcp_servers"]["portal_detail_path"])
			assert.Equal(t, false, bySlug["agent"]["access_granted_via_app"], "catalog-style provider resolves false")
			assert.Equal(t, "/portal/plugins/asset-catalog#/assets/{id}", bySlug["agent"]["portal_detail_path"])
		})
	}
}

func TestPortalDetailURL(t *testing.T) {
	assert.Equal(t, "", portalDetailURL("", "x"))
	assert.Equal(t, "/portal/plugins/asset-catalog#/assets/ast_1", portalDetailURL("/portal/plugins/asset-catalog#/assets/{id}", "ast_1"))
	assert.Equal(t, "/portal/plugins/asset-catalog#/assets/a%2Fb%3Fc", portalDetailURL("/portal/plugins/asset-catalog#/assets/{id}", "a/b?c"), "ids are path-escaped")
	assert.Equal(t, "/portal/plugins/x", portalDetailURL("/portal/plugins/x", "ignored"), "template without placeholder is returned as is")
}

// Built-in catalog items are always reachable through an App credential.
func TestCatalogItems_BuiltInsAreAppGranted(t *testing.T) {
	assert.True(t, llmCatalogItem(&models.LLM{Name: "L"}).Attributes.AccessGrantedViaApp)
	assert.True(t, datasourceCatalogItem(&models.Datasource{Name: "D"}).Attributes.AccessGrantedViaApp)
	assert.True(t, toolCatalogItem(&models.Tool{Name: "T"}).Attributes.AccessGrantedViaApp)
}
