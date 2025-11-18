package api

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

var emptyFile embed.FS

func setupTestAPI(t *testing.T) (*API, *gorm.DB) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	return api, db
}

func performRequest(r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandleGetConfig(t *testing.T) {
	// Save original env var and restore it after the test
	originalSiteURL := os.Getenv("SITE_URL")
	originalTIBEnabled := os.Getenv("TIB_ENABLED")
	originalDefaultSignupMode := os.Getenv("DEFAULT_SIGNUP_MODE")
	originalProxyURL := os.Getenv("PROXY_URL")

	defer func() {
		os.Setenv("SITE_URL", originalSiteURL)
		os.Setenv("TIB_ENABLED", originalTIBEnabled)
		os.Setenv("DEFAULT_SIGNUP_MODE", originalDefaultSignupMode)
		os.Setenv("PROXY_URL", originalProxyURL)
		// Force config reload on next Get() call
		// We can't directly set the config, but we can make sure it's reloaded from env vars
	}()

	testCases := []struct {
		name       string
		tibEnabled bool
		siteURL    string
		checkURL   bool
	}{
		{
			name:       "TIB Enabled",
			tibEnabled: true,
			siteURL:    "",
			checkURL:   true,
		},
		{
			name:       "TIB Disabled",
			tibEnabled: false,
			siteURL:    "",
			checkURL:   true,
		},
		{
			name:       "With Custom Site URL",
			tibEnabled: true,
			siteURL:    "https://custom.example.com",
			checkURL:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up test environment
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Set up request with test host
			req := httptest.NewRequest("GET", "/auth/config", nil)
			req.Host = "example.com"
			c.Request = req

			// Set environment variables for the test
			if tc.siteURL != "" {
				os.Setenv("SITE_URL", tc.siteURL)
			} else {
				os.Unsetenv("SITE_URL")
			}

			// Set TIB_ENABLED env var
			if tc.tibEnabled {
				os.Setenv("TIB_ENABLED", "true")
			} else {
				os.Unsetenv("TIB_ENABLED")
			}

			// Set default signup mode
			os.Setenv("DEFAULT_SIGNUP_MODE", "both")

			// Unset proxy URL for test
			os.Unsetenv("PROXY_URL")

			// Create API instance with test config
			authConfig := &auth.Config{
				TIBEnabled: tc.tibEnabled,
			}
			api := &API{
				config: authConfig,
			}

			// Call the handler
			api.handleGetConfig(c)

			// Assert response
			assert.Equal(t, http.StatusOK, w.Code)

			var response FrontendConfig
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			// Check response values
			if tc.checkURL {
				expectedURL := "http://example.com"
				if tc.siteURL != "" {
					expectedURL = tc.siteURL
				}
				assert.Equal(t, expectedURL, response.APIBaseURL)
			}
			assert.Equal(t, "both", response.DefaultSignUpMode)
			// Note: TIBEnabled is determined by sso.IsEnterpriseAvailable(), not by config
			// In CE builds, this will always be false; in Enterprise builds with SSO factory registered, it will be true
			// We don't assert a specific value since it depends on the build edition
		})
	}
}
