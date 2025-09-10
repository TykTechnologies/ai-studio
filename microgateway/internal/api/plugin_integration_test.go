// internal/api/plugin_integration_test.go
package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
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
	
	// Mock no plugins for this LLM (plugins would return empty)
	mockPluginManager.On("ExecutePluginChain", llm.ID, "pre_auth", mock.Anything, mock.Anything).
		Return(mock.Anything, nil)
	mockPluginManager.On("ExecutePluginChain", llm.ID, "on_response", mock.Anything, mock.Anything).
		Return(mock.Anything, nil)

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

	// Verify plugin chain was called
	mockPluginManager.AssertCalled(t, "ExecutePluginChain", llm.ID, "pre_auth", mock.Anything, mock.Anything)
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