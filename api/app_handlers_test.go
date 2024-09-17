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
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123")
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
	w = performRequest(api.router, "GET", "/api/v1/apps/search?searchTerm=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated App", searchResponse["data"][0].Attributes.Name)

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

// func TestAppEndpoints(t *testing.T) {
// 	api, _ := setupTestAPI(t)

// 	// Create a test user
// 	user, err := api.service.CreateUser("testuser@example.com", "Test User", "password123")
// 	assert.NoError(t, err)

// 	// Test Create App
// 	createAppInput := AppInput{
// 		Data: struct {
// 			Type       string `json:"type"`
// 			Attributes struct {
// 				Name          string `json:"name"`
// 				Description   string `json:"description"`
// 				UserID        uint   `json:"user_id"`
// 				DatasourceIDs []uint `json:"datasource_ids"`
// 				LLMIDs        []uint `json:"llm_ids"`
// 			} `json:"attributes"`
// 		}{
// 			Type: "apps",
// 			Attributes: struct {
// 				Name          string `json:"name"`
// 				Description   string `json:"description"`
// 				UserID        uint   `json:"user_id"`
// 				DatasourceIDs []uint `json:"datasource_ids"`
// 				LLMIDs        []uint `json:"llm_ids"`
// 			}{
// 				Name:        "Test App",
// 				Description: "This is a test app",
// 				UserID:      user.ID,
// 			},
// 		},
// 	}

// 	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
// 	assert.Equal(t, http.StatusCreated, w.Code)

// 	var response map[string]AppResponse
// 	err = json.Unmarshal(w.Body.Bytes(), &response)
// 	assert.NoError(t, err)
// 	assert.Equal(t, "Test App", response["data"].Attributes.Name)
// 	assert.Equal(t, "This is a test app", response["data"].Attributes.Description)
// 	assert.Equal(t, user.ID, response["data"].Attributes.UserID)

// 	appID := response["data"].ID

// 	// Test Get App
// 	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%s", appID), nil)
// 	assert.Equal(t, http.StatusOK, w.Code)

// 	// Test Update App
// 	updateAppInput := AppInput{
// 		Data: struct {
// 			Type       string `json:"type"`
// 			Attributes struct {
// 				Name          string `json:"name"`
// 				Description   string `json:"description"`
// 				UserID        uint   `json:"user_id"`
// 				DatasourceIDs []uint `json:"datasource_ids"`
// 				LLMIDs        []uint `json:"llm_ids"`
// 			} `json:"attributes"`
// 		}{
// 			Type: "apps",
// 			Attributes: struct {
// 				Name          string `json:"name"`
// 				Description   string `json:"description"`
// 				UserID        uint   `json:"user_id"`
// 				DatasourceIDs []uint `json:"datasource_ids"`
// 				LLMIDs        []uint `json:"llm_ids"`
// 			}{
// 				Name:        "Updated Test App",
// 				Description: "This is an updated test app",
// 			},
// 		},
// 	}

// 	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%s", appID), updateAppInput)
// 	assert.Equal(t, http.StatusOK, w.Code)

// 	var updatedResponse map[string]AppResponse
// 	err = json.Unmarshal(w.Body.Bytes(), &updatedResponse)
// 	assert.NoError(t, err)
// 	assert.Equal(t, "Updated Test App", updatedResponse["data"].Attributes.Name)
// 	assert.Equal(t, "This is an updated test app", updatedResponse["data"].Attributes.Description)

// 	// Test Get Apps by User ID
// 	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps", user.ID), nil)
// 	assert.Equal(t, http.StatusOK, w.Code)

// 	var userAppsResponse map[string][]AppResponse
// 	err = json.Unmarshal(w.Body.Bytes(), &userAppsResponse)
// 	assert.NoError(t, err)
// 	assert.Len(t, userAppsResponse["data"], 1)
// 	assert.Equal(t, appID, userAppsResponse["data"][0].ID)

// 	// Test Get App by Name
// 	w = performRequest(api.router, "GET", "/api/v1/apps/by-name?name=Updated Test App", nil)
// 	assert.Equal(t, http.StatusOK, w.Code)

// 	var appByNameResponse map[string]AppResponse
// 	err = json.Unmarshal(w.Body.Bytes(), &appByNameResponse)
// 	assert.NoError(t, err)
// 	assert.Equal(t, appID, appByNameResponse["data"].ID)

// 	// Test Activate App Credential
// 	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/activate-credential", appID), nil)
// 	assert.Equal(t, http.StatusNoContent, w.Code)

// 	// Test Deactivate App Credential
// 	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/deactivate-credential", appID), nil)
// 	assert.Equal(t, http.StatusNoContent, w.Code)

// 	// Test Delete App
// 	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/apps/%s", appID), nil)
// 	assert.Equal(t, http.StatusNoContent, w.Code)

// 	// Verify app is deleted
// 	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%s", appID), nil)
// 	assert.Equal(t, http.StatusNotFound, w.Code)
// }

// func TestAppEndpoints_ErrorCases(t *testing.T) {
// 	api, _ := setupTestAPI(t)

