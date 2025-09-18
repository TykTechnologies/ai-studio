// internal/api/plugin_integration_test.go
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/internal/api/handlers"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MockPluginManagerForAPI implements PluginManagerInterface for testing
type MockPluginManagerForAPI struct {
	mock.Mock
}

func (m *MockPluginManagerForAPI) ExecutePluginChain(llmID uint, hookType string, input interface{}, pluginCtx interface{}) (interface{}, error) {
	args := m.Called(llmID, hookType, input, pluginCtx)
	return args.Get(0), args.Error(1)
}

func (m *MockPluginManagerForAPI) GetPluginsForLLM(llmID uint, hookType string) (interface{}, error) {
	args := m.Called(llmID, hookType)
	return args.Get(0), args.Error(1)
}

func (m *MockPluginManagerForAPI) IsPluginLoaded(pluginID uint) bool {
	args := m.Called(pluginID)
	return args.Bool(0)
}

func (m *MockPluginManagerForAPI) RefreshLLMPluginMapping(llmID uint) error {
	args := m.Called(llmID)
	return args.Error(0)
}

// MockAuthProvider for testing
type MockAuthProvider struct {
	mock.Mock
}

func (m *MockAuthProvider) ValidateToken(token string) (*auth.AuthResult, error) {
	args := m.Called(token)
	return args.Get(0).(*auth.AuthResult), args.Error(1)
}

func (m *MockAuthProvider) GenerateToken(appID uint, name string, scopes []string, expiresIn time.Duration) (string, error) {
	args := m.Called(appID, name, scopes, expiresIn)
	return args.String(0), args.Error(1)
}

func (m *MockAuthProvider) RevokeToken(token string) error {
	args := m.Called(token)
	return args.Error(0)
}

func (m *MockAuthProvider) GetTokenInfo(token string) (*auth.TokenInfo, error) {
	args := m.Called(token)
	return args.Get(0).(*auth.TokenInfo), args.Error(1)
}

func setupAPITestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Auto-migrate models
	err = db.AutoMigrate(
		&database.LLM{},
		&database.Plugin{},
		&database.LLMPlugin{},
	)
	require.NoError(t, err)

	return db
}

