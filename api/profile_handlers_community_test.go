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

func TestProfileHandlers_CommunityEdition_Return402(t *testing.T) {
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
	api.setupProfileRoutes(r.Group("/api/v1"))

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"CreateProfile", "POST", "/api/v1/sso/profiles"},
		{"GetProfile", "GET", "/api/v1/sso/profiles/1"},
		{"UpdateProfile", "PUT", "/api/v1/sso/profiles/1"},
		{"DeleteProfile", "DELETE", "/api/v1/sso/profiles/1"},
		{"ListProfiles", "GET", "/api/v1/sso/profiles"},
		{"SetProfileUseInLoginPage", "PUT", "/api/v1/sso/profiles/1/use-in-login"},
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
				assert.Contains(t, errResp.Errors[0].Detail, "Enterprise feature",
					"Error message should mention Enterprise feature")
			}
		})
	}
}

func TestGetLoginPageProfile_CommunityEdition_ReturnsNil(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Setup router (no authentication needed for public endpoint)
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Register routes
	api.setupProfileRoutes(r.Group("/api/v1"))

	w := apitest.PerformRequest(r, "GET", "/api/v1/sso/login-page-profile", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify data is nil (SSO not available in CE)
	data, exists := response["data"]
	assert.True(t, exists, "Response should have 'data' field")
	assert.Nil(t, data, "Login page profile should be nil in Community Edition")
}

// setupProfileRoutes registers profile-related routes
func (a *API) setupProfileRoutes(r *gin.RouterGroup) {
	r.POST("/sso/profiles", a.createProfile)
	r.GET("/sso/profiles/:id", a.getProfile)
	r.PUT("/sso/profiles/:id", a.updateProfile)
	r.DELETE("/sso/profiles/:id", a.deleteProfile)
	r.GET("/sso/profiles", a.listProfiles)
	r.PUT("/sso/profiles/:id/use-in-login", a.setProfileUseInLoginPage)
	r.GET("/sso/login-page-profile", a.getLoginPageProfile)
}
