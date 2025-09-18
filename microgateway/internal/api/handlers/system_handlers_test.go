// internal/api/handlers/system_handlers_test.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGateway for testing reload functionality
type MockGateway struct {
	reloadCalled bool
	reloadError  error
}

func (m *MockGateway) Reload() error {
	m.reloadCalled = true
	return m.reloadError
}

// Implement the minimum interface for aigateway.Gateway
func (m *MockGateway) Start() error                                      { return nil }
func (m *MockGateway) Stop(ctx context.Context) error                    { return nil }
func (m *MockGateway) Handler() http.Handler                             { return http.NotFoundHandler() }
func (m *MockGateway) GetPort() int                                      { return 8080 }
func (m *MockGateway) AddResponseHook(hook proxy.ResponseHook)           { /* no-op for testing */ }

func TestReloadConfiguration_Handler(t *testing.T) {
	_, router := setupTestHandlers(t)

	t.Run("ValidReload", func(t *testing.T) {
		mockGateway := &MockGateway{}
		router.POST("/system/reload", ReloadConfiguration(mockGateway))

		req := httptest.NewRequest("POST", "/system/reload", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, mockGateway.reloadCalled)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["message"], "reloaded successfully")
	})

	t.Run("ReloadError", func(t *testing.T) {
		_, router2 := setupTestHandlers(t)
		mockGateway := &MockGateway{
			reloadError: fmt.Errorf("reload failed"),
		}
		router2.POST("/system/reload", ReloadConfiguration(mockGateway))

		req := httptest.NewRequest("POST", "/system/reload", nil)
		w := httptest.NewRecorder()

		router2.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.True(t, mockGateway.reloadCalled)
	})

	t.Run("NilGateway", func(t *testing.T) {
		_, router3 := setupTestHandlers(t)
		router3.POST("/system/reload", ReloadConfiguration(nil))

		req := httptest.NewRequest("POST", "/system/reload", nil)
		w := httptest.NewRecorder()

		router3.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestGetSystemInfo_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.GET("/system/info", GetSystemInfo(container, "test", "hash123", "2023-01-01"))

	t.Run("ValidInfo", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/system/info", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "microgateway", data["service"])
		assert.Equal(t, "test", data["version"])
		assert.Equal(t, "hash123", data["build_hash"])
		assert.Contains(t, data, "stats")
	})
}