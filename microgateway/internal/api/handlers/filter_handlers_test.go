// internal/api/handlers/filter_handlers_test.go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListFilters_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)

	// Create test filters
	filter1 := &database.Filter{
		Name:        "Test Filter 1",
		Description: "First test filter",
		Script:      "result = true",
		IsActive:    true,
	}
	filter2 := &database.Filter{
		Name:        "Test Filter 2",
		Description: "Second test filter", 
		Script:      "result = false",
		IsActive:    true, // Create as active first
	}

	container.FilterService.CreateFilter(&services.CreateFilterRequest{
		Name:     filter1.Name,
		Description:     filter1.Description,
		Script:   filter1.Script,
		IsActive: filter1.IsActive,
	})
	createdFilter2, err := container.FilterService.CreateFilter(&services.CreateFilterRequest{
		Name:     filter2.Name,
		Description:     filter2.Description,
		Script:   filter2.Script,
		IsActive: filter2.IsActive,
	})
	require.NoError(t, err)

	// Make filter2 inactive to test filtering
	isActive := false
	updateReq := &services.UpdateFilterRequest{
		IsActive: &isActive,
	}
	container.FilterService.UpdateFilter(createdFilter2.ID, updateReq)

	router.GET("/filters", ListFilters(container))

	t.Run("ListActiveFilters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/filters?active=true", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 1) // Only active filters
	})

	t.Run("ListFiltersWithDescription", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/filters?active=true", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 1)
		filterData := data[0].(map[string]interface{})
		assert.NotEmpty(t, filterData["description"])
	})
}

func TestCreateFilter_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.POST("/filters", CreateFilter(container))

	t.Run("ValidCreate", func(t *testing.T) {
		reqData := services.CreateFilterRequest{
			Name:       "API Test Filter",
			Description: "API test filter",
			Script:     "result = true",
			IsActive:   true,
			OrderIndex: 1,
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("POST", "/filters", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "API Test Filter", data["name"])
		assert.Equal(t, "API test filter", data["description"])
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/filters", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("MissingRequiredFields", func(t *testing.T) {
		reqData := map[string]interface{}{
			"name": "", // Empty name should fail validation
			"description": "Created via handler test",
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("POST", "/filters", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetFilter_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.GET("/filters/:id", GetFilter(container))

	// Create test filter
	testFilter := &services.CreateFilterRequest{
		Name:   "Get Test Filter",
		Description: "Handler test filter",
		Script: "result = true",
	}
	createdFilter, err := container.FilterService.CreateFilter(testFilter)
	require.NoError(t, err)

	t.Run("ValidGet", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/filters/"+strconv.Itoa(int(createdFilter.ID)), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "Get Test Filter", data["name"])
	})

	t.Run("InvalidID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/filters/invalid", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("NotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/filters/999", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestLLMFilters_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.GET("/llms/:id/filters", GetLLMFilters(container))
	router.PUT("/llms/:id/filters", UpdateLLMFilters(container))

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	err := container.Repository.CreateLLM(llm)
	require.NoError(t, err)

	// Create test filter
	filter := &services.CreateFilterRequest{
		Name:   "LLM Test Filter",
		Description: "Handler test filter",
		Script: "result = true",
	}
	createdFilter, err := container.FilterService.CreateFilter(filter)
	require.NoError(t, err)

	t.Run("UpdateLLMFilters", func(t *testing.T) {
		reqData := map[string]interface{}{
			"filter_ids": []uint{createdFilter.ID},
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("PUT", "/llms/"+strconv.Itoa(int(llm.ID))+"/filters", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GetLLMFilters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/llms/"+strconv.Itoa(int(llm.ID))+"/filters", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 1)
		filterData := data[0].(map[string]interface{})
		assert.Equal(t, "LLM Test Filter", filterData["name"])
	})

	t.Run("RemoveAllFilters", func(t *testing.T) {
		reqData := map[string]interface{}{
			"filter_ids": []uint{}, // Empty array to remove all filters
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("PUT", "/llms/"+strconv.Itoa(int(llm.ID))+"/filters", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Verify no filters are associated
		req2 := httptest.NewRequest("GET", "/llms/"+strconv.Itoa(int(llm.ID))+"/filters", nil)
		w2 := httptest.NewRecorder()

		router.ServeHTTP(w2, req2)

		var response map[string]interface{}
		err := json.Unmarshal(w2.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 0) // No filters
	})
}

func TestUpdateFilter_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.PUT("/filters/:id", UpdateFilter(container))

	// Create test filter
	testFilter := &services.CreateFilterRequest{
		Name:   "Update Test Filter",
		Description: "Handler test filter", 
		Script: "result = true",
	}
	createdFilter, err := container.FilterService.CreateFilter(testFilter)
	require.NoError(t, err)

	t.Run("ValidUpdate", func(t *testing.T) {
		updateData := map[string]interface{}{
			"name":   "Updated via API",
			"script": "result = false",
		}

		jsonData, _ := json.Marshal(updateData)
		req := httptest.NewRequest("PUT", "/filters/"+strconv.Itoa(int(createdFilter.ID)), bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "Updated via API", data["name"])
	})
}

func TestDeleteFilter_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.DELETE("/filters/:id", DeleteFilter(container))

	// Create test filter
	testFilter := &services.CreateFilterRequest{
		Name:   "Delete Test Filter",
		Description: "Handler test filter",
		Script: "result = true",
	}
	createdFilter, err := container.FilterService.CreateFilter(testFilter)
	require.NoError(t, err)

	t.Run("ValidDelete", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/filters/"+strconv.Itoa(int(createdFilter.ID)), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("DeleteNotFound", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/filters/999", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Filter delete returns 404 for non-existent filters (unlike LLM idempotent deletes)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}