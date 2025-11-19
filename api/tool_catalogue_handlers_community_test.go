//go:build !enterprise
// +build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestToolCatalogueHandlers_CommunityEdition_Return402(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Create admin user for authentication
	admin := createTestUserWithSettings(t, service, "admin@test.com", "Admin", true, true, true, true, false)

	// Setup router with admin user context
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", admin)
		c.Next()
	})

	// Register routes
	api.setupToolCatalogueRoutes(r.Group("/api/v1"))

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"CreateToolCatalogue", "POST", "/api/v1/tool-catalogues"},
		{"GetToolCatalogue", "GET", "/api/v1/tool-catalogues/1"},
		{"UpdateToolCatalogue", "PATCH", "/api/v1/tool-catalogues/1"},
		{"DeleteToolCatalogue", "DELETE", "/api/v1/tool-catalogues/1"},
		{"ListToolCatalogues", "GET", "/api/v1/tool-catalogues"},
		{"SearchToolCatalogues", "GET", "/api/v1/tool-catalogues/search"},
		{"AddToolToToolCatalogue", "POST", "/api/v1/tool-catalogues/1/tools/1"},
		{"RemoveToolFromToolCatalogue", "DELETE", "/api/v1/tool-catalogues/1/tools/1"},
		{"GetToolCatalogueTools", "GET", "/api/v1/tool-catalogues/1/tools"},
		{"GetToolCatalogueToolsSecure", "GET", "/api/v1/tool-catalogues/1/tools/secure"},
		{"GetToolDocumentation", "GET", "/api/v1/tool-catalogues/1/tools/2/documentation"},
		{"AddTagToToolCatalogue", "POST", "/api/v1/tool-catalogues/1/tags"},
		{"RemoveTagFromToolCatalogue", "DELETE", "/api/v1/tool-catalogues/1/tags/tag1"},
		{"GetToolCatalogueTags", "GET", "/api/v1/tool-catalogues/1/tags"},
		{"GetToolUserApps", "GET", "/api/v1/tools/1/apps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := apitest.PerformRequest(r, tt.method, tt.path, nil)

			assert.Equal(t, http.StatusPaymentRequired, w.Code,
				"Handler %s should return 402 in Community Edition", tt.name)

			var errResp ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &errResp)
			assert.NoError(t, err)

			// Verify Enterprise feature message
			if len(errResp.Errors) > 0 {
				assert.Contains(t, errResp.Errors[0].Detail, "Enterprise",
					"Error message should mention Enterprise")
			}
		})
	}
}

// setupToolCatalogueRoutes registers tool catalogue-related routes
func (a *API) setupToolCatalogueRoutes(r *gin.RouterGroup) {
	r.POST("/tool-catalogues", a.createToolCatalogue)
	r.GET("/tool-catalogues/:id", a.getToolCatalogue)
	r.PATCH("/tool-catalogues/:id", a.updateToolCatalogue)
	r.DELETE("/tool-catalogues/:id", a.deleteToolCatalogue)
	r.GET("/tool-catalogues", a.listToolCatalogues)
	r.GET("/tool-catalogues/search", a.searchToolCatalogues)
	r.POST("/tool-catalogues/:id/tools/:toolId", a.addToolToToolCatalogue)
	r.DELETE("/tool-catalogues/:id/tools/:toolId", a.removeToolFromToolCatalogue)
	r.GET("/tool-catalogues/:id/tools", a.getToolCatalogueTools)
	r.GET("/tool-catalogues/:id/tools/secure", a.getToolCatalogueToolsSecure)
	r.GET("/tool-catalogues/:id/tools/:toolId/documentation", a.GetToolDocumentation)
	r.POST("/tool-catalogues/:id/tags", a.addTagToToolCatalogue)
	r.DELETE("/tool-catalogues/:id/tags/:tag", a.removeTagFromToolCatalogue)
	r.GET("/tool-catalogues/:id/tags", a.getToolCatalogueTags)
	r.GET("/tools/:toolId/apps", a.getToolUserApps)
}
