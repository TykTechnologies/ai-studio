package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a test user
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123", true)
	assert.NoError(t, err)

	// Test Create App
	createAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			} `json:"attributes"`
		}{
			Type: "apps",
			Attributes: struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			}{
				Name:        "Test App",
				Description: "A test app",
				UserID:      user.ID,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
	assert.Equal(t, http.StatusCreated, w.Code)
	return

	var response map[string]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test App", response["data"].Attributes.Name)

	appID := response["data"].ID

	// Test Get App
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%s", appID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update App
	updateAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			} `json:"attributes"`
		}{
			Type: "apps",
			Attributes: struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			}{
				Name:        "Updated App",
				Description: "An updated test app",
				UserID:      user.ID,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%s", appID), updateAppInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Apps
	w = performRequest(api.router, "GET", "/api/v1/apps", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "Updated App", listResponse["data"][0].Attributes.Name)

	// Test Search Apps
	w = performRequest(api.router, "GET", "/api/v1/apps/search?search_term=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	if w.Code != http.StatusOK {
		fmt.Println(w.Body.String())
	}

	var searchResponse map[string][]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(searchResponse), 1)
	if len(searchResponse) > 0 {
		assert.Len(t, searchResponse["data"], 1)

		if len(searchResponse["data"]) > 0 {
			assert.Equal(t, "Updated App", searchResponse["data"][0].Attributes.Name)
		}
	}

	// Test Count Apps
	w = performRequest(api.router, "GET", "/api/v1/apps/count", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var countResponse map[string]int64
	err = json.Unmarshal(w.Body.Bytes(), &countResponse)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), countResponse["count"])

	// Test Count Apps by User ID
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps/count", user.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &countResponse)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), countResponse["count"])

	// Test Get Apps by User ID
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps", user.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var userAppsResponse map[string][]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &userAppsResponse)
	assert.NoError(t, err)
	assert.Len(t, userAppsResponse["data"], 1)
	assert.Equal(t, "Updated App", userAppsResponse["data"][0].Attributes.Name)

	// Test Get App by Name
	w = performRequest(api.router, "GET", "/api/v1/apps/by-name?name=Updated App", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var appByNameResponse map[string]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &appByNameResponse)
	assert.NoError(t, err)
	assert.Equal(t, "Updated App", appByNameResponse["data"].Attributes.Name)

	// Test Activate App Credential
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/activate-credential", appID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Deactivate App Credential
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/deactivate-credential", appID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Delete App
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/apps/%s", appID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify app is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%s", appID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAppEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent app
	w := performRequest(api.router, "GET", "/api/v1/apps/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent app
	updateAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			} `json:"attributes"`
		}{
			Type: "apps",
			Attributes: struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			}{
				Name:        "Updated App",
				Description: "An updated test app",
				UserID:      1,
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/apps/999", updateAppInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent app
	w = performRequest(api.router, "DELETE", "/api/v1/apps/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Create app with invalid input
	invalidCreateAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			} `json:"attributes"`
		}{
			Type: "apps",
			Attributes: struct {
				Name          string `json:"name"`
				Description   string `json:"description"`
				UserID        uint   `json:"user_id"`
				DatasourceIDs []uint `json:"datasource_ids"`
				LLMIDs        []uint `json:"llm_ids"`
			}{
				Name:        "",
				Description: "",
				UserID:      0,
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/apps", invalidCreateAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Search Apps with empty search term
	w = performRequest(api.router, "GET", "/api/v1/apps/search", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get Apps by non-existent User ID
	w = performRequest(api.router, "GET", "/api/v1/users/999/apps", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]AppResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)

	// Test Get App by non-existent name
	w = performRequest(api.router, "GET", "/api/v1/apps/by-name?name=NonExistentApp", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Activate Credential for non-existent app
	w = performRequest(api.router, "POST", "/api/v1/apps/999/activate-credential", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test Deactivate Credential for non-existent app
	w = performRequest(api.router, "POST", "/api/v1/apps/999/deactivate-credential", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAppPagination(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a test user
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123", true)
	assert.NoError(t, err)

	// Create multiple apps
	createApp := func(name string) {
		input := AppInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Name          string `json:"name"`
					Description   string `json:"description"`
					UserID        uint   `json:"user_id"`
					DatasourceIDs []uint `json:"datasource_ids"`
					LLMIDs        []uint `json:"llm_ids"`
				} `json:"attributes"`
			}{
				Type: "apps",
				Attributes: struct {
					Name          string `json:"name"`
					Description   string `json:"description"`
					UserID        uint   `json:"user_id"`
					DatasourceIDs []uint `json:"datasource_ids"`
					LLMIDs        []uint `json:"llm_ids"`
				}{
					Name:        name,
					Description: "Test app",
					UserID:      user.ID,
				},
			},
		}
		w := performRequest(api.router, "POST", "/api/v1/apps", input)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	// Create 15 apps
	for i := 1; i <= 15; i++ {
		createApp(fmt.Sprintf("Test App %d", i))
	}

	// Test listApps pagination
	t.Run("List Apps Pagination", func(t *testing.T) {
		// First page
		w := performRequest(api.router, "GET", "/api/v1/apps?page=1&page_size=10", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
		assert.Equal(t, "2", w.Header().Get("X-Total-Pages"))

		var response map[string][]AppResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response["data"], 10)

		// Second page
		w = performRequest(api.router, "GET", "/api/v1/apps?page=2&page_size=10", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
		assert.Equal(t, "2", w.Header().Get("X-Total-Pages"))

		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response["data"], 5)

		// Page size larger than total
		w = performRequest(api.router, "GET", "/api/v1/apps?page=1&page_size=20", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
		assert.Equal(t, "1", w.Header().Get("X-Total-Pages"))

		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response["data"], 15)
	})

	// Test searchApps pagination
	t.Run("Search Apps Pagination", func(t *testing.T) {
		// First page
		w := performRequest(api.router, "GET", "/api/v1/apps/search?search_term=Test&page=1&page_size=10", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
		assert.Equal(t, "2", w.Header().Get("X-Total-Pages"))

		var response map[string][]AppResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response["data"], 10)

		// Second page
		w = performRequest(api.router, "GET", "/api/v1/apps/search?search_term=Test&page=2&page_size=10", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
		assert.Equal(t, "2", w.Header().Get("X-Total-Pages"))

		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response["data"], 5)

		// Search with no results
		w = performRequest(api.router, "GET", "/api/v1/apps/search?search_term=NonexistentApp&page=1&page_size=10", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "0", w.Header().Get("X-Total-Count"))
		assert.Equal(t, "0", w.Header().Get("X-Total-Pages"))

		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response["data"], 0)
	})
}