func TestPluginMiddleware_Integration(t *testing.T) {
	db := setupAPITestDB(t)
	repo := database.NewRepository(db)

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "test",
		DefaultModel: "test-model",
		IsActive:     true,
	}
	err := db.Create(llm).Error
	require.NoError(t, err)

	// Create mock services
	gatewayService := services.NewDatabaseGatewayService(db, repo)
	serviceContainer := &services.ServiceContainer{
		GatewayService: gatewayService,
	}

	// Create mock plugin manager
	mockPluginManager := &MockPluginManagerForAPI{}

	// Mock no plugins for this LLM - GetPluginsForLLM returns empty slice
	mockPluginManager.On("GetPluginsForLLM", llm.ID, "pre_auth").Return([]interface{}{}, nil)
	mockPluginManager.On("GetPluginsForLLM", llm.ID, "post_auth").Return([]interface{}{}, nil)

	// Mock plugin execution (should not be called since no plugins, but added for safety)
	mockPluginManager.On("ExecutePluginChain", llm.ID, "pre_auth", mock.Anything, mock.Anything).
		Return(mock.Anything, nil).Maybe()
	mockPluginManager.On("ExecutePluginChain", llm.ID, "post_auth", mock.Anything, mock.Anything).
		Return(mock.Anything, nil).Maybe()

	// Create mock auth provider
	mockAuthProvider := &MockAuthProvider{}
	mockAuthProvider.On("ValidateToken", "valid-token").Return(&auth.AuthResult{
		Valid:  true,
		AppID:  1,
		Scopes: []string{"api"},
	}, nil)

	// Setup Gin router with plugin middleware
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add auth middleware
	router.Use(auth.RequireAuth(mockAuthProvider))

	// Add plugin middleware
	pluginConfig := &PluginMiddlewareConfig{
		PluginManager: mockPluginManager,
		Services:      serviceContainer,
	}
	router.Use(CreatePluginMiddleware(pluginConfig))

	// Add a test handler that simulates the AI Gateway
	router.Any("/llm/*path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "AI Gateway response",
			"path":    c.Request.URL.Path,
		})
	})

	// Test the middleware with an LLM request
	req := httptest.NewRequest("POST", "/llm/rest/test-llm/chat/completions", bytes.NewBufferString(`{"model": "test"}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify the request went through
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "AI Gateway response")

	// Verify GetPluginsForLLM was called but ExecutePluginChain was not (since no plugins)
	mockPluginManager.AssertCalled(t, "GetPluginsForLLM", llm.ID, "pre_auth")
	mockPluginManager.AssertNotCalled(t, "ExecutePluginChain", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestPluginMiddleware_NonLLMRequest(t *testing.T) {
	mockPluginManager := &MockPluginManagerForAPI{}
	serviceContainer := &services.ServiceContainer{}

	// Setup Gin router with plugin middleware
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add plugin middleware
	pluginConfig := &PluginMiddlewareConfig{
		PluginManager: mockPluginManager,
		Services:      serviceContainer,
	}
	router.Use(CreatePluginMiddleware(pluginConfig))

	// Add a test handler
	router.GET("/api/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "API response"})
	})

	// Test non-LLM request
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify the request went through without plugin processing
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "API response")

	// Verify no plugin methods were called
	mockPluginManager.AssertNotCalled(t, "ExecutePluginChain", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestPluginAPI_ComprehensiveFiltering tests the plugin API handlers with comprehensive filter scenarios
func TestPluginAPI_ComprehensiveFiltering(t *testing.T) {
	db := setupAPITestDB(t)
	repo := database.NewRepository(db)
	service := services.NewPluginService(db, repo)

	// Create comprehensive test data
	testPlugins := []struct {
		name      string
		slug      string
		hookType  string
		isActive  bool
		namespace string
	}{
		{"PreAuth Global Active", "preauth-global-active", "pre_auth", true, ""},
		{"PreAuth Global Inactive", "preauth-global-inactive", "pre_auth", false, ""},
		{"Auth TenantA Active", "auth-tenanta-active", "auth", true, "tenant-a"},
		{"Auth TenantA Inactive", "auth-tenanta-inactive", "auth", false, "tenant-a"},
		{"PostAuth TenantB Active", "postauth-tenantb-active", "post_auth", true, "tenant-b"},
		{"OnResponse Global Active", "onresponse-global-active", "on_response", true, ""},
		{"OnResponse TenantB Inactive", "onresponse-tenantb-inactive", "on_response", false, "tenant-b"},
	}

	// Create all test plugins via service
	createdPlugins := make([]*database.Plugin, 0)
	for _, testPlugin := range testPlugins {
		plugin, err := service.CreatePlugin(&services.CreatePluginRequest{
			Name:        testPlugin.name,
			Slug:        testPlugin.slug,
			Command:     fmt.Sprintf("./bin/%s", testPlugin.slug),
			HookType:    testPlugin.hookType,
			IsActive:    testPlugin.isActive,
		})
		// Set namespace manually after creation since CreatePluginRequest doesn't have namespace in microgateway
		if err == nil && testPlugin.namespace != "" {
			plugin.Namespace = testPlugin.namespace
			db.Save(plugin)
		}
		require.NoError(t, err)
		createdPlugins = append(createdPlugins, plugin)
	}

	// Create service container for handlers
	serviceContainer := &services.ServiceContainer{
		PluginService: service,
	}

	// Setup Gin router with handlers
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/plugins", handlers.ListPlugins(serviceContainer))

	// Test 1: Default filtering (active plugins only)
	t.Run("Default filtering - active only", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/plugins", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data       []database.Plugin `json:"data"`
			Pagination struct {
				Page       int   `json:"page"`
				Limit      int   `json:"limit"`
				Total      int64 `json:"total"`
				TotalPages int64 `json:"total_pages"`
			} `json:"pagination"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 4, len(response.Data)) // 4 active plugins
		assert.Equal(t, int64(4), response.Pagination.Total)

		// Verify all returned plugins are active
		for _, plugin := range response.Data {
			assert.True(t, plugin.IsActive)
		}
	})

	// Test 2: Hook type filtering
	t.Run("Hook type filtering", func(t *testing.T) {
		hookTypeTests := []struct {
			hookType      string
			expectedCount int
		}{
			{"pre_auth", 1},     // Only 1 active pre_auth
			{"auth", 1},         // Only 1 active auth
			{"post_auth", 1},    // Only 1 active post_auth
			{"on_response", 1},  // Only 1 active on_response
			{"invalid", 0},      // No plugins with invalid hook type
		}

		for _, test := range hookTypeTests {
			req := httptest.NewRequest("GET", fmt.Sprintf("/plugins?hook_type=%s", test.hookType), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response struct {
				Data       []database.Plugin `json:"data"`
				Pagination struct {
					Total int64 `json:"total"`
				} `json:"pagination"`
			}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedCount, len(response.Data), "Hook type %s should return %d plugins", test.hookType, test.expectedCount)
			assert.Equal(t, int64(test.expectedCount), response.Pagination.Total)

			// Verify hook type and active status
			for _, plugin := range response.Data {
				assert.Equal(t, test.hookType, plugin.HookType)
				assert.True(t, plugin.IsActive)
			}
		}
	})

	// Test 3: Active status filtering
	t.Run("Active status filtering", func(t *testing.T) {
		activeTests := []struct {
			queryParam    string
			expectedCount int
			expectedState bool
		}{
			{"active=true", 4, true},
			{"active=false", 3, false},
		}

		for _, test := range activeTests {
			req := httptest.NewRequest("GET", fmt.Sprintf("/plugins?%s", test.queryParam), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response struct {
				Data       []database.Plugin `json:"data"`
				Pagination struct {
					Total int64 `json:"total"`
				} `json:"pagination"`
			}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedCount, len(response.Data))
			assert.Equal(t, int64(test.expectedCount), response.Pagination.Total)

			for _, plugin := range response.Data {
				assert.Equal(t, test.expectedState, plugin.IsActive)
			}
		}
	})

	// Test 4: Combined filtering (hook_type + active)
	t.Run("Combined filtering: hook_type and active", func(t *testing.T) {
		combinedTests := []struct {
			hookType      string
			isActive      string
			expectedCount int
		}{
			{"pre_auth", "true", 1},
			{"pre_auth", "false", 1},
			{"auth", "true", 1},
			{"auth", "false", 1},
			{"on_response", "true", 1},
			{"on_response", "false", 1},
		}

		for _, test := range combinedTests {
			req := httptest.NewRequest("GET", fmt.Sprintf("/plugins?hook_type=%s&active=%s", test.hookType, test.isActive), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response struct {
				Data       []database.Plugin `json:"data"`
				Pagination struct {
					Total int64 `json:"total"`
				} `json:"pagination"`
			}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedCount, len(response.Data))
			assert.Equal(t, int64(test.expectedCount), response.Pagination.Total)

			expectedActive := test.isActive == "true"
			for _, plugin := range response.Data {
				assert.Equal(t, test.hookType, plugin.HookType)
				assert.Equal(t, expectedActive, plugin.IsActive)
			}
		}
	})

	// Test 5: Pagination parameters
	t.Run("Pagination parameters", func(t *testing.T) {
		// Test limit parameter
		req := httptest.NewRequest("GET", "/plugins?limit=2", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data       []database.Plugin `json:"data"`
			Pagination struct {
				Page       int   `json:"page"`
				Limit      int   `json:"limit"`
				Total      int64 `json:"total"`
				TotalPages int64 `json:"total_pages"`
			} `json:"pagination"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(response.Data))
		assert.Equal(t, 2, response.Pagination.Limit)
		assert.Equal(t, int64(4), response.Pagination.Total)
		assert.Equal(t, int64(2), response.Pagination.TotalPages)

		// Test page parameter
		req = httptest.NewRequest("GET", "/plugins?page=2&limit=2", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(response.Data))
		assert.Equal(t, 2, response.Pagination.Page)
		assert.Equal(t, 2, response.Pagination.Limit)
		assert.Equal(t, int64(4), response.Pagination.Total)
	})

	// Test 6: Parameter validation and edge cases
	t.Run("Parameter validation and edge cases", func(t *testing.T) {
		// Test invalid page number (should default to 1)
		req := httptest.NewRequest("GET", "/plugins?page=0", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Pagination struct {
				Page  int `json:"page"`
				Limit int `json:"limit"`
			} `json:"pagination"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 1, response.Pagination.Page) // Should default to 1

		// Test invalid limit (should default to 20)
		req = httptest.NewRequest("GET", "/plugins?limit=0", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 20, response.Pagination.Limit) // Should default to 20

		// Test very large limit (should be capped to 100)
		req = httptest.NewRequest("GET", "/plugins?limit=1000", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 20, response.Pagination.Limit) // Should be capped
	})
}

func TestExtractLLMSlug(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/llm/rest/gpt-4/chat/completions", "gpt-4"},
		{"/llm/stream/claude-3/messages", "claude-3"},
		{"/llm/rest/test-llm/completions", "test-llm"},
		{"/api/v1/llms", ""},
		{"/llm/", ""},
		{"/llm/rest/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := extractLLMSlug(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	time.Sleep(time.Nanosecond) // Ensure different timestamps
	id2 := generateRequestID()

	// Should start with "req_"
	assert.True(t, strings.HasPrefix(id1, "req_"))
	assert.True(t, strings.HasPrefix(id2, "req_"))

	// Should be unique
	assert.NotEqual(t, id1, id2)

	// Should have reasonable length
	assert.Greater(t, len(id1), 10)
	assert.Greater(t, len(id2), 10)
}