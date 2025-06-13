package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Helper function to parse uint from string
func parseUint(s string) uint64 {
	id, _ := strconv.ParseUint(s, 10, 64)
	return id
}

func setupTestAPIWithAdminUser(t *testing.T) (*API, *gin.Engine, *models.User) {
	api, db := setupTestAPI(t)

	// Create admin user
	adminUser := &models.User{
		Email:             "admin@example.com",
		Name:              "Admin User",
		IsAdmin:           true,
		AccessToSSOConfig: true,
		ShowChat:          true,
		ShowPortal:        true,
	}
	err := db.Create(adminUser).Error
	assert.NoError(t, err)

	// Setup router with middleware to add admin user to context
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", adminUser)
		c.Next()
	})

	// Copy routes from api.router to r
	for _, route := range api.router.Routes() {
		r.Handle(route.Method, route.Path, route.HandlerFunc)
	}

	api.router = r
	return api, r, adminUser
}

func TestUserEndpoints(t *testing.T) {
	api, _, _ := setupTestAPIWithAdminUser(t)

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
				ShowChat:             true,
				ShowPortal:           true,
				EmailVerified:        true,
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
	api, _, _ := setupTestAPIWithAdminUser(t)

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
	assert.Contains(t, errorResponse["errors"].([]interface{})[0].(map[string]interface{})["detail"], "Email is already in use")
}

func TestSkipUserQuickStart(t *testing.T) {
	api, _, _ := setupTestAPIWithAdminUser(t)

	// Create a test user
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
				Email:    "quickstart@example.com",
				Name:     "QuickStart User",
				Password: "password123",
				IsAdmin:  false,
				Groups:   []uint{},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]UserResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	userID := response["data"].ID

	// Test Skip QuickStart
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/users/%s/skip-quick-start", userID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Get the user from the database
	user, err := api.service.GetUserByID(uint(parseUint(userID)))
	assert.NoError(t, err)

	// Create a new request with the user in context
	w = performRequest(api.router, "GET", "/api/v1/me", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var meResponse map[string]UserWithEntitlementsResponse
	err = json.Unmarshal(w.Body.Bytes(), &meResponse)
	assert.NoError(t, err)
	assert.True(t, meResponse["data"].Attributes.UIOptions.SkipQuickStart)
}

// Helper function to perform a request with a user in context
func performRequestWithUser(r *gin.Engine, method, path string, body interface{}, user *models.User) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", user)
	c.Request = httptest.NewRequest(method, path, nil)
	r.HandleContext(c)
	return w
}

func TestListUsersSearchFunctionality(t *testing.T) {
	api, _, _ := setupTestAPIWithAdminUser(t)

	// Create test users
	users := []UserInput{
		{
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
					Email:    "alice@example.com",
					Name:     "Alice Smith",
					Password: "password123",
					IsAdmin:  false,
					Groups:   []uint{},
				},
			},
		},
		{
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
					Email:    "bob@example.com",
					Name:     "Bob Johnson",
					Password: "password123",
					IsAdmin:  false,
					Groups:   []uint{},
				},
			},
		},
	}

	for _, user := range users {
		w := performRequest(api.router, "POST", "/api/v1/users", user)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	// Test search by email
	w := performRequest(api.router, "GET", "/api/v1/users?search=alice", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].([]interface{})
	assert.Equal(t, 1, len(data))

	// Test search by name
	w = performRequest(api.router, "GET", "/api/v1/users?search=johnson", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data = response["data"].([]interface{})
	assert.Equal(t, 1, len(data))

	// Test search with no results
	w = performRequest(api.router, "GET", "/api/v1/users?search=nonexistent", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data = response["data"].([]interface{})
	assert.Equal(t, 0, len(data))
}

func TestListUsersErrorResponseFormat(t *testing.T) {
	api, _, _ := setupTestAPIWithAdminUser(t)

	// Test invalid page number
	w := performRequest(api.router, "GET", "/api/v1/users?page=0", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errorResponse ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.NotEmpty(t, errorResponse.Errors)
	assert.Equal(t, "Bad Request", errorResponse.Errors[0].Title)

	// Test invalid page size
	w = performRequest(api.router, "GET", "/api/v1/users?page_size=0", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.NotEmpty(t, errorResponse.Errors)
	assert.Equal(t, "Bad Request", errorResponse.Errors[0].Title)

	// Test invalid sort field
	w = performRequest(api.router, "GET", "/api/v1/users?sort=invalid_field", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.NotEmpty(t, errorResponse.Errors)
	assert.Equal(t, "Bad Request", errorResponse.Errors[0].Title)
}
