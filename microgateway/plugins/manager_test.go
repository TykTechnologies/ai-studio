// plugins/manager_test.go
package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockPluginService implements the PluginServiceInterface for testing
type MockPluginService struct {
	mock.Mock
}

func (m *MockPluginService) CreatePlugin(req *services.CreatePluginRequest) (*database.Plugin, error) {
	args := m.Called(req)
	return args.Get(0).(*database.Plugin), args.Error(1)
}

func (m *MockPluginService) GetPlugin(id uint) (*database.Plugin, error) {
	args := m.Called(id)
	return args.Get(0).(*database.Plugin), args.Error(1)
}

func (m *MockPluginService) ListPlugins(page, limit int, hookType string, isActive bool) ([]database.Plugin, int64, error) {
	args := m.Called(page, limit, hookType, isActive)
	return args.Get(0).([]database.Plugin), args.Get(1).(int64), args.Error(2)
}

func (m *MockPluginService) UpdatePlugin(id uint, req *services.UpdatePluginRequest) (*database.Plugin, error) {
	args := m.Called(id, req)
	return args.Get(0).(*database.Plugin), args.Error(1)
}

func (m *MockPluginService) DeletePlugin(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockPluginService) GetPluginsForLLM(llmID uint) ([]database.Plugin, error) {
	args := m.Called(llmID)
	return args.Get(0).([]database.Plugin), args.Error(1)
}

func (m *MockPluginService) UpdateLLMPlugins(llmID uint, pluginIDs []uint) error {
	args := m.Called(llmID, pluginIDs)
	return args.Error(0)
}

func (m *MockPluginService) GetLLMPluginConfig(llmID, pluginID uint) (map[string]interface{}, error) {
	args := m.Called(llmID, pluginID)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockPluginService) ValidatePluginChecksum(pluginID uint, filePath string) error {
	args := m.Called(pluginID, filePath)
	return args.Error(0)
}

func (m *MockPluginService) TestPlugin(pluginID uint, testData interface{}) (interface{}, error) {
	args := m.Called(pluginID, testData)
	return args.Get(0), args.Error(1)
}

func (m *MockPluginService) PluginSlugExists(slug string) (bool, error) {
	args := m.Called(slug)
	return args.Bool(0), args.Error(1)
}


func TestNewPluginManager(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	assert.NotNil(t, manager)
	assert.Equal(t, mockService, manager.service)
	assert.NotNil(t, manager.loadedPlugins)
	assert.NotNil(t, manager.llmPluginMap)
	assert.NotNil(t, manager.pluginClients)
	assert.NotNil(t, manager.reattachConfigs)
}

