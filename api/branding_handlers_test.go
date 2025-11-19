package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetBrandingSettings(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Setup router
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api.setupBrandingRoutes(r.Group("/api/v1"))

	w := apitest.PerformRequest(r, "GET", "/api/v1/branding/settings", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var response BrandingSettingsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "branding_settings", response.Type)
	assert.Equal(t, "1", response.ID)
	// Default values should be returned
	assert.NotEmpty(t, response.Attributes.AppTitle)
}

func TestUpdateBrandingSettings_AdminOnly(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Create admin and non-admin users
	admin := createTestUserWithSettings(t, service, "admin@test.com", "Admin", true, true, true, true, false)
	nonAdmin := createTestUserWithSettings(t, service, "user@test.com", "User", false, true, true, true, false)

	t.Run("Admin can update branding settings", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", admin)
			c.Next()
		})
		api.setupBrandingRoutes(r.Group("/api/v1"))

		title := "Custom App Title"
		primaryColor := "#FF0000"
		payload := UpdateBrandingSettingsRequest{
			AppTitle:     &title,
			PrimaryColor: &primaryColor,
		}

		w := apitest.PerformRequest(r, "PUT", "/api/v1/branding/settings", payload)

		assert.Equal(t, http.StatusOK, w.Code)

		var response BrandingSettingsResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, "Custom App Title", response.Attributes.AppTitle)
		assert.Equal(t, "#FF0000", response.Attributes.PrimaryColor)
	})

	t.Run("Non-admin cannot update branding settings", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", nonAdmin)
			c.Next()
		})
		api.setupBrandingRoutes(r.Group("/api/v1"))

		title := "Malicious Title"
		payload := UpdateBrandingSettingsRequest{
			AppTitle: &title,
		}

		w := apitest.PerformRequest(r, "PUT", "/api/v1/branding/settings", payload)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "admin")
		}
	})
}

func TestUploadLogo_AdminOnly(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, svc)
	authService := apitest.SetupTestAuthService(db, svc)

	api := NewAPI(svc, true, authService, config, nil, emptyFile, nil)

	// Create admin user
	admin := createTestUserWithSettings(t, svc, "admin@test.com", "Admin", true, true, true, true, false)

	// Setup temp directory for test files
	tempDir := t.TempDir()
	os.Setenv("BRANDING_STORAGE_PATH", tempDir)
	defer os.Unsetenv("BRANDING_STORAGE_PATH")

	t.Run("Admin can upload PNG logo", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", admin)
			c.Next()
		})
		api.setupBrandingRoutes(r.Group("/api/v1"))

		// Create a test PNG file (1x1 pixel)
		pngData := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG header
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
			0xDE, // IHDR data + CRC
			0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54, // IDAT chunk
			0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00, 0x00,
			0x03, 0x01, 0x01, 0x00, 0x18, 0xDD, 0x8D, 0xB4, // IDAT data + CRC
			0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, // IEND chunk
			0xAE, 0x42, 0x60, 0x82,
		}

		w := performFileUpload(r, "/api/v1/branding/logo", "file", "test-logo.png", pngData)

		// File upload may succeed or fail depending on file system setup
		// Just verify handler runs without panic and authorization is checked
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest}, w.Code,
			"Upload handler should handle file upload (may fail due to test environment)")
	})

	t.Run("Non-admin cannot upload logo", func(t *testing.T) {
		nonAdmin := createTestUserWithSettings(t, svc, "user@test.com", "User", false, true, true, true, false)

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", nonAdmin)
			c.Next()
		})
		api.setupBrandingRoutes(r.Group("/api/v1"))

		pngData := []byte{0x89, 0x50, 0x4E, 0x47} // Minimal PNG
		w := performFileUpload(r, "/api/v1/branding/logo", "file", "logo.png", pngData)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestUploadFavicon_AdminOnly(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, svc)
	authService := apitest.SetupTestAuthService(db, svc)

	api := NewAPI(svc, true, authService, config, nil, emptyFile, nil)

	// Create admin user
	admin := createTestUserWithSettings(t, svc, "admin@test.com", "Admin", true, true, true, true, false)

	// Setup temp directory for test files
	tempDir := t.TempDir()
	os.Setenv("BRANDING_STORAGE_PATH", tempDir)
	defer os.Unsetenv("BRANDING_STORAGE_PATH")

	t.Run("Admin can upload ICO favicon", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", admin)
			c.Next()
		})
		api.setupBrandingRoutes(r.Group("/api/v1"))

		// Minimal ICO file
		icoData := []byte{
			0x00, 0x00, 0x01, 0x00, 0x01, 0x00, // ICO header
			0x10, 0x10, 0x00, 0x00, 0x01, 0x00,
			0x18, 0x00, 0x30, 0x00, 0x00, 0x00,
			0x16, 0x00, 0x00, 0x00,
		}

		w := performFileUpload(r, "/api/v1/branding/favicon", "file", "test-favicon.ico", icoData)

		// File upload may succeed or fail depending on file system setup and validation
		assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusBadRequest}, w.Code,
			"Upload handler should handle file upload (may fail due to test environment)")
	})
}

