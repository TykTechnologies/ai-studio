package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mcpAnalyticsHarness struct {
	r        *gin.Engine
	adminKey string
	ownerKey string
	otherKey string
	server   uint
	app      uint
	q        string
}

func setupMCPAnalyticsAPI(t *testing.T) *mcpAnalyticsHarness {
	t.Helper()
	db := apitest.SetupTestDB(t)
	require.NoError(t, analytics.Migrate(db))
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	cfg.TestMode = false
	authService := apitest.SetupTestAuthService(db, service)
	authService.Config.TestMode = false
	a := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)

	mkUser := func(email string, admin bool) *models.User {
		u := models.NewUser()
		u.Email, u.Name, u.Password, u.IsAdmin, u.EmailVerified = email, email, "hash", admin, true
		require.NoError(t, u.Create(db))
		return u
	}
	admin := mkUser("admin@tyk.io", true)
	owner := mkUser("owner@tyk.io", false)
	other := mkUser("other@tyk.io", false)

	app := &models.App{Name: "Owner App", UserID: owner.ID}
	require.NoError(t, app.Create(db))
	conn := &models.TykConnection{Name: "dash", DashboardURL: "https://dash.example.com", Status: models.TykConnectionActive}
	require.NoError(t, db.Create(conn).Error)
	srv := &models.MCPServer{ConnectionID: &conn.ID, TykAPIID: "api-1", Name: "Weather", Slug: "weather"}
	require.NoError(t, db.Create(srv).Error)

	day := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	appID, srvID := app.ID, srv.ID
	rows := []models.MCPCallRecord{
		{Timestamp: day, ConnectionID: conn.ID, MCPServerID: &srvID, AppID: &appID, PrimitiveType: "tool", PrimitiveName: "forecast", ResponseCode: 200, LatencyTotalMs: 10},
		{Timestamp: day.Add(time.Minute), ConnectionID: conn.ID, MCPServerID: &srvID, AppID: &appID, PrimitiveType: "tool", PrimitiveName: "forecast", ResponseCode: 404, LatencyTotalMs: 30},
		{Timestamp: day.Add(2 * time.Minute), ConnectionID: conn.ID, MCPServerID: &srvID, PrimitiveType: "tool", PrimitiveName: "radar", ResponseCode: 200, JSONRPCErrorCode: -32000, LatencyTotalMs: 20},
	}
	require.NoError(t, db.Create(&rows).Error)

	return &mcpAnalyticsHarness{r: a.router, adminKey: admin.APIKey, ownerKey: owner.APIKey, otherKey: other.APIKey,
		server: srv.ID, app: app.ID, q: "?start_date=2026-10-20&end_date=2026-10-20"}
}

func getJSON(t *testing.T, r *gin.Engine, path, key string, out interface{}) int {
	t.Helper()
	w := apitest.PerformAuthRequest(r, "GET", path, nil, key)
	if out != nil && w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), out), w.Body.String())
	}
	return w.Code
}

