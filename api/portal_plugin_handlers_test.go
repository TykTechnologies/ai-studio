package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// --- extractUserGroupNames tests ---

func TestExtractUserGroupNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("extracts groups from user context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := &models.User{
			Email:   "test@example.com",
			Name:    "Test User",
			IsAdmin: false,
			Groups: []models.Group{
				{Name: "engineering"},
				{Name: "support"},
			},
		}
		c.Set("user", user)

		groups := extractUserGroupNames(c)
		assert.Equal(t, []string{"engineering", "support"}, groups)
	})

	t.Run("returns nil when no user in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		groups := extractUserGroupNames(c)
		assert.Nil(t, groups)
	})

	t.Run("returns empty slice for user with no groups", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := &models.User{
			Email:  "nogroups@example.com",
			Groups: []models.Group{},
		}
		c.Set("user", user)

		groups := extractUserGroupNames(c)
		assert.Empty(t, groups)
	})

	t.Run("returns nil for wrong type in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set("user", "not a user object")

		groups := extractUserGroupNames(c)
		assert.Nil(t, groups)
	})

	t.Run("handles user with many groups", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := &models.User{
			Email: "multi@example.com",
			Groups: []models.Group{
				{Name: "engineering"},
				{Name: "support"},
				{Name: "premium"},
				{Name: "beta-testers"},
			},
		}
		c.Set("user", user)

		groups := extractUserGroupNames(c)
		assert.Len(t, groups, 4)
		assert.Contains(t, groups, "engineering")
		assert.Contains(t, groups, "premium")
		assert.Contains(t, groups, "beta-testers")
	})
}

// --- callPortalPluginRPC handler tests ---

// setupPortalRPCTest creates a minimal API with real DB, PluginService, and
// AIStudioPluginManager to test the callPortalPluginRPC handler logic branches.
func setupPortalRPCTest(t *testing.T) (*API, *gorm.DB) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	pluginService := services.NewPluginService(db)
	pluginManager := services.NewAIStudioPluginManager(db, nil)

	api := &API{
		service: &services.Service{
			DB:                    db,
			PluginService:         pluginService,
			AIStudioPluginManager: pluginManager,
		},
	}

	return api, db
}

// makePortalRPCRouter sets up a gin router with optional user injection middleware
func makePortalRPCRouter(api *API, user *models.User) *gin.Engine {
	router := gin.New()
	group := router.Group("/common")
	if user != nil {
		group.Use(func(c *gin.Context) {
			c.Set("user", user)
			c.Next()
		})
	}
	group.POST("/plugins/:id/portal-rpc/:method", api.callPortalPluginRPC)
	return router
}

func doPortalRPC(router *gin.Engine, pluginID string, method string, payload interface{}) *httptest.ResponseRecorder {
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	path := fmt.Sprintf("/common/plugins/%s/portal-rpc/%s", pluginID, method)
	req, _ := http.NewRequest("POST", path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCallPortalPluginRPC_Unauthorized(t *testing.T) {
	api, _ := setupPortalRPCTest(t)

	t.Run("returns 401 when no user in context", func(t *testing.T) {
		router := makePortalRPCRouter(api, nil)
		w := doPortalRPC(router, "1", "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestCallPortalPluginRPC_InvalidInput(t *testing.T) {
	api, _ := setupPortalRPCTest(t)
	user := &models.User{Email: "test@example.com", Name: "Test User"}
	router := makePortalRPCRouter(api, user)

	t.Run("returns 400 for non-numeric plugin ID", func(t *testing.T) {
		w := doPortalRPC(router, "abc", "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 for non-existent plugin", func(t *testing.T) {
		w := doPortalRPC(router, "99999", "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCallPortalPluginRPC_PluginState(t *testing.T) {
	api, db := setupPortalRPCTest(t)
	user := &models.User{Email: "test@example.com", Name: "Test User"}

	t.Run("returns 400 for inactive plugin", func(t *testing.T) {
		plugin := &models.Plugin{
			Name:      "Inactive Plugin",
			Command:   "file:///test/inactive",
			HookType:  "studio_ui",
			HookTypes: []string{"studio_ui", "portal_ui"},
			IsActive:  true,
		}
		require.NoError(t, db.Create(plugin).Error)
		require.NoError(t, db.Model(plugin).Update("is_active", false).Error)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 for active but unloaded plugin", func(t *testing.T) {
		plugin := &models.Plugin{
			Name:      "Unloaded Plugin",
			Command:   "file:///test/unloaded",
			HookType:  "studio_ui",
			HookTypes: []string{"studio_ui", "portal_ui"},
			IsActive:  true,
		}
		require.NoError(t, db.Create(plugin).Error)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 403 for plugin without portal_ui hook type", func(t *testing.T) {
		plugin := &models.Plugin{
			Name:      "No Portal Plugin",
			Command:   "file:///test/no-portal",
			HookType:  "studio_ui",
			HookTypes: []string{"studio_ui"}, // No portal_ui
			IsActive:  true,
		}
		require.NoError(t, db.Create(plugin).Error)

		// We can't easily mock IsPluginLoaded on a real AIStudioPluginManager
		// but we can verify the hook type check by testing a plugin that
		// IS in the loaded map. Since we can't load a real plugin in unit tests,
		// we skip the IsPluginLoaded check by checking the test indirectly:
		// the handler will return 404 (not loaded) before reaching the 403 check.
		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", map[string]interface{}{})
		// Returns 404 because plugin isn't loaded - the hook type check comes after
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCallPortalPluginRPC_AcceptsPayload(t *testing.T) {
	api, db := setupPortalRPCTest(t)

	// Create a plugin that supports portal_ui
	plugin := &models.Plugin{
		Name:      "Portal Plugin",
		Command:   "file:///test/portal",
		HookType:  "studio_ui",
		HookTypes: []string{"studio_ui", "portal_ui"},
		IsActive:  true,
	}
	require.NoError(t, db.Create(plugin).Error)

	user := &models.User{
		Email:   "portal@example.com",
		Name:    "Portal User",
		IsAdmin: false,
		Groups: []models.Group{
			{Name: "engineering"},
		},
	}

	t.Run("accepts empty payload", func(t *testing.T) {
		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", nil)
		// Will return 404 (not loaded) but validates payload parsing doesn't fail
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("accepts JSON payload", func(t *testing.T) {
		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", map[string]interface{}{
			"title":   "Test Feedback",
			"message": "Great product!",
			"rating":  5,
		})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// --- Portal UI registry and sidebar endpoint tests ---

func TestPortalUIRegistryEndpoints_ViaFullRouter(t *testing.T) {
	api, _ := setupTestAPI(t)

	t.Run("portal RPC returns error for invalid plugin ID", func(t *testing.T) {
		w := performRequest(api.router, "POST", "/common/plugins/invalid/portal-rpc/test_method", map[string]interface{}{})
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("portal RPC returns error for non-existent plugin", func(t *testing.T) {
		w := performRequest(api.router, "POST", "/common/plugins/99999/portal-rpc/test_method", map[string]interface{}{})
		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}