func TestPluginManager_LoadPlugin_Inactive(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	// Mock inactive plugin
	inactivePlugin := &database.Plugin{
		ID:       1,
		Name:     "Test Plugin",
		Slug:     "test-plugin",
		Command:  "./test-plugin",
		HookType: "pre_auth",
		IsActive: false,
	}

	mockService.On("GetPlugin", uint(1)).Return(inactivePlugin, nil)

	_, err := manager.LoadPlugin(1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin 1 is not active")

	mockService.AssertExpectations(t)
}

func TestPluginManager_UnloadPlugin_NotLoaded(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	err := manager.UnloadPlugin(999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin 999 is not loaded")
}

func TestPluginManager_IsPluginLoaded(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	// Initially no plugins loaded
	assert.False(t, manager.IsPluginLoaded(1))

	// Add a mock loaded plugin
	manager.loadedPlugins[1] = &LoadedPlugin{
		ID:   1,
		Name: "Test Plugin",
	}

	assert.True(t, manager.IsPluginLoaded(1))
}

func TestPluginManager_GetLoadedPlugins(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	// Initially empty
	plugins := manager.GetLoadedPlugins()
	assert.Empty(t, plugins)

	// Add mock loaded plugins
	manager.loadedPlugins[1] = &LoadedPlugin{ID: 1, Name: "Plugin 1"}
	manager.loadedPlugins[2] = &LoadedPlugin{ID: 2, Name: "Plugin 2"}

	plugins = manager.GetLoadedPlugins()
	assert.Len(t, plugins, 2)
}

func TestPluginManager_RefreshLLMPluginMapping(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	llmID := uint(1)
	mockPlugins := []database.Plugin{
		{ID: 1, Name: "Plugin 1", HookType: "pre_auth", IsActive: true},
		{ID: 2, Name: "Plugin 2", HookType: "auth", IsActive: true},
	}

	mockService.On("GetPluginsForLLM", llmID).Return(mockPlugins, nil)

	err := manager.RefreshLLMPluginMapping(llmID)
	assert.NoError(t, err)

	// Check that mapping was created
	manager.mu.RLock()
	pluginIDs := manager.llmPluginMap[llmID]
	manager.mu.RUnlock()

	assert.Len(t, pluginIDs, 2)
	assert.Contains(t, pluginIDs, uint(1))
	assert.Contains(t, pluginIDs, uint(2))

	mockService.AssertExpectations(t)
}

func TestPluginManager_RefreshLLMPluginMapping_Empty(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	llmID := uint(1)
	var mockPlugins []database.Plugin // Empty slice

	mockService.On("GetPluginsForLLM", llmID).Return(mockPlugins, nil)

	err := manager.RefreshLLMPluginMapping(llmID)
	assert.NoError(t, err)

	// Check that no mapping was created
	manager.mu.RLock()
	_, exists := manager.llmPluginMap[llmID]
	manager.mu.RUnlock()

	assert.False(t, exists)

	mockService.AssertExpectations(t)
}

func TestPluginManager_Shutdown(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	// Add some mock loaded plugins
	manager.loadedPlugins[1] = &LoadedPlugin{
		ID:         1,
		Name:       "Plugin 1",
		IsHealthy:  true,
		Client:     nil, // Mock client would be nil for this test
		GRPCClient: nil,
	}
	manager.loadedPlugins[2] = &LoadedPlugin{
		ID:         2,
		Name:       "Plugin 2",
		IsHealthy:  true,
		Client:     nil,
		GRPCClient: nil,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := manager.Shutdown(ctx)
	assert.NoError(t, err)

	// Check that all maps are cleared
	assert.Empty(t, manager.loadedPlugins)
	assert.Empty(t, manager.pluginClients)
	assert.Empty(t, manager.reattachConfigs)
	assert.Empty(t, manager.llmPluginMap)
}

func TestPluginManager_GetPluginsForLLM_NoPlugins(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	llmID := uint(1)
	hookType := interfaces.HookTypePreAuth
	var mockPlugins []database.Plugin // Empty slice

	mockService.On("GetPluginsForLLM", llmID).Return(mockPlugins, nil)

	plugins, err := manager.GetPluginsForLLM(llmID, hookType)
	assert.NoError(t, err)
	assert.Empty(t, plugins)

	mockService.AssertExpectations(t)
}

func TestPluginManager_ExecutePluginChain_NoPlugins(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	llmID := uint(1)
	hookType := interfaces.HookTypePreAuth
	input := &interfaces.PluginRequest{Method: "GET", Path: "/test"}
	ctx := &interfaces.PluginContext{RequestID: "test-123"}

	var mockPlugins []database.Plugin // Empty slice
	mockService.On("GetPluginsForLLM", llmID).Return(mockPlugins, nil)

	result, err := manager.ExecutePluginChain(llmID, hookType, input, ctx)
	assert.NoError(t, err)
	assert.Equal(t, input, result) // Should return input unchanged when no plugins

	mockService.AssertExpectations(t)
}

func TestPluginManager_SaveReattachConfig_NotLoaded(t *testing.T) {
	mockService := &MockPluginService{}
	manager := NewPluginManager(mockService)

	err := manager.SaveReattachConfig(999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin 999 is not loaded")
}

// Test hook type validation in conversion functions
func TestHookTypeConversions(t *testing.T) {
	validHookTypes := []interfaces.HookType{
		interfaces.HookTypePreAuth,
		interfaces.HookTypeAuth,
		interfaces.HookTypePostAuth,
		interfaces.HookTypeOnResponse,
	}

	for _, hookType := range validHookTypes {
		// Test that hook type string conversions work
		assert.NotEmpty(t, string(hookType))
		
		// Test that each hook type is a valid string
		validStrings := []string{"pre_auth", "auth", "post_auth", "on_response"}
		assert.Contains(t, validStrings, string(hookType))
	}
}

// Test plugin context creation
func TestPluginContext(t *testing.T) {
	ctx := &interfaces.PluginContext{
		RequestID:    "test-request-123",
		LLMID:        1,
		LLMSlug:      "test-llm",
		Vendor:       "test-vendor",
		AppID:        2,
		UserID:       3,
		Metadata:     map[string]interface{}{"key": "value"},
		TraceContext: map[string]string{"trace-id": "abc123"},
	}

	assert.Equal(t, "test-request-123", ctx.RequestID)
	assert.Equal(t, uint(1), ctx.LLMID)
	assert.Equal(t, "test-llm", ctx.LLMSlug)
	assert.Equal(t, "test-vendor", ctx.Vendor)
	assert.Equal(t, uint(2), ctx.AppID)
	assert.Equal(t, uint(3), ctx.UserID)
	assert.Equal(t, "value", ctx.Metadata["key"])
	assert.Equal(t, "abc123", ctx.TraceContext["trace-id"])
}

// Test plugin request and response structures
func TestPluginRequestResponse(t *testing.T) {
	req := &interfaces.PluginRequest{
		Method:     "POST",
		Path:       "/api/test",
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(`{"test": true}`),
		RemoteAddr: "192.168.1.1",
		Context: &interfaces.PluginContext{
			RequestID: "test-123",
		},
	}

	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "/api/test", req.Path)
	assert.Equal(t, "application/json", req.Headers["Content-Type"])
	assert.Equal(t, `{"test": true}`, string(req.Body))
	assert.Equal(t, "192.168.1.1", req.RemoteAddr)
	assert.Equal(t, "test-123", req.Context.RequestID)

	resp := &interfaces.PluginResponse{
		Modified:     true,
		StatusCode:   200,
		Headers:      map[string]string{"X-Plugin": "test"},
		Body:         []byte(`{"modified": true}`),
		Block:        false,
		ErrorMessage: "",
	}

	assert.True(t, resp.Modified)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "test", resp.Headers["X-Plugin"])
	assert.Equal(t, `{"modified": true}`, string(resp.Body))
	assert.False(t, resp.Block)
	assert.Empty(t, resp.ErrorMessage)
}

// Test auth request and response structures
func TestAuthRequestResponse(t *testing.T) {
	authReq := &interfaces.AuthRequest{
		Credential: "bearer token123",
		AuthType:   "bearer",
		Request: &interfaces.PluginRequest{
			Method: "GET",
			Path:   "/protected",
		},
	}

	assert.Equal(t, "bearer token123", authReq.Credential)
	assert.Equal(t, "bearer", authReq.AuthType)
	assert.Equal(t, "GET", authReq.Request.Method)
	assert.Equal(t, "/protected", authReq.Request.Path)

	authResp := &interfaces.AuthResponse{
		Authenticated: true,
		UserID:        "user123",
		AppID:         "app456",
		Claims:        map[string]string{"role": "admin"},
		ErrorMessage:  "",
	}

	assert.True(t, authResp.Authenticated)
	assert.Equal(t, "user123", authResp.UserID)
	assert.Equal(t, "app456", authResp.AppID)
	assert.Equal(t, "admin", authResp.Claims["role"])
	assert.Empty(t, authResp.ErrorMessage)
}