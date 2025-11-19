//go:build !enterprise
// +build !enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Test CE Logic: getGroup should return Default group only
func TestGetGroup_CommunityEdition_ReturnsDefaultGroupOnly(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Get or create default group
	defaultGroup, err := models.GetOrCreateDefaultGroup(db)
	assert.NoError(t, err)
	assert.Equal(t, "Default", defaultGroup.Name)

	// Setup router
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api.setupGroupRoutes(r.Group("/api/v1"))

	// Test 1: Getting default group by ID should succeed
	t.Run("Get default group succeeds", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/groups/%d", defaultGroup.ID)
		w := apitest.PerformRequest(r, "GET", path, nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data GroupResponse `json:"data"`
		}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "Default", response.Data.Attributes.Name)
	})

	// Test 2: Getting non-default group should return 404
	t.Run("Get non-default group returns 404", func(t *testing.T) {
		path := "/api/v1/groups/9999"
		w := apitest.PerformRequest(r, "GET", path, nil)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "not found")
		}
	})

	// Test 3: Invalid group ID should return 404
	t.Run("Invalid group ID returns 404", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/groups/invalid", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Test CE Logic: listGroups should return Default group only
func TestListGroups_CommunityEdition_ReturnsDefaultGroupOnly(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Ensure default group exists
	defaultGroup, err := models.GetOrCreateDefaultGroup(db)
	assert.NoError(t, err)

	// Setup router
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api.setupGroupRoutes(r.Group("/api/v1"))

	w := apitest.PerformRequest(r, "GET", "/api/v1/groups", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []GroupListResponse `json:"data"`
		Meta map[string]int      `json:"meta"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Should return exactly 1 group (Default)
	assert.Len(t, response.Data, 1, "CE should return only Default group")
	assert.Equal(t, fmt.Sprintf("%d", defaultGroup.ID), response.Data[0].ID)
	assert.Equal(t, "Default", response.Data[0].Attributes.Name)

	// Check metadata
	assert.Equal(t, 1, response.Meta["total_count"])
	assert.Equal(t, 1, response.Meta["page_size"])
}

// Test CE Logic: getUserGroups should return Default group for any user
func TestGetUserGroups_CommunityEdition_ReturnsDefaultGroup(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Create a test user
	user := createTestUser(t, service)

	// Ensure default group exists
	defaultGroup, err := models.GetOrCreateDefaultGroup(db)
	assert.NoError(t, err)

	// Setup router
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api.setupGroupRoutes(r.Group("/api/v1"))

	// Get groups for user
	path := fmt.Sprintf("/api/v1/users/%d/groups", user.ID)
	w := apitest.PerformRequest(r, "GET", path, nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []GroupResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Should return Default group
	assert.Len(t, response.Data, 1, "CE should return only Default group for any user")
	assert.Equal(t, fmt.Sprintf("%d", defaultGroup.ID), response.Data[0].ID)
	assert.Equal(t, "Default", response.Data[0].Attributes.Name)
}

// Test serialization helper
func TestSerializeGroup_CommunityEdition(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)

	// Create test data
	group, err := service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)

	// Test serialization
	response := serializeGroup(group)

	assert.Equal(t, "groups", response.Type)
	assert.Equal(t, fmt.Sprintf("%d", group.ID), response.ID)
	assert.Equal(t, "Test Group", response.Attributes.Name)
}

// Test 402 stub handlers - all should return Enterprise Edition required
func TestGroupHandlers_CommunityEdition_Return402(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Setup router
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api.setupGroupRoutes(r.Group("/api/v1"))

	tests := []struct {
		name   string
		method string
		path   string
	}{
		// Group CRUD (except GET and List - tested above)
		{"CreateGroup", "POST", "/api/v1/groups"},
		{"UpdateGroup", "PATCH", "/api/v1/groups/1"},
		{"DeleteGroup", "DELETE", "/api/v1/groups/1"},

		// User management
		{"AddUserToGroup", "POST", "/api/v1/groups/1/users"},
		{"RemoveUserFromGroup", "DELETE", "/api/v1/groups/1/users/1"},
		{"ListGroupUsers", "GET", "/api/v1/groups/1/users"},
		{"UpdateGroupUsers", "PUT", "/api/v1/groups/1/users"},

		// LLM Catalogue management
		{"AddCatalogueToGroup", "POST", "/api/v1/groups/1/catalogues"},
		{"RemoveCatalogueFromGroup", "DELETE", "/api/v1/groups/1/catalogues/1"},
		{"ListGroupCatalogues", "GET", "/api/v1/groups/1/catalogues"},
		{"UpdateGroupCatalogues", "PUT", "/api/v1/groups/1/catalogues"},

		// Data Catalogue management
		{"AddDataCatalogueToGroup", "POST", "/api/v1/groups/1/data-catalogues"},
		{"RemoveDataCatalogueFromGroup", "DELETE", "/api/v1/groups/1/data-catalogues/1"},
		{"ListGroupDataCatalogues", "GET", "/api/v1/groups/1/data-catalogues"},

		// Tool Catalogue management
		{"AddToolCatalogueToGroup", "POST", "/api/v1/groups/1/tool-catalogues"},
		{"RemoveToolCatalogueFromGroup", "DELETE", "/api/v1/groups/1/tool-catalogues/1"},
		{"ListGroupToolCatalogues", "GET", "/api/v1/groups/1/tool-catalogues"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := apitest.PerformRequest(r, tt.method, tt.path, nil)

			assert.Equal(t, http.StatusPaymentRequired, w.Code,
				"Handler %s should return 402 in Community Edition", tt.name)

			var errResp ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &errResp)
			assert.NoError(t, err)

			// Verify Enterprise feature message
			if len(errResp.Errors) > 0 {
				assert.Contains(t, errResp.Errors[0].Detail, "Enterprise",
					"Error message should mention Enterprise")
			}
		})
	}
}

// setupGroupRoutes registers group-related routes
func (a *API) setupGroupRoutes(r *gin.RouterGroup) {
	// Group CRUD
	r.POST("/groups", a.createGroup)
	r.GET("/groups/:id", a.getGroup)
	r.PATCH("/groups/:id", a.updateGroup)
	r.DELETE("/groups/:id", a.deleteGroup)
	r.GET("/groups", a.listGroups)

	// User management
	r.POST("/groups/:id/users", a.addUserToGroup)
	r.DELETE("/groups/:id/users/:userId", a.removeUserFromGroup)
	r.GET("/groups/:id/users", a.listGroupUsers)
	r.PUT("/groups/:id/users", a.updateGroupUsers)

	// LLM Catalogue management
	r.POST("/groups/:id/catalogues", a.addCatalogueToGroup)
	r.DELETE("/groups/:id/catalogues/:catalogueId", a.removeCatalogueFromGroup)
	r.GET("/groups/:id/catalogues", a.listGroupCatalogues)
	r.PUT("/groups/:id/catalogues", a.updateGroupCatalogues)

	// Data Catalogue management
	r.POST("/groups/:id/data-catalogues", a.addDataCatalogueToGroup)
	r.DELETE("/groups/:id/data-catalogues/:catalogueId", a.removeDataCatalogueFromGroup)
	r.GET("/groups/:id/data-catalogues", a.listGroupDataCatalogues)

	// Tool Catalogue management
	r.POST("/groups/:id/tool-catalogues", a.addToolCatalogueToGroup)
	r.DELETE("/groups/:id/tool-catalogues/:catalogueId", a.removeToolCatalogueFromGroup)
	r.GET("/groups/:id/tool-catalogues", a.listGroupToolCatalogues)

	// User groups
	r.GET("/users/:userId/groups", a.getUserGroups)
}
