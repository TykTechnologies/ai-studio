package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

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

	// Create a test group first
	group := &models.Group{
		Name: "Test Group",
	}
	err := api.service.DB.Create(group).Error
	assert.NoError(t, err)
	assert.NotZero(t, group.ID)

	// Create another test group
	secondGroup := &models.Group{
		Name: "Second Test Group",
	}
	err = api.service.DB.Create(secondGroup).Error
	assert.NoError(t, err)
	assert.NotZero(t, secondGroup.ID)

	// Test Create User with groups
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
				Groups:               []uint{group.ID, secondGroup.ID}, // Assign to both groups
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test@example.com", response["data"].Attributes.Email)

	userID := response["data"].ID
	// Verify user has the correct groups assigned
	id, _ := strconv.ParseUint(userID, 10, 64)
	createdUser, err := api.service.GetUserByID(uint(id), "Groups")
	assert.NoError(t, err)
	assert.Len(t, createdUser.Groups, 2)
	// Test attempting to create a user with a non-existent group
	nonExistentGroupID := uint(9999) // Using a high number that's unlikely to exist
	createUserWithBadGroupInput := UserInput{
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
				Email:    "nonexistent-group@example.com",
				Name:     "Non-existent Group User",
				Password: "password123",
				IsAdmin:  false,
				Groups:   []uint{nonExistentGroupID},
			},
		},
	}

	w = performRequest(api.router, "POST", "/api/v1/users", createUserWithBadGroupInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error response contains the expected message about non-existent group
	var errorResponseForGroup map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &errorResponseForGroup)
	assert.NoError(t, err)

	// Safely check error response structure
	assert.Contains(t, errorResponseForGroup, "errors")
	errorsArrayGroup, okGroup := errorResponseForGroup["errors"].([]interface{})
	assert.True(t, okGroup)
	assert.NotEmpty(t, errorsArrayGroup)
	errorObjGroup, okGroup := errorsArrayGroup[0].(map[string]interface{})
	assert.True(t, okGroup)
	assert.Contains(t, errorObjGroup, "detail")
	assert.Contains(t, errorObjGroup["detail"].(string), "groups not found")

	// Check if the groups match the ones we assigned
	groupIDs := make([]uint, len(createdUser.Groups))
	for i, g := range createdUser.Groups {
		groupIDs[i] = g.ID
	}
	assert.Contains(t, groupIDs, group.ID)
	assert.Contains(t, groupIDs, secondGroup.ID)

	// Test Get User
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%s", userID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test email uniqueness - try to create a user with the same email
	w = performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error response contains the expected message
	var errorResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)

	// Safely check error response structure
	assert.Contains(t, errorResponse, "errors")
	errorsArray, ok := errorResponse["errors"].([]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, errorsArray)
	errorObj, ok := errorsArray[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, errorObj, "detail")
	assert.Contains(t, errorObj["detail"], "Email is already in use")

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

