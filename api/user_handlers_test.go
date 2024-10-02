package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create User
	createUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			}{
				Email:    "test@example.com",
				Name:     "Test User",
				Password: "password123",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]UserResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test@example.com", response["data"].Attributes.Email)

	userID := response["data"].ID

	// Test Get User
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%s", userID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update User
	updateUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			}{
				Email: "updated@example.com",
				Name:  "Updated User",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%s", userID), updateUserInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Users
	w = performRequest(api.router, "GET", "/api/v1/users", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Delete User
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/users/%s", userID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}
