package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// w4e: only an administrator may set or change an asset's
// allow_internal_host; an omitted value keeps the stored one.
func TestAllowInternalHostUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := func(user *models.User) (*gin.Context, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		if user != nil {
			c.Set("user", user)
		}
		return c, w
	}
	on, off := true, false
	admin, member := &models.User{IsAdmin: true}, &models.User{}

	c, _ := ctx(member)
	v, ok := allowInternalHostUpdate(c, true, nil)
	assert.True(t, ok)
	assert.True(t, v, "omitted keeps the stored value, for anyone")
	v, ok = allowInternalHostUpdate(c, true, &on)
	assert.True(t, ok)
	assert.True(t, v, "resending the stored value is no change")

	for _, tc := range []struct {
		user   *models.User
		stored bool
		req    *bool
	}{{member, false, &on}, {member, true, &off}, {nil, false, &on}} {
		c, w := ctx(tc.user)
		v, ok := allowInternalHostUpdate(c, tc.stored, tc.req)
		assert.False(t, ok)
		assert.Equal(t, tc.stored, v)
		assert.Equal(t, http.StatusForbidden, w.Code)
	}

	c, _ = ctx(admin)
	v, ok = allowInternalHostUpdate(c, false, &on)
	assert.True(t, ok)
	assert.True(t, v)
}

// asAdmin returns a request function authenticated as a new administrator.
func asAdmin(t *testing.T, api *API, db *gorm.DB) func(method, path string, body any) *httptest.ResponseRecorder {
	admin := createTestUserWithSettings(t, api.service, "netguard-admin@example.com", "Admin", true, true, true, true, false)
	admin.APIKey = "netguard-admin-test-key"
	require.NoError(t, db.Save(admin).Error)
	return func(method, path string, body any) *httptest.ResponseRecorder {
		var buf []byte
		if body != nil {
			buf, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, path, bytes.NewBuffer(buf))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin.APIKey)
		w := httptest.NewRecorder()
		api.router.ServeHTTP(w, req)
		return w
	}
}

// Through the tool API (an administrator): set on create, kept by an update
// that omits it, cleared by one that sends false. Without a user (no
// administrator) setting it is refused.
func TestToolAPI_AllowInternalHost(t *testing.T) {
	api, db := setupTestAPI(t)
	do := asAdmin(t, api, db)
	spec := base64.StdEncoding.EncodeToString([]byte(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"servers":[{"url":"http://ollama.internal:8080"}],"paths":{}}`))
	body := func(attrs map[string]any) map[string]any {
		a := map[string]any{"name": "Internal API", "tool_type": models.ToolTypeREST, "oas_spec": spec, "privacy_score": 1}
		for k, v := range attrs {
			a[k] = v
		}
		return map[string]any{"data": map[string]any{"type": "tools", "attributes": a}}
	}
	stored := func(id uint) bool {
		var tool models.Tool
		require.NoError(t, db.First(&tool, id).Error)
		return tool.AllowInternalHost
	}

	w := performRequest(api.router, "POST", "/api/v1/tools", body(map[string]any{"allow_internal_host": true}))
	require.Equal(t, http.StatusForbidden, w.Code, "not an administrator: %s", w.Body.String())

	w = do("POST", "/api/v1/tools", body(map[string]any{"allow_internal_host": true}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var tool models.Tool
	require.NoError(t, db.Where("name = ?", "Internal API").First(&tool).Error)
	assert.True(t, stored(tool.ID))
	assert.Contains(t, w.Body.String(), `"allow_internal_host":true`)

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%d", tool.ID), body(nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, stored(tool.ID), "an update that omits it keeps it")

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%d", tool.ID), body(map[string]any{"allow_internal_host": false}))
	require.Equal(t, http.StatusForbidden, w.Code, "not an administrator: %s", w.Body.String())
	assert.True(t, stored(tool.ID))

	w = do("PATCH", fmt.Sprintf("/api/v1/tools/%d", tool.ID), body(map[string]any{"allow_internal_host": false}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, stored(tool.ID))
}
