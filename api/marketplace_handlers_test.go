package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMarketplaceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Auto-migrate marketplace tables
	err = db.AutoMigrate(
		&models.MarketplacePlugin{},
		&models.MarketplaceIndex{},
		&models.InstalledPluginVersion{},
		&models.MarketplaceConfig{},
		&models.Plugin{},
	)
	assert.NoError(t, err)

	return db
}

func seedMarketplaceTestData(t *testing.T, db *gorm.DB) {
	// Create test marketplace index
	index := &models.MarketplaceIndex{
		SourceURL:    "https://example.com/index.yaml",
		IsDefault:    true,
		IsActive:     true,
		SyncStatus:   "success",
		PluginCount:  3,
		LastSynced:   time.Now(),
	}
	assert.NoError(t, db.Create(index).Error)

	// Create test marketplace plugins
	plugins := []*models.MarketplacePlugin{
		{
			PluginID:         "com.tyk.echo-agent",
			Version:          "1.0.0",
			Name:             "Echo Agent",
			Description:      "A simple echo agent for testing",
			Category:         "agents",
			Publisher:        "tyk-official",
			Maturity:         "stable",
			OCIRegistry:      "ghcr.io",
			OCIRepository:    "tyk/plugins/echo-agent",
			OCITag:           "1.0.0",
			OCIDigest:        "sha256:abc123",
			OCIPlatforms:     []string{"linux/amd64", "darwin/amd64"},
			PrimaryHook:      "agent",
			Hooks:            []string{"agent"},
			RequiredServices: []string{"llms.proxy"},
			IconURL:          "https://example.com/icon.png",
			PluginCreatedAt:  time.Now().Add(-30 * 24 * time.Hour),
			PluginUpdatedAt:  time.Now().Add(-1 * 24 * time.Hour),
			Deprecated:       false,
			LastSynced:       time.Now(),
			SyncedFromURL:    "https://example.com/index.yaml",
		},
		{
			PluginID:         "com.tyk.echo-agent",
			Version:          "0.9.0",
			Name:             "Echo Agent",
			Description:      "A simple echo agent for testing (older version)",
			Category:         "agents",
			Publisher:        "tyk-official",
			Maturity:         "stable",
			OCIRegistry:      "ghcr.io",
			OCIRepository:    "tyk/plugins/echo-agent",
			OCITag:           "0.9.0",
			OCIDigest:        "sha256:def456",
			OCIPlatforms:     []string{"linux/amd64"},
			PrimaryHook:      "agent",
			Hooks:            []string{"agent"},
			RequiredServices: []string{"llms.proxy"},
			IconURL:          "https://example.com/icon.png",
			PluginCreatedAt:  time.Now().Add(-30 * 24 * time.Hour),
			PluginUpdatedAt:  time.Now().Add(-7 * 24 * time.Hour),
			Deprecated:       false,
			LastSynced:       time.Now(),
			SyncedFromURL:    "https://example.com/index.yaml",
		},
		{
			PluginID:         "com.tyk.slack-connector",
			Version:          "2.0.0",
			Name:             "Slack Connector",
			Description:      "Connect to Slack",
			Category:         "connectors",
			Publisher:        "tyk-verified",
			Maturity:         "beta",
			OCIRegistry:      "ghcr.io",
			OCIRepository:    "tyk/plugins/slack-connector",
			OCITag:           "2.0.0",
			OCIDigest:        "sha256:ghi789",
			OCIPlatforms:     []string{"linux/amd64", "darwin/amd64", "darwin/arm64"},
			PrimaryHook:      "tool",
			Hooks:            []string{"tool"},
			RequiredServices: []string{"tools.call"},
			IconURL:          "https://example.com/slack-icon.png",
			PluginCreatedAt:  time.Now().Add(-60 * 24 * time.Hour),
			PluginUpdatedAt:  time.Now().Add(-2 * 24 * time.Hour),
			Deprecated:       false,
			LastSynced:       time.Now(),
			SyncedFromURL:    "https://example.com/index.yaml",
		},
		{
			PluginID:         "com.tyk.old-plugin",
			Version:          "1.0.0",
			Name:             "Old Plugin",
			Description:      "Deprecated plugin",
			Category:         "agents",
			Publisher:        "community",
			Maturity:         "alpha",
			OCIRegistry:      "ghcr.io",
			OCIRepository:    "tyk/plugins/old-plugin",
			OCITag:           "1.0.0",
			OCIDigest:        "sha256:jkl012",
			PrimaryHook:      "agent",
			Deprecated:       true,
			DeprecatedMessage: "This plugin is no longer maintained",
			ReplacementPlugin: "com.tyk.echo-agent",
			LastSynced:       time.Now(),
			SyncedFromURL:    "https://example.com/index.yaml",
		},
	}

	for _, plugin := range plugins {
		assert.NoError(t, db.Create(plugin).Error)
	}
}

func setupMarketplaceService(t *testing.T, db *gorm.DB) *services.MarketplaceService {
	// Create a minimal marketplace service for testing
	return services.NewMarketplaceService(
		db,
		nil, // ociClient - not needed for handler tests
		nil, // pluginService - not needed for handler tests
		nil, // pluginManager - not needed for handler tests
		"./.test-marketplace-cache",
		"https://example.com/index.yaml",
		1*time.Hour,
	)
}

func TestListPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name:           "list all plugins",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.NotNil(t, resp["plugins"])
				assert.NotNil(t, resp["total"])
			},
		},
		{
			name:           "filter by category",
			queryParams:    "?category=agents",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.NotNil(t, resp["plugins"])
			},
		},
		{
			name:           "filter by publisher",
			queryParams:    "?publisher=tyk-official",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.NotNil(t, resp["plugins"])
			},
		},
		{
			name:           "search by name",
			queryParams:    "?search=echo",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.NotNil(t, resp["plugins"])
			},
		},
		{
			name:           "exclude deprecated",
			queryParams:    "?include_deprecated=false",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				plugins := resp["plugins"].([]interface{})
				for _, p := range plugins {
					plugin := p.(map[string]interface{})
					assert.False(t, plugin["deprecated"].(bool))
				}
			},
		},
		{
			name:           "pagination",
			queryParams:    "?page=1&page_size=2",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, float64(1), resp["page"])
				assert.Equal(t, float64(2), resp["page_size"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/v1/marketplace/plugins"+tt.queryParams, nil)

			handlers.ListPlugins(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil && w.Code == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestGetPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	tests := []struct {
		name           string
		pluginID       string
		version        string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name:           "get latest version",
			pluginID:       "com.tyk.echo-agent",
			version:        "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "com.tyk.echo-agent", resp["plugin_id"])
				assert.Equal(t, "1.0.0", resp["version"])
			},
		},
		{
			name:           "get specific version",
			pluginID:       "com.tyk.echo-agent",
			version:        "0.9.0",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "com.tyk.echo-agent", resp["plugin_id"])
				assert.Equal(t, "0.9.0", resp["version"])
			},
		},
		{
			name:           "plugin not found",
			pluginID:       "com.tyk.nonexistent",
			version:        "",
			expectedStatus: http.StatusNotFound,
			checkResponse:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/api/v1/marketplace/plugins/" + tt.pluginID
			if tt.version != "" {
				url += "?version=" + tt.version
			}
			c.Request = httptest.NewRequest("GET", url, nil)
			c.Params = gin.Params{{Key: "id", Value: tt.pluginID}}

			handlers.GetPlugin(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil && w.Code == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestGetInstallMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	tests := []struct {
		name           string
		pluginID       string
		version        string
		expectedStatus int
		checkResponse  func(*testing.T, map[string]interface{})
	}{
		{
			name:           "get install metadata for latest version",
			pluginID:       "com.tyk.echo-agent",
			version:        "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.Equal(t, "com.tyk.echo-agent", resp["plugin_id"])
				assert.Equal(t, "Echo Agent", resp["name"])
				assert.Contains(t, resp["oci_reference"], "oci://ghcr.io/tyk/plugins/echo-agent")
				assert.Equal(t, "agent", resp["hook_type"])
				assert.True(t, resp["is_agent"].(bool))

				// Check required scopes
				scopes := resp["required_scopes"].([]interface{})
				assert.Contains(t, scopes, "llms.proxy")
			},
		},
		{
			name:           "get install metadata with digest",
			pluginID:       "com.tyk.echo-agent",
			version:        "1.0.0",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				ociRef := resp["oci_reference"].(string)
				assert.Contains(t, ociRef, "@sha256:abc123")
			},
		},
		{
			name:           "connector plugin is not an agent",
			pluginID:       "com.tyk.slack-connector",
			version:        "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp map[string]interface{}) {
				assert.False(t, resp["is_agent"].(bool))
				assert.Equal(t, "tool", resp["hook_type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/api/v1/marketplace/plugins/" + tt.pluginID + "/install-metadata"
			if tt.version != "" {
				url += "?version=" + tt.version
			}
			c.Request = httptest.NewRequest("GET", url, nil)
			c.Params = gin.Params{{Key: "id", Value: tt.pluginID}}

			handlers.GetInstallMetadata(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil && w.Code == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				tt.checkResponse(t, response)
			}
		})
	}
}

func TestGetPluginVersions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/marketplace/plugins/com.tyk.echo-agent/versions", nil)
	c.Params = gin.Params{{Key: "id", Value: "com.tyk.echo-agent"}}

	handlers.GetPluginVersions(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "com.tyk.echo-agent", response["plugin_id"])

	versions := response["versions"].([]interface{})
	assert.GreaterOrEqual(t, len(versions), 2) // Should have 1.0.0 and 0.9.0
}

func TestGetCategories(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/marketplace/categories", nil)

	handlers.GetCategories(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	categories := response["categories"].([]interface{})
	assert.Contains(t, categories, "agents")
	assert.Contains(t, categories, "connectors")
}

func TestGetPublishers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/marketplace/publishers", nil)

	handlers.GetPublishers(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	publishers := response["publishers"].([]interface{})
	assert.Contains(t, publishers, "tyk-official")
	assert.Contains(t, publishers, "tyk-verified")
	assert.Contains(t, publishers, "community")
}

func TestGetStats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/marketplace/stats", nil)

	handlers.GetStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.NotNil(t, response["total_plugins"])
	assert.NotNil(t, response["total_versions"])
	assert.NotNil(t, response["deprecated_plugins"])
	assert.NotNil(t, response["category_breakdown"])
	assert.NotNil(t, response["publisher_breakdown"])
}

func TestSyncMarketplace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/marketplace/sync", nil)

	handlers.SyncMarketplace(c)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Marketplace sync initiated", response["message"])
	assert.Equal(t, "in_progress", response["status"])
}

func TestGetSyncStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMarketplaceTestDB(t)
	seedMarketplaceTestData(t, db)

	marketplaceService := setupMarketplaceService(t, db)
	handlers := NewMarketplaceHandlers(marketplaceService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/marketplace/sync-status", nil)

	handlers.GetSyncStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	indexes := response["indexes"].([]interface{})
	assert.GreaterOrEqual(t, len(indexes), 1)

	firstIndex := indexes[0].(map[string]interface{})
	assert.Equal(t, "https://example.com/index.yaml", firstIndex["source_url"])
	assert.Equal(t, "success", firstIndex["sync_status"])
}
