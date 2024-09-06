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
	user, err := api.service.CreateUser("testuser@example.com", "Test User", "password123")
	assert.NoError(t, err)

	// Test Create App
	createAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				UserID      uint   `json:"user_id"`
			} `json:"attributes"`
		}{
			Type: "apps",
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				UserID      uint   `json:"user_id"`
			}{
				Name:        "Test App",
				Description: "This is a test app",
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
	assert.Equal(t, "This is a test app", response["data"].Attributes.Description)
	assert.Equal(t, user.ID, response["data"].Attributes.UserID)

	appID := response["data"].ID

	// Test Get App
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%s", appID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update App
	updateAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				UserID      uint   `json:"user_id"`
			} `json:"attributes"`
		}{
			Type: "apps",
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				UserID      uint   `json:"user_id"`
			}{
				Name:        "Updated Test App",
				Description: "This is an updated test app",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%s", appID), updateAppInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updatedResponse map[string]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &updatedResponse)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Test App", updatedResponse["data"].Attributes.Name)
	assert.Equal(t, "This is an updated test app", updatedResponse["data"].Attributes.Description)

	// Test Get Apps by User ID
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps", user.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var userAppsResponse map[string][]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &userAppsResponse)
	assert.NoError(t, err)
	assert.Len(t, userAppsResponse["data"], 1)
	assert.Equal(t, appID, userAppsResponse["data"][0].ID)

	// Test Get App by Name
	w = performRequest(api.router, "GET", "/api/v1/apps/by-name?name=Updated Test App", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var appByNameResponse map[string]AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &appByNameResponse)
	assert.NoError(t, err)
	assert.Equal(t, appID, appByNameResponse["data"].ID)

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

func TestAppEndpoints_ErrorCases(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create App with Invalid Input
	invalidAppInput := AppInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				UserID      uint   `json:"user_id"`
			} `json:"attributes"`
		}{
			Type: "apps",
			Attributes: struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				UserID      uint   `json:"user_id"`
			}{
				Name:        "",
				Description: "This is an invalid app",
				UserID:      0,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", invalidAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get App with Invalid ID
	w = performRequest(api.router, "GET", "/api/v1/apps/invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Update App with Invalid ID
	w = performRequest(api.router, "PATCH", "/api/v1/apps/invalid", AppInput{})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Delete App with Invalid ID
	w = performRequest(api.router, "DELETE", "/api/v1/apps/invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get Apps by Invalid User ID
	w = performRequest(api.router, "GET", "/api/v1/users/invalid/apps", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get App by Name with Missing Name
	w = performRequest(api.router, "GET", "/api/v1/apps/by-name", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Activate App Credential with Invalid App ID
	w = performRequest(api.router, "POST", "/api/v1/apps/invalid/activate-credential", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Deactivate App Credential with Invalid App ID
	w = performRequest(api.router, "POST", "/api/v1/apps/invalid/deactivate-credential", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAppEndpoints_MultipleApps(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create test users
	user1, _ := api.service.CreateUser("user1@example.com", "User 1", "password123")
	user2, _ := api.service.CreateUser("user2@example.com", "User 2", "password456")

	// Create multiple apps
	createApp := func(name, description string, userID uint) string {
		input := AppInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					UserID      uint   `json:"user_id"`
				} `json:"attributes"`
			}{
				Type: "apps",
				Attributes: struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					UserID      uint   `json:"user_id"`
				}{
					Name:        name,
					Description: description,
					UserID:      userID,
				},
			},
		}

		w := performRequest(api.router, "POST", "/api/v1/apps", input)
		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]AppResponse
		json.Unmarshal(w.Body.Bytes(), &response)
		return response["data"].ID
	}

	app1ID := createApp("App 1", "Description 1", user1.ID)
	app2ID := createApp("App 2", "Description 2", user1.ID)
	app3ID := createApp("App 3", "Description 3", user2.ID)

	// Test Get Apps by User ID for user1
	w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps", user1.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var user1AppsResponse map[string][]AppResponse
	json.Unmarshal(w.Body.Bytes(), &user1AppsResponse)
	assert.Len(t, user1AppsResponse["data"], 2)
	assert.ElementsMatch(t, []string{app1ID, app2ID}, []string{user1AppsResponse["data"][0].ID, user1AppsResponse["data"][1].ID})

	// Test Get Apps by User ID for user2
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/apps", user2.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var user2AppsResponse map[string][]AppResponse
	json.Unmarshal(w.Body.Bytes(), &user2AppsResponse)
	assert.Len(t, user2AppsResponse["data"], 1)
	assert.Equal(t, app3ID, user2AppsResponse["data"][0].ID)

	// Test activating credentials for all apps
	for _, appID := range []string{app1ID, app2ID, app3ID} {
		w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/activate-credential", appID), nil)
		assert.Equal(t, http.StatusNoContent, w.Code)
	}

	// Test deactivating credentials for user1's apps
	for _, appID := range []string{app1ID, app2ID} {
		w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%s/deactivate-credential", appID), nil)
		assert.Equal(t, http.StatusNoContent, w.Code)
	}
}
