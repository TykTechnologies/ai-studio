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

func TestDataCatalogueHandlers_CommunityEdition_Return402(t *testing.T) {
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
	api.setupDataCatalogueRoutes(r.Group("/api/v1"))

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"CreateDataCatalogue", "POST", "/api/v1/data-catalogues"},
		{"GetDataCatalogue", "GET", "/api/v1/data-catalogues/1"},
		{"UpdateDataCatalogue", "PATCH", "/api/v1/data-catalogues/1"},
		{"DeleteDataCatalogue", "DELETE", "/api/v1/data-catalogues/1"},
		{"ListDataCatalogues", "GET", "/api/v1/data-catalogues"},
		{"SearchDataCatalogues", "GET", "/api/v1/data-catalogues/search"},
		{"AddTagToDataCatalogue", "POST", "/api/v1/data-catalogues/1/tags"},
		{"RemoveTagFromDataCatalogue", "DELETE", "/api/v1/data-catalogues/1/tags/tag1"},
		{"AddDatasourceToDataCatalogue", "POST", "/api/v1/data-catalogues/1/datasources/1"},
		{"RemoveDatasourceFromDataCatalogue", "DELETE", "/api/v1/data-catalogues/1/datasources/1"},
		{"GetDataCataloguesByTag", "GET", "/api/v1/data-catalogues/tag/tag1"},
		{"GetDataCataloguesByDatasource", "GET", "/api/v1/data-catalogues/datasource/1"},
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

// setupDataCatalogueRoutes registers data catalogue-related routes
func (a *API) setupDataCatalogueRoutes(r *gin.RouterGroup) {
	r.POST("/data-catalogues", a.createDataCatalogue)
	r.GET("/data-catalogues/:id", a.getDataCatalogue)
	r.PATCH("/data-catalogues/:id", a.updateDataCatalogue)
	r.DELETE("/data-catalogues/:id", a.deleteDataCatalogue)
	r.GET("/data-catalogues", a.listDataCatalogues)
	r.GET("/data-catalogues/search", a.searchDataCatalogues)
	r.POST("/data-catalogues/:id/tags", a.addTagToDataCatalogue)
	r.DELETE("/data-catalogues/:id/tags/:tag", a.removeTagFromDataCatalogue)
	r.POST("/data-catalogues/:id/datasources/:datasourceId", a.addDatasourceToDataCatalogue)
	r.DELETE("/data-catalogues/:id/datasources/:datasourceId", a.removeDatasourceFromDataCatalogue)
	r.GET("/data-catalogues/tag/:tag", a.getDataCataloguesByTag)
	r.GET("/data-catalogues/datasource/:datasourceId", a.getDataCataloguesByDatasource)
}
