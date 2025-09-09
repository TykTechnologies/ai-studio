// internal/api/handlers/llm_handlers_test.go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestHandlers(t *testing.T) (*services.ServiceContainer, *gin.Engine) {
	// Create test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate
	err = database.Migrate(db)
	require.NoError(t, err)

	// Create test config
	cfg := &config.Config{
		Security: config.SecurityConfig{
			EncryptionKey: "12345678901234567890123456789012",
		},
		Cache: config.CacheConfig{
			Enabled: true,
			MaxSize: 100,
		},
		Analytics: config.AnalyticsConfig{
			Enabled:    true,
			BufferSize: 10,
		},
	}

	// Create service container
	container, err := services.NewServiceContainer(db, cfg)
	require.NoError(t, err)

	// Setup gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	return container, router
}

func TestListLLMs_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	
	// Create test LLMs
	llm1 := &database.LLM{
		Name:         "Test GPT-4",
		Slug:         "test-gpt-4",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	llm2 := &database.LLM{
		Name:         "Test Claude",
		Slug:         "test-claude", 
		Vendor:       "anthropic",
		DefaultModel: "claude-3",
		IsActive:     true, // Create as active first
	}
	
	container.Repository.CreateLLM(llm1)
	container.Repository.CreateLLM(llm2)
	
	// Now explicitly set llm2 to inactive
	llm2.IsActive = false
	container.Repository.UpdateLLM(llm2)

	// Setup route
	router.GET("/llms", ListLLMs(container))

	t.Run("ListActiveLLMs", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/llms?active=true", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 1)
		
		pagination := response["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["total"])
	})

	t.Run("ListInactiveLLMs", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/llms?active=false", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 1)
	})

	t.Run("PaginationParameters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/llms?page=1&limit=1&active=true", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		pagination := response["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(1), pagination["limit"])
	})
}

func TestCreateLLM_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.POST("/llms", CreateLLM(container))

	t.Run("ValidCreate", func(t *testing.T) {
		reqData := services.CreateLLMRequest{
			Name:         "Test OpenAI",
			Vendor:       "openai", 
			DefaultModel: "gpt-4",
			APIKey:       "sk-test123",
			IsActive:     true,
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("POST", "/llms", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "Test OpenAI", data["name"])
		assert.Equal(t, "test-openai", data["slug"])
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/llms", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("MissingRequiredFields", func(t *testing.T) {
		reqData := services.CreateLLMRequest{
			Name: "Incomplete LLM",
			// Missing vendor and model
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("POST", "/llms", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code) // Will fail validation in service
	})
}

func TestGetLLM_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.GET("/llms/:id", GetLLM(container))

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test GPT-4",
		Slug:         "test-gpt-4",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	container.Repository.CreateLLM(llm)

	t.Run("ValidGet", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/llms/"+strconv.Itoa(int(llm.ID)), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "Test GPT-4", data["name"])
	})

	t.Run("InvalidID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/llms/invalid", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("NotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/llms/999", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestUpdateLLM_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.PUT("/llms/:id", UpdateLLM(container))

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test GPT-4",
		Slug:         "test-gpt-4",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		MaxTokens:    4096,
		IsActive:     true,
	}
	container.Repository.CreateLLM(llm)

	t.Run("ValidUpdate", func(t *testing.T) {
		updateReq := services.UpdateLLMRequest{
			MaxTokens: intPtr(8192),
			IsActive:  boolPtr(false),
		}

		jsonData, _ := json.Marshal(updateReq)
		req := httptest.NewRequest("PUT", "/llms/"+strconv.Itoa(int(llm.ID)), bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, float64(8192), data["max_tokens"])
		assert.False(t, data["is_active"].(bool))
	})

	t.Run("UpdateNotFound", func(t *testing.T) {
		updateReq := services.UpdateLLMRequest{
			MaxTokens: intPtr(8192),
		}

		jsonData, _ := json.Marshal(updateReq)
		req := httptest.NewRequest("PUT", "/llms/999", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDeleteLLM_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.DELETE("/llms/:id", DeleteLLM(container))

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test GPT-4",
		Slug:         "test-gpt-4",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	container.Repository.CreateLLM(llm)

	t.Run("ValidDelete", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/llms/"+strconv.Itoa(int(llm.ID)), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		// Verify LLM is soft deleted
		_, err := container.Repository.GetLLM(llm.ID)
		assert.Error(t, err)
	})

	t.Run("DeleteNotFound", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/llms/999", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Helper functions for tests
func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}