// TestAdminPermissions verifies that only super admins can create/update/delete admin users
func TestAdminPermissions(t *testing.T) {
	// Setup for regular admin tests
	api, db := setupTestAPI(t)

	// Create a regular admin (not the first user, so not super admin)
	regularAdmin := &models.User{
		ID:                2, // ID 2 is a regular admin
		Email:             "regular-admin@example.com",
		Name:              "Regular Admin",
		IsAdmin:           true,
		AccessToSSOConfig: true,
		ShowChat:          true,
		ShowPortal:        true,
	}
	err := db.Create(regularAdmin).Error
	assert.NoError(t, err)

	// Setup router with regular admin user in context
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", regularAdmin)
		c.Next()
	})

	// Copy routes from api.router to r
	for _, route := range api.router.Routes() {
		r.Handle(route.Method, route.Path, route.HandlerFunc)
	}

	api.router = r

	// Test 1: Regular admin cannot create admin users
	createAdminUserInput := UserInput{
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
				Email:    "new-admin@example.com",
				Name:     "New Admin User",
				Password: "password123",
				IsAdmin:  true, // Trying to create an admin user
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/users", createAdminUserInput)
	assert.Equal(t, http.StatusForbidden, w.Code)

	var errorResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)

	// Safely check error response structure
	assert.Contains(t, errorResponse, "errors")
	errorsArray, ok := errorResponse["errors"].([]interface{})
	assert.True(t, ok)
	assert.NotEmpty(t, errorsArray)
	errorObj, ok := errorsArray[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, errorObj, "detail")
	assert.Contains(t, errorObj["detail"], "operation only allowed for super admin user")

	// Setup for super admin tests
	apiSuper, dbSuper := setupTestAPI(t)

	// Create a super admin (ID 1)
	superAdmin := &models.User{
		ID:                1, // ID 1 is super admin
		Email:             "super-admin@example.com",
		Name:              "Super Admin",
		IsAdmin:           true,
		AccessToSSOConfig: true,
		ShowChat:          true,
		ShowPortal:        true,
	}
	err = dbSuper.Create(superAdmin).Error
	assert.NoError(t, err)

	// Setup router with super admin user
	rSuper := gin.New()
	rSuper.Use(func(c *gin.Context) {
		c.Set("user", superAdmin)
		c.Next()
	})

	for _, route := range apiSuper.router.Routes() {
		rSuper.Handle(route.Method, route.Path, route.HandlerFunc)
	}

	apiSuper.router = rSuper

	// Test 2: Super admin can create admin users
	w = performRequest(apiSuper.router, "POST", "/api/v1/users", createAdminUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResponse map[string]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &createResponse)
	assert.NoError(t, err)
	assert.Equal(t, "new-admin@example.com", createResponse["data"].Attributes.Email)
	assert.True(t, createResponse["data"].Attributes.IsAdmin)

	adminUserID := createResponse["data"].ID

	// Test 3: Regular admin cannot update admin users
	// First create an admin user that we'll try to update
	adminToUpdate := &models.User{
		Email:   "admin-to-update@example.com",
		Name:    "Admin To Update",
		IsAdmin: true,
	}
	err = db.Create(adminToUpdate).Error
	assert.NoError(t, err)

	updateAdminUserInput := UserInput{
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
				Email:   "updated-admin@example.com",
				Name:    "Updated Admin User",
				IsAdmin: true,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%d", adminToUpdate.ID), updateAdminUserInput)
	assert.Equal(t, http.StatusForbidden, w.Code)

	var updateErrorResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &updateErrorResponse)
	assert.NoError(t, err)
	// Safely check that we have errors
	assert.Contains(t, updateErrorResponse, "errors")

	// Test 4: Regular admin cannot delete admin users
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/users/%d", adminToUpdate.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	var deleteErrorResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &deleteErrorResponse)
	assert.NoError(t, err)
	// Safely check that we have errors
	assert.Contains(t, deleteErrorResponse, "errors")

	// Test 5: Super admin can update admin users
	w = performRequest(apiSuper.router, "PATCH", fmt.Sprintf("/api/v1/users/%s", adminUserID), updateAdminUserInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test 6: Super admin can delete admin users
	w = performRequest(apiSuper.router, "DELETE", fmt.Sprintf("/api/v1/users/%s", adminUserID), nil)
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
	// Test case 2: Trying to update a user's email to one that already exists
	// Create a second user with a different email
	secondUserInput := UserInput{
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
				Email:    "test2@example.com",
				Name:     "Second Test User",
				Password: "password123",
				IsAdmin:  false,
				Groups:   []uint{},
			},
		},
	}

	w = performRequest(api.router, "POST", "/api/v1/users", secondUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var secondUserResponse map[string]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &secondUserResponse)
	assert.NoError(t, err)
	secondUserID := secondUserResponse["data"].ID

	// Now try to update the second user's email to match the first user's email
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
				Email:  "test@example.com", // Already used by the first user
				Name:   "Updated Second User",
				Groups: []uint{},
			},
		},
	}

	// Attempt to update with duplicate email should fail
	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%s", secondUserID), updateUserInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Verify error response
	var updateErrorResponse map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &updateErrorResponse)
	assert.NoError(t, err)
	assert.Contains(t, updateErrorResponse["errors"].([]interface{})[0].(map[string]interface{})["detail"], "Email is already in use")
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

	// Retrieve user to check if flag was set
	id, _ := strconv.ParseUint(userID, 10, 64)
	userWithFlag, err := api.service.GetUserByID(uint(id))
	assert.NoError(t, err)
	assert.True(t, userWithFlag.SkipQuickStart, "SkipQuickStart flag should be true")
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
