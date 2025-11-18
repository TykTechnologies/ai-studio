// go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/sso"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSSOTestService(t *testing.T) (*API, *gin.Engine, *gorm.DB) {
	api, db := setupTestAPI(t)

	// Create default group
	defaultGroup := &models.Group{
		Name: "Default Group",
	}
	err := db.Create(defaultGroup).Error
	require.NoError(t, err)
	require.Equal(t, uint(1), defaultGroup.ID) // Default group should have ID 1

	// Setup config and SSO service
	config := &sso.Config{
		APISecret: "test-secret",
		LogLevel:  "info",
	}
	ssoService := sso.NewService(config, gin.New(), db, nil)
	if err := ssoService.InitInternalTIB(); err != nil {
		t.Fatalf("Failed to initialize SSO service: %v", err)
	}
	api.ssoService = ssoService
	api.config.TIBAPISecret = config.APISecret
	api.config.TIBEnabled = true // Enable TIB for tests

	r := gin.New()
	return api, r, db
}

func TestHandleNonceRequest(t *testing.T) {
	api, r, _ := setupSSOTestService(t)
	r.POST("/api/sso", api.handleNonceRequest)

	t.Run("Valid request", func(t *testing.T) {
		request := sso.NonceTokenRequest{
			ForSection:   "dashboard",
			EmailAddress: "test@example.com",
		}

		w := performRequest(r, "POST", "/api/sso", request)

		assert.Equal(t, http.StatusOK, w.Code)

		var response sso.NonceTokenResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "ok", response.Status)
		assert.Equal(t, "Nonce token created", response.Message)
		assert.NotNil(t, response.Meta)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		w := performRequest(r, "POST", "/api/sso", "invalid json")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid section", func(t *testing.T) {
		request := sso.NonceTokenRequest{
			ForSection:   "invalid",
			EmailAddress: "test@example.com",
		}

		w := performRequest(r, "POST", "/api/sso", request)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleSSO(t *testing.T) {
	api, r, _ := setupSSOTestService(t)
	r.GET("/sso", api.handleSSO)

	t.Run("Missing nonce token", func(t *testing.T) {
		w := performRequest(r, "GET", "/sso", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid nonce token", func(t *testing.T) {
		w := performRequest(r, "GET", "/sso?nonce=invalid", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Valid nonce token", func(t *testing.T) {
		// First create a nonce token
		request := sso.NonceTokenRequest{
			ForSection:   "dashboard",
			EmailAddress: "test@example.com",
			DisplayName:  "Test User",
			GroupID:      "1",
		}
		nonceToken, err := api.ssoService.GenerateNonce(request)
		require.NoError(t, err)
		require.NotNil(t, nonceToken)

		w := performRequest(r, "GET", "/sso?nonce="+*nonceToken, nil)
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
	})
}

func TestSSOAuthMiddleware(t *testing.T) {
	api, r, _ := setupSSOTestService(t)
	r.Use(api.SSOAuthMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	t.Run("Missing authorization header", func(t *testing.T) {
		w := performRequest(r, "GET", "/test", nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Invalid authorization header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "invalid")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Valid authorization header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", api.config.TIBAPISecret)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
