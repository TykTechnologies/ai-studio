package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestUserEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create User
	createUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			}{
				Email:                "test@example.com",
				Name:                 "Test User",
				Password:             "password123",
				IsAdmin:              true,
				ShowChat:             true,
				ShowPortal:           true,
				EmailVerified:        true,
				NotificationsEnabled: true,
				AccessToSSOConfig:    true,
				Groups:               []uint{},
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
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			}{
				Email:                "updated@example.com",
				Name:                 "Updated User",
				IsAdmin:              true,
				NotificationsEnabled: true,
				AccessToSSOConfig:    true,
				Groups:               []uint{},
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%s", userID), updateUserInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Users
	w = performRequest(api.router, "GET", "/api/v1/users", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify pagination headers
	assert.NotEmpty(t, w.Header().Get("X-Total-Count"))
	assert.NotEmpty(t, w.Header().Get("X-Total-Pages"))

	// Verify response structure
	var usersResponse map[string][]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &usersResponse)
	assert.NoError(t, err)
	assert.NotEmpty(t, usersResponse["data"])

	// Check user fields including Role
	foundUser := false
	for _, user := range usersResponse["data"] {
		if user.ID == userID {
			foundUser = true
			assert.Equal(t, "updated@example.com", user.Attributes.Email)
			assert.Equal(t, "Updated User", user.Attributes.Name)
			assert.True(t, user.Attributes.IsAdmin)
			assert.True(t, user.Attributes.NotificationsEnabled)
			assert.True(t, user.Attributes.AccessToSSOConfig)
			assert.NotEmpty(t, user.Attributes.Role) // Check Role field is present and not empty
		}
	}
	assert.True(t, foundUser, "Updated user not found in response")

	// Test Delete User
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/users/%s", userID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestUserEmailUniqueness(t *testing.T) {
	api, _ := setupTestAPI(t)

	createUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			}{
				Email:    "test@example.com",
				Name:     "Test User",
				Password: "password123",
				IsAdmin:  true,
				Groups:   []uint{},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var firstUserResponse map[string]UserResponse
	err := json.Unmarshal(w.Body.Bytes(), &firstUserResponse)
	assert.NoError(t, err)

	w = performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errorResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)

	errors := errorResponse["errors"].([]interface{})
	assert.Equal(t, "Bad Request", errors[0].(map[string]interface{})["title"])
	assert.Equal(t, "Email is already in use", errors[0].(map[string]interface{})["detail"])

	createUserInput.Data.Attributes.Email = "TEST@example.com"
	w = performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	createUserInput.Data.Attributes.Email = "another@example.com"
	w = performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var secondUserResponse map[string]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &secondUserResponse)
	assert.NoError(t, err)

	updateUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email                string `json:"email"`
				Name                 string `json:"name"`
				Password             string `json:"password,omitempty"`
				IsAdmin              bool   `json:"is_admin"`
				ShowChat             bool   `json:"show_chat"`
				ShowPortal           bool   `json:"show_portal"`
				EmailVerified        bool   `json:"email_verified"`
				NotificationsEnabled bool   `json:"notifications_enabled"`
				AccessToSSOConfig    bool   `json:"access_to_sso_config"`
				Groups               []uint `json:"groups"`
			}{
				Email:   "test@example.com",
				Name:    "Updated User",
				IsAdmin: true,
				Groups:  []uint{},
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%s", secondUserResponse["data"].ID), updateUserInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSkipUserQuickStart(t *testing.T) {
	api, db := setupTestAPI(t)

	// Create a test user
	user := &models.User{
		Email:          "test@example.com",
		Name:           "Test User",
		IsAdmin:        false,
		ShowPortal:     true,
		ShowChat:       true,
		EmailVerified:  true,
		SkipQuickStart: false, // Initially false
	}
	err := user.Create(db)
	assert.NoError(t, err)
	assert.False(t, user.SkipQuickStart)

	// Test the skipUserQuickStart endpoint
	w := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/users/%d/skip-quick-start", user.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify response format
	var response map[string]string
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "success", response["status"])

	// Verify the user's SkipQuickStart flag was updated in the database
	var updatedUser models.User
	err = db.First(&updatedUser, user.ID).Error
	assert.NoError(t, err)
	assert.True(t, updatedUser.SkipQuickStart)

	// Test with invalid user ID
	w = performRequest(api.router, "POST", "/api/v1/users/invalid/skip-quick-start", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test with non-existent user ID
	w = performRequest(api.router, "POST", "/api/v1/users/9999/skip-quick-start", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestListUsersSearchFunctionality tests the search functionality of the listUsers endpoint
func TestListUsersSearchFunctionality(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create multiple users with different attributes for search testing
	users := []struct {
		email    string
		name     string
		isAdmin  bool
		password string
	}{
		{email: "john.doe@example.com", name: "John Doe", isAdmin: false, password: "password1"},
		{email: "jane.smith@example.com", name: "Jane Smith", isAdmin: true, password: "password2"},
		{email: "bob.jones@company.com", name: "Bob Jones", isAdmin: false, password: "password3"},
	}

	// Create the users
	for _, userData := range users {
		createUserInput := UserInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Email                string `json:"email"`
					Name                 string `json:"name"`
					Password             string `json:"password,omitempty"`
					IsAdmin              bool   `json:"is_admin"`
					ShowChat             bool   `json:"show_chat"`
					ShowPortal           bool   `json:"show_portal"`
					EmailVerified        bool   `json:"email_verified"`
					NotificationsEnabled bool   `json:"notifications_enabled"`
					AccessToSSOConfig    bool   `json:"access_to_sso_config"`
					Groups               []uint `json:"groups"`
				} `json:"attributes"`
			}{
				Type: "users",
				Attributes: struct {
					Email                string `json:"email"`
					Name                 string `json:"name"`
					Password             string `json:"password,omitempty"`
					IsAdmin              bool   `json:"is_admin"`
					ShowChat             bool   `json:"show_chat"`
					ShowPortal           bool   `json:"show_portal"`
					EmailVerified        bool   `json:"email_verified"`
					NotificationsEnabled bool   `json:"notifications_enabled"`
					AccessToSSOConfig    bool   `json:"access_to_sso_config"`
					Groups               []uint `json:"groups"`
				}{
					Email:    userData.email,
					Name:     userData.name,
					Password: userData.password,
					IsAdmin:  userData.isAdmin,
					Groups:   []uint{},
				},
			},
		}

		w := performRequest(api.router, "POST", "/api/v1/users", createUserInput)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	// Test 1: Search by email domain
	w := performRequest(api.router, "GET", "/api/v1/users?search=example.com", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var responseByDomain map[string][]UserResponse
	err := json.Unmarshal(w.Body.Bytes(), &responseByDomain)
	assert.NoError(t, err)

	// Should find 2 users with example.com domain
	assert.Len(t, responseByDomain["data"], 2)
	assert.NotEmpty(t, w.Header().Get("X-Total-Count"))
	assert.NotEmpty(t, w.Header().Get("X-Total-Pages"))

	// Verify all returned users have example.com in their email
	for _, user := range responseByDomain["data"] {
		assert.Contains(t, user.Attributes.Email, "example.com")
	}

	// Test 2: Search by name
	w = performRequest(api.router, "GET", "/api/v1/users?search=Bob", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var responseByName map[string][]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &responseByName)
	assert.NoError(t, err)

	// Should find 1 user with Bob in name
	assert.Len(t, responseByName["data"], 1)
	assert.Equal(t, "Bob Jones", responseByName["data"][0].Attributes.Name)

	// Test 3: Search with no matches
	w = performRequest(api.router, "GET", "/api/v1/users?search=nonexistent", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var responseNoMatches map[string][]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &responseNoMatches)
	assert.NoError(t, err)
	assert.Empty(t, responseNoMatches["data"])

	// Test 4: Search with sorting
	w = performRequest(api.router, "GET", "/api/v1/users?search=example.com&sort=name", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var responseSorted map[string][]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &responseSorted)
	assert.NoError(t, err)

	// Should be in alphabetical order by name
	assert.Len(t, responseSorted["data"], 2)
	assert.Equal(t, "Jane Smith", responseSorted["data"][0].Attributes.Name)
	assert.Equal(t, "John Doe", responseSorted["data"][1].Attributes.Name)
}

// TestListUsersErrorResponseFormat tests that the error response format from the listUsers endpoint is correct
func TestListUsersErrorResponseFormat(t *testing.T) {
	// This test verifies that the error response from the listUsers endpoint
	// follows the expected format when an error occurs

	// Create a regular API
	api, db := setupTestAPI(t)

	// Make a real error happen by closing the DB connection
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	sqlDB.Close()

	// Test regular listing with a closed DB - should produce a real error
	w := performRequest(api.router, "GET", "/api/v1/users", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify the error response format
	var errorResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)

	// Check that the error response has the correct structure
	errors, exists := errorResponse["errors"]
	assert.True(t, exists)
	errorList := errors.([]interface{})
	assert.NotEmpty(t, errorList)

	// Check that the error has title and detail fields
	errorObj := errorList[0].(map[string]interface{})
	assert.Equal(t, "Internal Server Error", errorObj["title"])
	assert.NotEmpty(t, errorObj["detail"])
}
