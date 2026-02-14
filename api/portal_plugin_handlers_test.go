package api

import (
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/gin-gonic/gin"
)

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

func TestPortalPluginEndpoints_BasicValidation(t *testing.T) {
	api, _ := setupTestAPI(t)

	t.Run("portal RPC returns error for invalid plugin ID", func(t *testing.T) {
		w := performRequest(api.router, "POST", "/common/plugins/invalid/portal-rpc/test_method", map[string]interface{}{})
		// Should get an error response (400 or 500 depending on service availability)
		assert.NotEqual(t, 200, w.Code)
	})

	t.Run("portal RPC returns error for non-existent plugin", func(t *testing.T) {
		w := performRequest(api.router, "POST", "/common/plugins/99999/portal-rpc/test_method", map[string]interface{}{})
		assert.NotEqual(t, 200, w.Code)
	})
}