func TestMCPAnalytics_AdminRoutes(t *testing.T) {
	h := setupMCPAnalyticsAPI(t)
	base := fmt.Sprintf("/api/v1/analytics/mcp/servers/%d", h.server)

	var sum analytics.MCPSummary
	require.Equal(t, http.StatusOK, getJSON(t, h.r, base+"/summary"+h.q, h.adminKey, &sum))
	assert.Equal(t, int64(3), sum.Calls)
	assert.Equal(t, int64(2), sum.Errors, "HTTP 404 and a JSON-RPC error")
	assert.InDelta(t, 20, sum.AvgMs, 1e-9)
	assert.InDelta(t, 20, sum.P50Ms, 1e-9)

	var ts analytics.MCPTimeseries
	require.Equal(t, http.StatusOK, getJSON(t, h.r, base+"/timeseries"+h.q+"&interval=hour", h.adminKey, &ts))
	assert.Equal(t, "hour", ts.Interval)
	require.Len(t, ts.Points, 1)
	assert.Equal(t, int64(3), ts.Points[0].Calls)

	var tools MCPToolStatsPage
	require.Equal(t, http.StatusOK, getJSON(t, h.r, base+"/tools"+h.q+"&page_size=1", h.adminKey, &tools))
	assert.Equal(t, int64(2), tools.Meta.TotalCount)
	assert.Equal(t, 2, tools.Meta.TotalPages)
	require.Len(t, tools.Data, 1)
	assert.Equal(t, "forecast", tools.Data[0].PrimitiveName)

	var apps MCPAppStatsPage
	require.Equal(t, http.StatusOK, getJSON(t, h.r, base+"/apps"+h.q, h.adminKey, &apps))
	require.Len(t, apps.Data, 2)
	require.NotNil(t, apps.Data[0].AppID)
	assert.Equal(t, "Owner App", apps.Data[0].AppName)
	assert.Nil(t, apps.Data[1].AppID)

	var usage MCPAppUsage
	require.Equal(t, http.StatusOK, getJSON(t, h.r, fmt.Sprintf("/api/v1/analytics/mcp/apps/%d%s", h.app, h.q), h.adminKey, &usage))
	require.Len(t, usage.Servers, 1)
	assert.Equal(t, "Weather", usage.Servers[0].ServerName)
	assert.Equal(t, int64(2), usage.Servers[0].Calls)

	var ov analytics.MCPOverview
	require.Equal(t, http.StatusOK, getJSON(t, h.r, "/api/v1/analytics/mcp/overview"+h.q, h.adminKey, &ov))
	assert.Equal(t, int64(3), ov.Calls)
	require.NotEmpty(t, ov.TopTools)

	// Parameters are validated.
	assert.Equal(t, http.StatusBadRequest, getJSON(t, h.r, base+"/summary", h.adminKey, nil))
	assert.Equal(t, http.StatusBadRequest, getJSON(t, h.r, base+"/summary?start_date=20-10-2026&end_date=2026-10-20", h.adminKey, nil))
	assert.Equal(t, http.StatusBadRequest, getJSON(t, h.r, base+"/timeseries"+h.q+"&interval=week", h.adminKey, nil))
	assert.Equal(t, http.StatusBadRequest, getJSON(t, h.r, "/api/v1/analytics/mcp/servers/x/summary"+h.q, h.adminKey, nil))

	// Analytics read is required: a portal user is refused every admin route.
	for _, p := range []string{
		"/api/v1/analytics/mcp/overview",
		base + "/summary", base + "/timeseries", base + "/tools", base + "/apps",
		fmt.Sprintf("/api/v1/analytics/mcp/apps/%d", h.app),
	} {
		assert.Equal(t, http.StatusForbidden, getJSON(t, h.r, p+h.q, h.ownerKey, nil), p)
	}
}

func TestMCPAnalytics_PortalAppUsage(t *testing.T) {
	h := setupMCPAnalyticsAPI(t)
	path := fmt.Sprintf("/common/apps/%d/analytics/mcp%s", h.app, h.q)

	var usage MCPAppUsage
	require.Equal(t, http.StatusOK, getJSON(t, h.r, path, h.ownerKey, &usage))
	require.Len(t, usage.Servers, 1)
	assert.Equal(t, int64(2), usage.Servers[0].Calls, "only the App's own calls")

	// Another user (even one who knows the id) gets nothing.
	assert.Equal(t, http.StatusForbidden, getJSON(t, h.r, path, h.otherKey, nil))
	assert.Equal(t, http.StatusNotFound, getJSON(t, h.r, "/common/apps/99999/analytics/mcp"+h.q, h.ownerKey, nil))
	assert.Equal(t, http.StatusUnauthorized, apitest.PerformRequest(h.r, "GET", path, nil).Code)
}
