package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// The agent tracing settings (spec §11) are accepted and stored on every
// edition; they only take effect with Enterprise agent tracing running.

func tracingAttrs(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var resp struct {
		Data struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp), string(body))
	return resp.Data.Attributes
}

// LLM: both fields round-trip, a PATCH that omits them keeps them, and an
// unknown mode is a 400. The opt-out defaults to false (propagation on).
func TestLLMTracingAttributes(t *testing.T) {
	api, db := setupTestAPI(t)
	body := map[string]any{"data": map[string]any{"type": "llms", "attributes": map[string]any{
		"name": "Tracing LLM", "vendor": "openai", "api_endpoint": "https://api.openai.com", "privacy_score": 5,
	}}}
	w := performRequest(api.router, http.MethodPost, "/api/v1/llms", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	attrs := tracingAttrs(t, w.Body.Bytes())
	assert.Equal(t, "", attrs["tracing_content_mode_max"])
	assert.Equal(t, false, attrs["tracing_propagation_opt_out"], "propagation stays on by default")

	var stored models.LLM
	require.NoError(t, db.Where("name = ?", "Tracing LLM").First(&stored).Error)
	path := "/api/v1/llms/" + idStr(stored.ID)
	patch := func(a map[string]any) *httptest.ResponseRecorder {
		return performRequest(api.router, http.MethodPatch, path, map[string]any{"data": map[string]any{"type": "llms", "attributes": a}})
	}

	w = patch(map[string]any{"tracing_content_mode_max": "metadata", "tracing_propagation_opt_out": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	attrs = tracingAttrs(t, w.Body.Bytes())
	assert.Equal(t, "metadata", attrs["tracing_content_mode_max"])
	assert.Equal(t, true, attrs["tracing_propagation_opt_out"])

	w = patch(map[string]any{"short_description": "edited"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	attrs = tracingAttrs(t, w.Body.Bytes())
	assert.Equal(t, "metadata", attrs["tracing_content_mode_max"], "a PATCH that omits it keeps the cap")
	assert.Equal(t, true, attrs["tracing_propagation_opt_out"], "a PATCH that omits it keeps the opt-out")

	require.NoError(t, db.First(&stored, stored.ID).Error)
	assert.Equal(t, "metadata", stored.TracingContentModeMax)
	assert.True(t, stored.TracingPropagationOptOut)

	w = patch(map[string]any{"tracing_content_mode_max": "everything"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// App: the settings round-trip, an update that omits them keeps them, and
// invalid values are refused.
func TestAppTracingAttribute(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "tracing-app@example.com", Name: "Tracing", IsAdmin: true, EmailVerified: true}
	require.NoError(t, user.Create(db))

	tracing := map[string]any{"enabled": true, "content_mode": "redacted", "max_request_override": "redacted",
		"session_idle_timeout": "2h", "payload_days": 3, "owner_may_view_content": false}
	body := map[string]any{"data": map[string]any{"type": "app", "attributes": map[string]any{
		"name": "Tracing App", "description": "d", "user_id": user.ID, "tracing": tracing,
	}}}
	w := performRequest(api.router, http.MethodPost, "/api/v1/apps", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	attrs := tracingAttrs(t, w.Body.Bytes())
	assert.Equal(t, tracing["content_mode"], attrs["tracing"].(map[string]any)["content_mode"])

	var stored models.App
	require.NoError(t, db.Where("name = ?", "Tracing App").First(&stored).Error)
	days := 3
	enabled := true
	want := models.AppTracingSettings{Enabled: &enabled, ContentMode: "redacted", MaxRequestOverride: "redacted",
		SessionIdleTimeout: "2h", PayloadDays: &days}
	assert.Equal(t, want, stored.Tracing)

	path := "/api/v1/apps/" + idStr(stored.ID)
	update := func(a map[string]any) *httptest.ResponseRecorder {
		a["name"], a["description"], a["user_id"] = "Tracing App", "d", user.ID
		return performRequest(api.router, http.MethodPatch, path, map[string]any{"data": map[string]any{"type": "app", "attributes": a}})
	}
	w = update(map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, db.First(&stored, stored.ID).Error)
	assert.Equal(t, want, stored.Tracing, "an update that omits tracing keeps it")

	for _, bad := range []map[string]any{
		{"content_mode": "loud"},
		{"max_request_override": "everything"},
		{"session_idle_timeout": "soon"},
		{"session_idle_timeout": "10s"},
		{"payload_days": 1000},
		// jyx.175: not available until the Portal trace views.
		{"owner_may_view_content": true},
	} {
		w = update(map[string]any{"tracing": bad})
		assert.Equal(t, http.StatusBadRequest, w.Code, "%v: %s", bad, w.Body.String())
	}

	w = update(map[string]any{"tracing": map[string]any{}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "owner_may_view_content", "never in responses")
	var raw *string
	require.NoError(t, db.Raw("SELECT tracing FROM apps WHERE id = ?", stored.ID).Scan(&raw).Error)
	assert.Nil(t, raw, "no overrides is stored as NULL")
}

// owner_may_view_content is a content-access grant: changing it needs
// traces:write on top of apps:write. Leaving it as stored does not.
func TestAppTracingOwnerContentNeedsTracesWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := &API{}
	ctxWith := func(perms ...authz.Permission) (*gin.Context, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		authz.WithContext(c, authz.NewContext(7, false, func() (authz.Set, error) { return authz.NewSet(perms...), nil }))
		return c, w
	}
	grant := &models.AppTracingSettings{OwnerMayViewContent: true}

	c, w := ctxWith(authz.Write("apps"))
	opt, ok := a.appTracingOption(c, grant, models.AppTracingSettings{})
	assert.False(t, ok)
	assert.Nil(t, opt)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "traces:write")

	stored := models.AppTracingSettings{ContentMode: "metadata", OwnerMayViewContent: true}
	c, w = ctxWith(authz.Write("apps"))
	_, ok = a.appTracingOption(c, &models.AppTracingSettings{ContentMode: "metadata", OwnerMayViewContent: true}, stored)
	assert.True(t, ok, "unchanged settings need no traces:write")
	assert.Equal(t, http.StatusOK, w.Code)

	c, _ = ctxWith(authz.Write("apps"))
	_, ok = a.appTracingOption(c, &models.AppTracingSettings{ContentMode: "metadata"}, stored)
	assert.False(t, ok, "revoking the grant is a change too")

	c, _ = ctxWith(authz.Write("apps"), authz.Write("traces"))
	opt, ok = a.appTracingOption(c, grant, models.AppTracingSettings{})
	assert.True(t, ok)
	assert.NotNil(t, opt)

	c, _ = ctxWith()
	opt, ok = a.appTracingOption(c, nil, models.AppTracingSettings{})
	assert.True(t, ok, "no tracing in the request leaves it alone")
	assert.Nil(t, opt)
}

// jyx.99: every App tracing field, not only the content grant, needs
// traces:write to change; apps:write alone widened capture and retention.
func TestAppTracingEveryFieldNeedsTracesWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := &API{}
	ctxWith := func(perms ...authz.Permission) (*gin.Context, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		authz.WithContext(c, authz.NewContext(7, false, func() (authz.Set, error) { return authz.NewSet(perms...), nil }))
		return c, w
	}
	on, days, fewer := true, 365, 3
	stored := models.AppTracingSettings{PayloadDays: &fewer}
	for name, change := range map[string]models.AppTracingSettings{
		"enabled":              {Enabled: &on, PayloadDays: &fewer},
		"content_mode":         {ContentMode: "full", PayloadDays: &fewer},
		"max_request_override": {MaxRequestOverride: "full", PayloadDays: &fewer},
		"session_idle_timeout": {SessionIdleTimeout: "2h", PayloadDays: &fewer},
		"payload_days":         {PayloadDays: &days},
		"cleared":              {},
	} {
		change := change
		c, w := ctxWith(authz.Write("apps"))
		_, ok := a.appTracingOption(c, &change, stored)
		assert.False(t, ok, "%s with apps:write only", name)
		assert.Equal(t, http.StatusForbidden, w.Code, name)

		c, _ = ctxWith(authz.Write("apps"), authz.Write("traces"))
		_, ok = a.appTracingOption(c, &change, stored)
		assert.True(t, ok, "%s with traces:write", name)
	}
	same := fewer
	c, _ := ctxWith(authz.Write("apps"))
	_, ok := a.appTracingOption(c, &models.AppTracingSettings{PayloadDays: &same}, stored)
	assert.True(t, ok, "an equal value behind another pointer is no change")
}

// Tools and datasources: the cap round-trips, an update that omits it keeps
// it, and an unknown mode is a 400.
func TestToolAndDatasourceTracingContentModeMax(t *testing.T) {
	api, db := setupTestAPI(t)

	t.Run("tool", func(t *testing.T) {
		tool := &models.Tool{Name: "Tracing Tool", ToolType: models.ToolTypeREST}
		require.NoError(t, tool.Create(db))
		path := "/api/v1/tools/" + idStr(tool.ID)
		put := func(a map[string]any) *httptest.ResponseRecorder {
			a["name"], a["tool_type"] = "Tracing Tool", models.ToolTypeREST
			return performRequest(api.router, http.MethodPatch, path, map[string]any{"data": map[string]any{"type": "tools", "attributes": a}})
		}
		w := put(map[string]any{"tracing_content_mode_max": "hashed"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "hashed", tracingAttrs(t, w.Body.Bytes())["tracing_content_mode_max"])

		w = put(map[string]any{"description": "edited"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "hashed", tracingAttrs(t, w.Body.Bytes())["tracing_content_mode_max"], "omitted keeps the cap")

		w = put(map[string]any{"tracing_content_mode_max": "nope"})
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

		w = put(map[string]any{"tracing_content_mode_max": ""})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, db.First(tool, tool.ID).Error)
		assert.Equal(t, "", tool.TracingContentModeMax, `"" removes the cap`)
	})

	t.Run("datasource", func(t *testing.T) {
		body := map[string]any{"data": map[string]any{"type": "datasources", "attributes": map[string]any{
			"name": "Tracing DS", "privacy_score": 1, "tracing_content_mode_max": "metadata",
		}}}
		w := performRequest(api.router, http.MethodPost, "/api/v1/datasources", body)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, "metadata", tracingAttrs(t, w.Body.Bytes())["tracing_content_mode_max"])

		var ds models.Datasource
		require.NoError(t, db.Where("name = ?", "Tracing DS").First(&ds).Error)
		assert.Equal(t, "metadata", ds.TracingContentModeMax)
		path := "/api/v1/datasources/" + idStr(ds.ID)
		update := func(a map[string]any) *httptest.ResponseRecorder {
			a["name"], a["privacy_score"] = "Tracing DS", 1
			return performRequest(api.router, http.MethodPatch, path, map[string]any{"data": map[string]any{"type": "datasources", "attributes": a}})
		}
		w = update(map[string]any{"short_description": "edited"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "metadata", tracingAttrs(t, w.Body.Bytes())["tracing_content_mode_max"], "omitted keeps the cap")

		w = update(map[string]any{"tracing_content_mode_max": "redacted"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "redacted", tracingAttrs(t, w.Body.Bytes())["tracing_content_mode_max"])

		w = update(map[string]any{"tracing_content_mode_max": "nope"})
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		w = performRequest(api.router, http.MethodPost, "/api/v1/datasources", map[string]any{"data": map[string]any{"type": "datasources",
			"attributes": map[string]any{"name": "Bad DS", "privacy_score": 1, "tracing_content_mode_max": "nope"}}})
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		var n int64
		require.NoError(t, db.Model(&models.Datasource{}).Where("name = ?", "Bad DS").Count(&n).Error)
		assert.Zero(t, n, "a refused cap leaves no datasource behind")
	})
}