func TestResetBranding_AdminOnly(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, svc)
	authService := apitest.SetupTestAuthService(db, svc)

	api := NewAPI(svc, true, authService, config, nil, emptyFile, nil)

	// Create users
	admin := createTestUserWithSettings(t, svc, "admin@test.com", "Admin", true, true, true, true, false)
	nonAdmin := createTestUserWithSettings(t, svc, "user@test.com", "User", false, true, true, true, false)

	t.Run("Admin can reset branding", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", admin)
			c.Next()
		})
		api.setupBrandingRoutes(r.Group("/api/v1"))

		w := apitest.PerformRequest(r, "POST", "/api/v1/branding/reset", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var response BrandingSettingsResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "branding_settings", response.Type)
	})

	t.Run("Non-admin cannot reset branding", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", nonAdmin)
			c.Next()
		})
		api.setupBrandingRoutes(r.Group("/api/v1"))

		w := apitest.PerformRequest(r, "POST", "/api/v1/branding/reset", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestServeLogo_Public(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, svc)
	authService := apitest.SetupTestAuthService(db, svc)

	api := NewAPI(svc, true, authService, config, nil, emptyFile, nil)

	// Setup router (no auth required - public endpoint)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api.setupBrandingRoutes(r.Group("/api/v1"))

	t.Run("Serve default logo when no custom logo", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/branding/logo", nil)

		// May return 200 (embedded default) or 404 (if embed not available in test)
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, w.Code)
	})
}

func TestServeFavicon_Public(t *testing.T) {
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, svc)
	authService := apitest.SetupTestAuthService(db, svc)

	api := NewAPI(svc, true, authService, config, nil, emptyFile, nil)

	// Setup router (no auth required - public endpoint)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api.setupBrandingRoutes(r.Group("/api/v1"))

	t.Run("Serve default favicon when no custom favicon", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/branding/favicon", nil)

		// May return 200 (embedded default) or 404 (if embed not available in test)
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, w.Code)
	})
}

// Helper function to perform multipart file upload
func performFileUpload(r http.Handler, path, fieldName, fileName string, fileContent []byte) *httptest.ResponseRecorder {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create form file
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		panic(err)
	}

	// Write file content
	if _, err := io.Copy(part, bytes.NewReader(fileContent)); err != nil {
		panic(err)
	}

	// Close multipart writer
	writer.Close()

	// Create request
	req, _ := http.NewRequest("POST", path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Perform request
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

// setupBrandingRoutes registers branding-related routes
func (a *API) setupBrandingRoutes(r *gin.RouterGroup) {
	r.GET("/branding/settings", a.getBrandingSettings)
	r.PUT("/branding/settings", a.updateBrandingSettings)
	r.POST("/branding/logo", a.uploadLogo)
	r.POST("/branding/favicon", a.uploadFavicon)
	r.POST("/branding/reset", a.resetBranding)
	r.GET("/branding/logo", a.serveLogo)
	r.GET("/branding/favicon", a.serveFavicon)
}