// 	// Test Create App with Invalid Input
// 	invalidAppInput := AppInput{
// 		Data: struct {
// 			Type       string `json:"type"`
// 			Attributes struct {
// 				Name          string `json:"name"`
// 				Description   string `json:"description"`
// 				UserID        uint   `json:"user_id"`
// 				DatasourceIDs []uint `json:"datasource_ids"`
// 				LLMIDs        []uint `json:"llm_ids"`
// 			} `json:"attributes"`
// 		}{
// 			Type: "apps",
// 			Attributes: struct {
// 				Name          string `json:"name"`
// 				Description   string `json:"description"`
// 				UserID        uint   `json:"user_id"`
// 				DatasourceIDs []uint `json:"datasource_ids"`
// 				LLMIDs        []uint `json:"llm_ids"`
// 			}{
// 				Name:        "",
// 				Description: "This is an invalid app",
// 				UserID:      0,
// 			},
// 		},
// 	}

// 	w := performRequest(api.router, "POST", "/api/v1/apps", invalidAppInput)
// 	assert.Equal(t, http.StatusBadRequest, w.Code)

// 	// Test Get App with Invalid ID
// 	w = performRequest(api.router, "GET", "/api/v1/apps/invalid", nil)
// 	assert.Equal(t, http.StatusBadRequest, w.Code)

// 	// Test Update App with Invalid ID
// 	w = performRequest(api.router, "PATCH", "/api/v1/apps/invalid", AppInput{})
// 	assert.Equal(t, http.StatusBadRequest, w.Code)

// 	// Test Delete App with Invalid ID
// 	w = performRequest(api.router, "DELETE", "/api/v1/apps/invalid", nil)
// 	assert.Equal(t, http.StatusBadRequest, w.Code)

// 	// Test Get Apps by Invalid User ID
// 	w = performRequest(api.router, "GET", "/api/v1/users/invalid/apps", nil)
// 	assert.Equal(t, http.StatusBadRequest, w.Code)

// 	// Test Get App by Name with Missing Name
// 	w = performRequest(api.router, "GET", "/api/v1/apps/by-name", nil)
// 	assert.Equal(t, http.StatusBadRequest, w.Code)

// 	// Test Activate App Credential with Invalid App ID
// 	w = performRequest(api.router, "POST", "/api/v1/apps/invalid/activate-credential", nil)
// 	assert.Equal(t, http.StatusBadRequest, w.Code)

// 	// Test Deactivate App Credential with Invalid App ID
// 	w = performRequest(api.router, "POST", "/api/v1/apps/invalid/deactivate-credential", nil)
// 	assert.Equal(t, http.StatusBadRequest, w.Code)
// }

// func TestAppEndpoints_MultipleApps(t *testing.T) {
// 	api, _ := setupTestAPI(t)

// 	// Create test users
// 	user1, _ := api.service.CreateUser("user1@example.com", "User 1", "password123")
// 	user2, _ := api.service.CreateUser("user2@example.com", "User 2", "password456")

// 	// Create multiple apps
// 	createApp := func(name, description string, userID uint) string {
// 		input := AppInput{
// 			Data: struct {
// 				Type       string `json:"type"`
// 				Attributes struct {
// 					Name          string `json:"name"`
// 					Description   string `json:"description"`
// 					UserID        uint   `json:"user_id"`
// 					DatasourceIDs []uint `json:"datasource_ids"`
// 					LLMIDs        []uint `json:"llm_ids"`
// 				} `json:"attributes"`
// 			}{
// 				Type: "apps",
// 				Attributes: struct {
// 					Name          string `json:"name"`
// 					Description   string `json:"description"`
// 					UserID        uint   `json:"user_id"`
// 					DatasourceIDs []uint `json:"datasource_ids"`
// 					LLMIDs        []uint `json:"llm_ids"`
// 				}{
// 					Name:        name,
// 					Description: description,
// 					UserID:      userID,
// 				},
// 			},
// 		}

// 		w := performRequest(api.router, "POST", "/api/v1/apps", input)
// 		assert.Equal(t, http.StatusCreated, w.Code)

// 		var response map[string]AppResponse
// 		json.Unmarshal(w.Body.Bytes(), &response)
// 		return response["data"].ID
// 	}

// 	app1ID := createApp("App 1", "Description 1", user1.ID)
// 	app2ID := createApp("App 2", "Description 2", user1.ID)
// 	app3ID := createApp("App 3", "Description 3", user2.ID)

// 	// Test Get Apps by User ID for user1
// 	w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps", user1.ID), nil)
// 	assert.Equal(t, http.StatusOK, w.Code)

// 	var user1AppsResponse map[string][]AppResponse
// 	json.Unmarshal(w.Body.Bytes(), &user1AppsResponse)
// 	assert.Len(t, user1AppsResponse["data"], 2)
// 	assert.ElementsMatch(t, []string{app1ID, app2ID}, []string{user1AppsResponse["data"][0].ID, user1AppsResponse["data"][1].ID})

// 	// Test Get Apps by User ID for user2
// 	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps", user2.ID), nil)
// 	assert.Equal(t, http.StatusOK, w.Code)

// 	var user2AppsResponse map[string][]AppResponse
// 	json.Unmarshal(w.Body.Bytes(), &user2AppsResponse)
// 	assert.Len(t, user2AppsResponse["data"], 1)
// 	assert.Equal(t, app3ID, user2AppsResponse["data"][0].ID)

// 	// Test activating credentials for all apps
// 	for _, appID := range []string{app1ID, app2ID, app3ID} {
// 		w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/activate-credential", appID), nil)
// 		assert.Equal(t, http.StatusNoContent, w.Code)
// 	}

// 	// Test deactivating credentials for user1's apps
// 	for _, appID := range []string{app1ID, app2ID} {
// 		w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/deactivate-credential", appID), nil)
// 		assert.Equal(t, http.StatusNoContent, w.Code)
// 	}
// }
