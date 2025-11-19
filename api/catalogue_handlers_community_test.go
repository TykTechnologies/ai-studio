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

func TestCatalogueHandlers_CommunityEdition_Return402(t *testing.T) {
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
	api.setupCatalogueRoutes(r.Group("/api/v1"))

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"CreateCatalogue", "POST", "/api/v1/catalogues"},
		{"GetCatalogue", "GET", "/api/v1/catalogues/1"},
		{"UpdateCatalogue", "PATCH", "/api/v1/catalogues/1"},
		{"DeleteCatalogue", "DELETE", "/api/v1/catalogues/1"},
		{"ListCatalogues", "GET", "/api/v1/catalogues"},
		{"SearchCatalogues", "GET", "/api/v1/catalogues/search"},
		{"AddLLMToCatalogue", "POST", "/api/v1/catalogues/1/llms"},
		{"RemoveLLMFromCatalogue", "DELETE", "/api/v1/catalogues/1/llms/1"},
		{"ListCatalogueLLMs", "GET", "/api/v1/catalogues/1/llms"},
		{"SearchCataloguesByName", "GET", "/api/v1/catalogues/search-by-name"},
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

// setupCatalogueRoutes registers catalogue-related routes
func (a *API) setupCatalogueRoutes(r *gin.RouterGroup) {
	r.POST("/catalogues", a.createCatalogue)
	r.GET("/catalogues/:id", a.getCatalogue)
	r.PATCH("/catalogues/:id", a.updateCatalogue)
	r.DELETE("/catalogues/:id", a.deleteCatalogue)
	r.GET("/catalogues", a.listCatalogues)
	r.GET("/catalogues/search", a.searchCatalogues)
	r.POST("/catalogues/:id/llms", a.addLLMToCatalogue)
	r.DELETE("/catalogues/:id/llms/:llmId", a.removeLLMFromCatalogue)
	r.GET("/catalogues/:id/llms", a.listCatalogueLLMs)
	r.GET("/catalogues/search-by-name", a.searchCataloguesByNameStub)
}
