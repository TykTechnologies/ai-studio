package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestPluginEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Plugin
	createPluginInput := services.CreatePluginRequest{
		Name:        "Test Plugin",
		Slug:        "test-plugin",
		Description: "A test plugin for unit testing",
		Command:     "/usr/local/bin/test-plugin",
		Checksum:    "abc123",
		Config:      map[string]interface{}{"timeout": 30, "debug": true},
		HookType:    "post_auth",
		IsActive:    true,
		Namespace:   "test-namespace",
	}

	w := performRequest(api.router, "POST", "/api/v1/plugins", createPluginInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createResponse struct {
		Data PluginResponse `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &createResponse)
	assert.NoError(t, err)
	assert.Equal(t, "Test Plugin", createResponse.Data.Attributes.Name)
	assert.Equal(t, "test-plugin", createResponse.Data.Attributes.Slug)
	assert.Equal(t, "post_auth", createResponse.Data.Attributes.HookType)
	assert.Equal(t, true, createResponse.Data.Attributes.IsActive)
	assert.Equal(t, "test-namespace", createResponse.Data.Attributes.Namespace)

	pluginID := createResponse.Data.ID

	// Test Get Plugin
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/plugins/%s", pluginID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var getResponse struct {
		Data PluginResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &getResponse)
	assert.NoError(t, err)
	assert.Equal(t, "Test Plugin", getResponse.Data.Attributes.Name)
	assert.Equal(t, "test-plugin", getResponse.Data.Attributes.Slug)

	// Test List Plugins (default - active only)
	w = performRequest(api.router, "GET", "/api/v1/plugins", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse PluginListResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(listResponse.Data), 1)
	assert.Equal(t, int64(1), listResponse.Meta.TotalCount)
	
	// Find our plugin in the list
	found := false
	for _, plugin := range listResponse.Data {
		if plugin.ID == pluginID {
			found = true
			assert.Equal(t, "Test Plugin", plugin.Attributes.Name)
			assert.Equal(t, true, plugin.Attributes.IsActive)
			break
		}
	}
	assert.True(t, found, "Created plugin should appear in active plugins list")

	// Test List Plugins with hook_type filter
	w = performRequest(api.router, "GET", "/api/v1/plugins?hook_type=post_auth", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	for _, plugin := range listResponse.Data {
		assert.Equal(t, "post_auth", plugin.Attributes.HookType)
	}

	// Test Update Plugin (make inactive)
	updatePluginInput := services.UpdatePluginRequest{
		Name:        stringPtr("Updated Test Plugin"),
		Description: stringPtr("Updated description"),
		IsActive:    boolPtr(false),
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/plugins/%s", pluginID), updatePluginInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse struct {
		Data PluginResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Test Plugin", updateResponse.Data.Attributes.Name)
	assert.Equal(t, "Updated description", updateResponse.Data.Attributes.Description)
	assert.Equal(t, false, updateResponse.Data.Attributes.IsActive)

	// Test List Active Plugins (should not include our inactive plugin)
	w = performRequest(api.router, "GET", "/api/v1/plugins?is_active=true", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	
	// Our plugin should not be in the active list
	foundInActiveList := false
	for _, plugin := range listResponse.Data {
		if plugin.ID == pluginID {
			foundInActiveList = true
			break
		}
	}
	assert.False(t, foundInActiveList, "Inactive plugin should not appear in active plugins list")

	// Test List Inactive Plugins (should include our inactive plugin)
	w = performRequest(api.router, "GET", "/api/v1/plugins?is_active=false", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	
	// Our plugin should be in the inactive list
	foundInInactiveList := false
	for _, plugin := range listResponse.Data {
		if plugin.ID == pluginID {
			foundInInactiveList = true
			assert.Equal(t, false, plugin.Attributes.IsActive)
			break
		}
	}
	assert.True(t, foundInInactiveList, "Inactive plugin should appear in inactive plugins list")

	// Test List All Plugins (no is_active filter - should show both active and inactive)
	w = performRequest(api.router, "GET", "/api/v1/plugins", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	
	// Should include our inactive plugin when no filter is applied
	foundInAllList := false
	for _, plugin := range listResponse.Data {
		if plugin.ID == pluginID {
			foundInAllList = true
			break
		}
	}
	assert.True(t, foundInAllList, "Plugin should appear in unfiltered list regardless of active status")

	// Test List with Namespace Filter
	w = performRequest(api.router, "GET", "/api/v1/plugins?namespace=test-namespace", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	
	// Should include our plugin in the namespace filter
	foundInNamespaceList := false
	for _, plugin := range listResponse.Data {
		if plugin.ID == pluginID {
			foundInNamespaceList = true
			assert.Equal(t, "test-namespace", plugin.Attributes.Namespace)
			break
		}
	}
	assert.True(t, foundInNamespaceList, "Plugin should appear when filtering by its namespace")

	// Test List with Different Namespace (should not include our plugin)
	w = performRequest(api.router, "GET", "/api/v1/plugins?namespace=different-namespace", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	
	// Should not include our plugin in different namespace
	foundInDifferentNamespace := false
	for _, plugin := range listResponse.Data {
		if plugin.ID == pluginID {
			foundInDifferentNamespace = true
			break
		}
	}
	assert.False(t, foundInDifferentNamespace, "Plugin should not appear when filtering by different namespace")

	// Test Delete Plugin
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", pluginID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify plugin is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/plugins/%s", pluginID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPluginValidation(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Plugin with missing required fields
	createPluginInput := services.CreatePluginRequest{
		// Missing Name, Slug, Command, HookType
		Description: "A test plugin",
		IsActive:    true,
	}

	w := performRequest(api.router, "POST", "/api/v1/plugins", createPluginInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errorResponse ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.Greater(t, len(errorResponse.Errors), 0)

	// Test Create Plugin with invalid hook type
	createPluginInput = services.CreatePluginRequest{
		Name:     "Test Plugin",
		Slug:     "test-plugin",
		Command:  "/usr/local/bin/test-plugin",
		HookType: "invalid_hook_type",
		IsActive: true,
	}

	w = performRequest(api.router, "POST", "/api/v1/plugins", createPluginInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Create Plugin with duplicate slug
	validPluginInput := services.CreatePluginRequest{
		Name:     "Valid Plugin",
		Slug:     "unique-plugin",
		Command:  "/usr/local/bin/valid-plugin",
		HookType: "pre_auth",
		IsActive: true,
	}

	w = performRequest(api.router, "POST", "/api/v1/plugins", validPluginInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Try to create another plugin with the same slug
	duplicateSlugInput := services.CreatePluginRequest{
		Name:     "Duplicate Plugin",
		Slug:     "unique-plugin", // Same slug
		Command:  "/usr/local/bin/duplicate-plugin",
		HookType: "post_auth",
		IsActive: true,
	}

	w = performRequest(api.router, "POST", "/api/v1/plugins", duplicateSlugInput)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestPluginPagination(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create multiple plugins for pagination testing
	pluginIDs := make([]string, 0)
	
	for i := 1; i <= 5; i++ {
		createPluginInput := services.CreatePluginRequest{
			Name:        fmt.Sprintf("Test Plugin %d", i),
			Slug:        fmt.Sprintf("test-plugin-%d", i),
			Description: fmt.Sprintf("Test plugin number %d", i),
			Command:     fmt.Sprintf("/usr/local/bin/test-plugin-%d", i),
			HookType:    "pre_auth",
			IsActive:    i%2 == 1, // Odd numbered plugins are active
			Namespace:   fmt.Sprintf("namespace-%d", i%3), // Rotate through 3 namespaces
		}

		w := performRequest(api.router, "POST", "/api/v1/plugins", createPluginInput)
		assert.Equal(t, http.StatusCreated, w.Code)

		var response PluginResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		pluginIDs = append(pluginIDs, response.ID)
	}

	// Test pagination with limit=2
	w := performRequest(api.router, "GET", "/api/v1/plugins?page=1&limit=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse PluginListResponse
	err := json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(listResponse.Data), 2)
	assert.Equal(t, 1, listResponse.Meta.PageNumber)
	assert.Equal(t, 2, listResponse.Meta.PageSize)

	// Test filtering by active status
	w = performRequest(api.router, "GET", "/api/v1/plugins?is_active=true", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	for _, plugin := range listResponse.Data {
		assert.True(t, plugin.Attributes.IsActive, "All plugins in active filter should be active")
	}

	// Test filtering by inactive status
	w = performRequest(api.router, "GET", "/api/v1/plugins?is_active=false", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	for _, plugin := range listResponse.Data {
		assert.False(t, plugin.Attributes.IsActive, "All plugins in inactive filter should be inactive")
	}

	// Cleanup
	for _, id := range pluginIDs {
		performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", id), nil)
	}
}

func TestPluginNamespaceFiltering(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create plugins in different namespaces
	globalPlugin := services.CreatePluginRequest{
		Name:     "Global Plugin",
		Slug:     "global-plugin",
		Command:  "/usr/local/bin/global-plugin",
		HookType: "pre_auth",
		IsActive: true,
		Namespace: "", // Global namespace
	}

	tenantAPlugin := services.CreatePluginRequest{
		Name:     "Tenant A Plugin",
		Slug:     "tenant-a-plugin",
		Command:  "/usr/local/bin/tenant-a-plugin",
		HookType: "post_auth",
		IsActive: true,
		Namespace: "tenant-a",
	}

	tenantBPlugin := services.CreatePluginRequest{
		Name:     "Tenant B Plugin",
		Slug:     "tenant-b-plugin",
		Command:  "/usr/local/bin/tenant-b-plugin",
		HookType: "on_response",
		IsActive: true,
		Namespace: "tenant-b",
	}

	// Create all plugins
	w := performRequest(api.router, "POST", "/api/v1/plugins", globalPlugin)
	assert.Equal(t, http.StatusCreated, w.Code)
	var globalResponse struct {
		Data PluginResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &globalResponse)

	w = performRequest(api.router, "POST", "/api/v1/plugins", tenantAPlugin)
	assert.Equal(t, http.StatusCreated, w.Code)
	var tenantAResponse struct {
		Data PluginResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &tenantAResponse)

	w = performRequest(api.router, "POST", "/api/v1/plugins", tenantBPlugin)
	assert.Equal(t, http.StatusCreated, w.Code)
	var tenantBResponse struct {
		Data PluginResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &tenantBResponse)

	// Test list without namespace filter (should show all)
	w = performRequest(api.router, "GET", "/api/v1/plugins", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse PluginListResponse
	err := json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(listResponse.Data), 3, "Should show all plugins when no namespace filter")

	// Test list with tenant-a namespace filter
	w = performRequest(api.router, "GET", "/api/v1/plugins?namespace=tenant-a", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	
	// Should include global + tenant-a plugins
	foundGlobal := false
	foundTenantA := false
	foundTenantB := false
	
	for _, plugin := range listResponse.Data {
		switch plugin.ID {
		case globalResponse.Data.ID:
			foundGlobal = true
		case tenantAResponse.Data.ID:
			foundTenantA = true
		case tenantBResponse.Data.ID:
			foundTenantB = true
		}
	}
	
	assert.True(t, foundGlobal, "Global plugins should appear in namespace-filtered results")
	assert.True(t, foundTenantA, "Tenant-specific plugin should appear in its namespace filter")
	assert.False(t, foundTenantB, "Other tenant plugins should not appear in namespace filter")

	// Cleanup
	performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", globalResponse.Data.ID), nil)
	performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", tenantAResponse.Data.ID), nil)
	performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", tenantBResponse.Data.ID), nil)
}

// TestPluginListingFiltersComprehensive tests all possible filter combinations for plugin listing
// TestPluginFilteringDebug debugs the current plugin filtering issues
func TestPluginFilteringDebug(t *testing.T) {
	// Use clean test setup without pre-existing test data
	api, _, _ := setupTestAPIForCommonTests(t)

	// Create our test plugins
	testPlugins := []struct {
		name      string
		slug      string
		hookType  string
		isActive  bool
		namespace string
	}{
		{"PreAuth Active Global", "preauth-active-global", "pre_auth", true, ""},
		{"PreAuth Inactive Global", "preauth-inactive-global", "pre_auth", false, ""},
		{"Auth Active TenantA", "auth-active-tenant-a", "auth", true, "tenant-a"},
	}

	createdPlugins := make([]string, 0)

	// Create test plugins
	for _, testPlugin := range testPlugins {
		createRequest := services.CreatePluginRequest{
			Name:      testPlugin.name,
			Slug:      testPlugin.slug,
			Command:   fmt.Sprintf("/usr/local/bin/%s", testPlugin.slug),
			HookType:  testPlugin.hookType,
			IsActive:  testPlugin.isActive,
			Namespace: testPlugin.namespace,
		}

		w := performRequest(api.router, "POST", "/api/v1/plugins", createRequest)
		assert.Equal(t, http.StatusCreated, w.Code, "Failed to create plugin %s", testPlugin.name)

		var response struct {
			Data PluginResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		t.Logf("Created plugin: ID=%s, Name=%s, HookType=%s, IsActive=%t, Namespace=%s", 
			response.Data.ID, response.Data.Attributes.Name, response.Data.Attributes.HookType, 
			response.Data.Attributes.IsActive, response.Data.Attributes.Namespace)
		createdPlugins = append(createdPlugins, response.Data.ID)
	}

	// Now test what the API actually returns for pre_auth active plugins
	w := performRequest(api.router, "GET", "/api/v1/plugins?hook_type=pre_auth&is_active=true", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse PluginListResponse
	err := json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)

	t.Logf("Found %d pre_auth active plugins:", len(listResponse.Data))
	for _, plugin := range listResponse.Data {
		t.Logf("  - ID=%s, Name=%s, HookType=%s, IsActive=%t, Namespace=%s", 
			plugin.ID, plugin.Attributes.Name, plugin.Attributes.HookType, 
			plugin.Attributes.IsActive, plugin.Attributes.Namespace)
	}

	// Test what happens when we request inactive pre_auth plugins
	w = performRequest(api.router, "GET", "/api/v1/plugins?hook_type=pre_auth&is_active=false", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)

	t.Logf("Found %d pre_auth inactive plugins:", len(listResponse.Data))
	for _, plugin := range listResponse.Data {
		t.Logf("  - ID=%s, Name=%s, HookType=%s, IsActive=%t, Namespace=%s", 
			plugin.ID, plugin.Attributes.Name, plugin.Attributes.HookType, 
			plugin.Attributes.IsActive, plugin.Attributes.Namespace)
	}

	// Test what happens when we don't specify is_active (should show all active by default?)
	w = performRequest(api.router, "GET", "/api/v1/plugins?hook_type=pre_auth", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)

	t.Logf("Found %d pre_auth plugins (no is_active filter):", len(listResponse.Data))
	for _, plugin := range listResponse.Data {
		t.Logf("  - ID=%s, Name=%s, HookType=%s, IsActive=%t, Namespace=%s", 
			plugin.ID, plugin.Attributes.Name, plugin.Attributes.HookType, 
			plugin.Attributes.IsActive, plugin.Attributes.Namespace)
	}

	// Cleanup
	for _, pluginID := range createdPlugins {
		performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", pluginID), nil)
	}
}

func TestPluginListingFiltersComprehensive(t *testing.T) {
	// Use clean test setup without pre-existing test data
	api, _, _ := setupTestAPIForCommonTests(t)

	// Create a focused set of test plugins to validate all filtering scenarios
	testPlugins := []struct {
		name      string
		slug      string
		hookType  string
		isActive  bool
		namespace string
	}{
		// Test each hook type with both active and inactive states
		{"PreAuth Active", "preauth-active", "pre_auth", true, ""},
		{"PreAuth Inactive", "preauth-inactive", "pre_auth", false, ""},
		{"Auth Active", "auth-active", "auth", true, "tenant-a"},
		{"Auth Inactive", "auth-inactive", "auth", false, "tenant-a"},
		{"PostAuth Active", "postauth-active", "post_auth", true, "tenant-b"},
		{"PostAuth Inactive", "postauth-inactive", "post_auth", false, ""},
		{"OnResponse Active", "onresponse-active", "on_response", true, ""},
		{"DataCollection Active", "datacollection-active", "data_collection", true, "tenant-a"},
	}

	createdPlugins := make([]string, 0)

	// Create all test plugins
	for _, testPlugin := range testPlugins {
		createRequest := services.CreatePluginRequest{
			Name:      testPlugin.name,
			Slug:      testPlugin.slug,
			Command:   fmt.Sprintf("/usr/local/bin/%s", testPlugin.slug),
			HookType:  testPlugin.hookType,
			IsActive:  testPlugin.isActive,
			Namespace: testPlugin.namespace,
		}

		w := performRequest(api.router, "POST", "/api/v1/plugins", createRequest)
		assert.Equal(t, http.StatusCreated, w.Code, "Failed to create plugin %s", testPlugin.name)

		var response struct {
			Data PluginResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		createdPlugins = append(createdPlugins, response.Data.ID)
	}

	// First, let's determine the actual counts by querying
	w := performRequest(api.router, "GET", "/api/v1/plugins", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var allPluginsResponse PluginListResponse
	err := json.Unmarshal(w.Body.Bytes(), &allPluginsResponse)
	assert.NoError(t, err)
	totalPlugins := len(allPluginsResponse.Data)
	t.Logf("Total plugins created: %d", totalPlugins)

	// Count by status
	activeCount := 0
	inactiveCount := 0
	for _, plugin := range allPluginsResponse.Data {
		if plugin.Attributes.IsActive {
			activeCount++
		} else {
			inactiveCount++
		}
	}
	t.Logf("Active plugins: %d, Inactive plugins: %d", activeCount, inactiveCount)

	// Test 1: Basic filtering behavior
	t.Run("Basic filtering behavior", func(t *testing.T) {
		// Test no filters (should return all plugins)
		w := performRequest(api.router, "GET", "/api/v1/plugins", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var response PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, totalPlugins, len(response.Data), "No filters should return all plugins")

		// Test active only filter
		w = performRequest(api.router, "GET", "/api/v1/plugins?is_active=true", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, activeCount, len(response.Data), "is_active=true should return only active plugins")
		for _, plugin := range response.Data {
			assert.True(t, plugin.Attributes.IsActive, "All plugins should be active")
		}

		// Test inactive only filter
		w = performRequest(api.router, "GET", "/api/v1/plugins?is_active=false", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, inactiveCount, len(response.Data), "is_active=false should return only inactive plugins")
		for _, plugin := range response.Data {
			assert.False(t, plugin.Attributes.IsActive, "All plugins should be inactive")
		}
	})

	// Test 2: Hook type filtering
	t.Run("Hook type filtering", func(t *testing.T) {
		hookTypes := []string{"pre_auth", "auth", "post_auth", "on_response", "data_collection"}
		
		for _, hookType := range hookTypes {
			// Test hook type without is_active (should return ALL plugins of that type)
			w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/plugins?hook_type=%s", hookType), nil)
			assert.Equal(t, http.StatusOK, w.Code)

			var response PluginListResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			
			// Verify all returned plugins have the correct hook type
			for _, plugin := range response.Data {
				assert.Equal(t, hookType, plugin.Attributes.HookType, "All plugins should have hook type %s", hookType)
			}
			
			// Test hook type + active filter
			w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/plugins?hook_type=%s&is_active=true", hookType), nil)
			assert.Equal(t, http.StatusOK, w.Code)
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			
			for _, plugin := range response.Data {
				assert.Equal(t, hookType, plugin.Attributes.HookType)
				assert.True(t, plugin.Attributes.IsActive, "All plugins should be active")
			}
			
			// Test hook type + inactive filter
			w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/plugins?hook_type=%s&is_active=false", hookType), nil)
			assert.Equal(t, http.StatusOK, w.Code)
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			
			for _, plugin := range response.Data {
				assert.Equal(t, hookType, plugin.Attributes.HookType)
				assert.False(t, plugin.Attributes.IsActive, "All plugins should be inactive")
			}
		}
		
		// Test invalid hook type
		w := performRequest(api.router, "GET", "/api/v1/plugins?hook_type=invalid_hook", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var response PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(response.Data), "Invalid hook type should return no plugins")
	})

	// Test 3: Namespace filtering
	t.Run("Namespace filtering", func(t *testing.T) {
		// Test namespace filtering - the key behavior is that global plugins should appear in ALL namespace queries
		namespaces := []string{"", "tenant-a", "tenant-b", "nonexistent-namespace"}
		
		for _, namespace := range namespaces {
			w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/plugins?namespace=%s", namespace), nil)
			assert.Equal(t, http.StatusOK, w.Code)

			var response PluginListResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			
			t.Logf("Namespace '%s' returned %d plugins", namespace, len(response.Data))

			// Verify namespace filtering logic: should include global plugins OR plugins in the specified namespace
			for _, plugin := range response.Data {
				assert.True(t, plugin.Attributes.Namespace == "" || plugin.Attributes.Namespace == namespace,
					"Plugin namespace '%s' should be empty (global) or match requested namespace '%s'", 
					plugin.Attributes.Namespace, namespace)
			}
			
			// If filtering by empty namespace, should only return global plugins
			if namespace == "" {
				for _, plugin := range response.Data {
					assert.Equal(t, "", plugin.Attributes.Namespace, "Global namespace filter should only return global plugins")
				}
			}
		}
		
		// Test namespace + active filtering
		w := performRequest(api.router, "GET", "/api/v1/plugins?namespace=tenant-a&is_active=true", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var response PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		
		for _, plugin := range response.Data {
			assert.True(t, plugin.Attributes.Namespace == "" || plugin.Attributes.Namespace == "tenant-a")
			assert.True(t, plugin.Attributes.IsActive)
		}
	})

	// Test 4: Combined filtering validation
	t.Run("Combined filtering validation", func(t *testing.T) {
		// Test hook_type + is_active combination
		w := performRequest(api.router, "GET", "/api/v1/plugins?hook_type=pre_auth&is_active=true", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var response PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		
		for _, plugin := range response.Data {
			assert.Equal(t, "pre_auth", plugin.Attributes.HookType)
			assert.True(t, plugin.Attributes.IsActive)
		}
		
		// Test hook_type + namespace combination
		w = performRequest(api.router, "GET", "/api/v1/plugins?hook_type=auth&namespace=tenant-a", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		
		for _, plugin := range response.Data {
			assert.Equal(t, "auth", plugin.Attributes.HookType)
			assert.True(t, plugin.Attributes.Namespace == "" || plugin.Attributes.Namespace == "tenant-a")
		}
		
		// Test all three filters combined
		w = performRequest(api.router, "GET", "/api/v1/plugins?hook_type=auth&is_active=true&namespace=tenant-a", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		
		for _, plugin := range response.Data {
			assert.Equal(t, "auth", plugin.Attributes.HookType)
			assert.True(t, plugin.Attributes.IsActive)
			assert.True(t, plugin.Attributes.Namespace == "" || plugin.Attributes.Namespace == "tenant-a")
		}
	})

	// Test 5: Pagination behavior
	t.Run("Pagination behavior", func(t *testing.T) {
		// Test basic pagination with active plugins
		w := performRequest(api.router, "GET", "/api/v1/plugins?is_active=true&page=1&limit=3", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var response PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		
		// Should respect the limit
		assert.LessOrEqual(t, len(response.Data), 3, "Should respect limit parameter")
		assert.Equal(t, int64(activeCount), response.Meta.TotalCount, "Total count should match active plugins")
		assert.Equal(t, 3, response.Meta.PageSize)
		assert.Equal(t, 1, response.Meta.PageNumber)
		
		// Verify all returned plugins are active
		for _, plugin := range response.Data {
			assert.True(t, plugin.Attributes.IsActive)
		}
		
		// Test pagination edge cases
		w = performRequest(api.router, "GET", "/api/v1/plugins?page=0&limit=0", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 1, response.Meta.PageNumber, "Page should default to 1")
		assert.Equal(t, 20, response.Meta.PageSize, "Limit should default to 20")
		
		// Test very large limit (should be capped)
		w = performRequest(api.router, "GET", "/api/v1/plugins?limit=1000", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 20, response.Meta.PageSize, "Large limit should be capped to 20")
	})

	// Test 8: Pagination with filters
	t.Run("Pagination with filters", func(t *testing.T) {
		// Test pagination on active plugins (5 total active)
		w := performRequest(api.router, "GET", "/api/v1/plugins?is_active=true&page=1&limit=3", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var listResponse PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(listResponse.Data))
		assert.Equal(t, int64(5), listResponse.Meta.TotalCount) // Total active plugins
		assert.Equal(t, 3, listResponse.Meta.PageSize)
		assert.Equal(t, 1, listResponse.Meta.PageNumber)
		assert.Equal(t, 2, listResponse.Meta.TotalPages) // ceil(5/3) = 2

		// Test second page
		w = performRequest(api.router, "GET", "/api/v1/plugins?is_active=true&page=2&limit=3", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(listResponse.Data)) // 5 total - 3 first page = 2 remaining
		assert.Equal(t, int64(5), listResponse.Meta.TotalCount)
		assert.Equal(t, 2, listResponse.Meta.PageNumber)

		// Test pagination on ALL plugins (8 total)
		w = performRequest(api.router, "GET", "/api/v1/plugins?page=1&limit=5", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Equal(t, 5, len(listResponse.Data))
		assert.Equal(t, int64(8), listResponse.Meta.TotalCount) // Total all plugins
		assert.Equal(t, 5, listResponse.Meta.PageSize)
		assert.Equal(t, 1, listResponse.Meta.PageNumber)
		assert.Equal(t, 2, listResponse.Meta.TotalPages) // ceil(8/5) = 2
	})

	// Test 9: Edge cases and error conditions
	t.Run("Edge cases and error conditions", func(t *testing.T) {
		// Test invalid hook type
		w := performRequest(api.router, "GET", "/api/v1/plugins?hook_type=invalid_hook", nil)
		assert.Equal(t, http.StatusOK, w.Code) // Should return empty result, not error

		var listResponse PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(listResponse.Data), "Invalid hook type should return empty result")
		assert.Equal(t, int64(0), listResponse.Meta.TotalCount)

		// Test invalid page numbers (should default to valid values)
		w = performRequest(api.router, "GET", "/api/v1/plugins?page=0&limit=0", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Equal(t, 1, listResponse.Meta.PageNumber) // Should default to 1
		assert.Equal(t, 20, listResponse.Meta.PageSize)  // Should default to 20

		// Test very large limit (should be capped)
		w = performRequest(api.router, "GET", "/api/v1/plugins?limit=1000", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Equal(t, 20, listResponse.Meta.PageSize) // Should be capped to 20

		// Test page beyond available results
		w = performRequest(api.router, "GET", "/api/v1/plugins?page=1000", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		err = json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(listResponse.Data), "Page beyond available results should return empty")
		assert.Equal(t, 1000, listResponse.Meta.PageNumber)
	})

	// Test 10: Special characters in filter values
	t.Run("Special characters in filter values", func(t *testing.T) {
		// Create plugin with special characters in namespace
		specialPlugin := services.CreatePluginRequest{
			Name:      "Special Characters Plugin",
			Slug:      "special-chars-plugin",
			Command:   "/usr/local/bin/special-plugin",
			HookType:  "pre_auth",
			IsActive:  true,
			Namespace: "tenant-with-dashes_and_underscores",
		}

		w := performRequest(api.router, "POST", "/api/v1/plugins", specialPlugin)
		assert.Equal(t, http.StatusCreated, w.Code)

		var createResponse struct {
			Data PluginResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &createResponse)
		assert.NoError(t, err)
		specialPluginID := createResponse.Data.ID

		// Test filtering by namespace with special characters
		w = performRequest(api.router, "GET", "/api/v1/plugins?namespace=tenant-with-dashes_and_underscores", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var listResponse PluginListResponse
		err = json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(listResponse.Data), 1, "Should find plugin with special characters in namespace")

		// Verify the plugin is found
		found := false
		for _, plugin := range listResponse.Data {
			if plugin.ID == specialPluginID {
				found = true
				assert.Equal(t, "tenant-with-dashes_and_underscores", plugin.Attributes.Namespace)
				break
			}
		}
		assert.True(t, found, "Plugin with special characters in namespace should be found")

		createdPlugins = append(createdPlugins, specialPluginID)
	})

	// Cleanup all created plugins
	for _, pluginID := range createdPlugins {
		w := performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", pluginID), nil)
		// Don't assert on delete status as some might be already deleted or fail due to dependencies
		if w.Code != http.StatusNoContent && w.Code != http.StatusNotFound {
			t.Logf("Warning: Failed to delete plugin %s, status: %d", pluginID, w.Code)
		}
	}
}

func TestPluginHookTypes(t *testing.T) {
	api, _ := setupTestAPI(t)

	hookTypes := []string{"pre_auth", "auth", "post_auth", "on_response", "data_collection"}
	pluginIDs := make([]string, 0)

	// Create plugins for each hook type
	for _, hookType := range hookTypes {
		createPluginInput := services.CreatePluginRequest{
			Name:     fmt.Sprintf("%s Plugin", hookType),
			Slug:     fmt.Sprintf("%s-plugin", hookType),
			Command:  fmt.Sprintf("/usr/local/bin/%s-plugin", hookType),
			HookType: hookType,
			IsActive: true,
		}

		w := performRequest(api.router, "POST", "/api/v1/plugins", createPluginInput)
		assert.Equal(t, http.StatusCreated, w.Code)

		var response struct {
			Data PluginResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, hookType, response.Data.Attributes.HookType)
		pluginIDs = append(pluginIDs, response.Data.ID)
	}

	// Test filtering by each hook type
	for _, hookType := range hookTypes {
		w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/plugins?hook_type=%s", hookType), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var listResponse PluginListResponse
		err := json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(listResponse.Data), 1, fmt.Sprintf("Should find at least one %s plugin", hookType))

		// Verify all returned plugins have the correct hook type
		for _, plugin := range listResponse.Data {
			assert.Equal(t, hookType, plugin.Attributes.HookType)
		}
	}

	// Cleanup
	for _, id := range pluginIDs {
		performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/plugins/%s", id), nil)
	}
}

// Helper functions for pointer types (needed for update requests)
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}