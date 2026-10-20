package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/agenttracing"
)

// recordingTracing is an Enterprise-like service that records what the
// handlers pass it.
type recordingTracing struct {
	err       error
	sessionQ  agenttracing.SessionQuery
	spanQ     agenttracing.SpanQuery
	searchQ   agenttracing.SearchQuery
	sessionID string
	id        []byte
	viewer    agenttracing.Viewer
	calls     int
}

func (r *recordingTracing) Start(context.Context) error { return nil }
func (r *recordingTracing) Stop()                       {}
func (r *recordingTracing) Running() bool               { return true }
func (r *recordingTracing) IsEnterpriseAvailable() bool { return true }

func (r *recordingTracing) result(v agenttracing.Viewer) (any, error) {
	r.calls++
	r.viewer = v
	if r.err != nil {
		return nil, r.err
	}
	return map[string]any{"ok": true}, nil
}
func (r *recordingTracing) ListSessions(_ context.Context, v agenttracing.Viewer, q agenttracing.SessionQuery) (any, error) {
	r.sessionQ = q
	return r.result(v)
}
func (r *recordingTracing) GetSession(_ context.Context, v agenttracing.Viewer, id string) (any, error) {
	r.sessionID = id
	return r.result(v)
}
func (r *recordingTracing) ListSessionSpans(_ context.Context, v agenttracing.Viewer, id string, q agenttracing.SpanQuery) (any, error) {
	r.sessionID, r.spanQ = id, q
	return r.result(v)
}
func (r *recordingTracing) GetTrace(_ context.Context, v agenttracing.Viewer, id []byte, q agenttracing.SpanQuery) (any, error) {
	r.id, r.spanQ = id, q
	return r.result(v)
}
func (r *recordingTracing) GetSpan(_ context.Context, v agenttracing.Viewer, id []byte) (any, error) {
	r.id = id
	return r.result(v)
}
func (r *recordingTracing) SearchSession(_ context.Context, v agenttracing.Viewer, id string, q agenttracing.SearchQuery) (any, error) {
	r.sessionID, r.searchQ = id, q
	return r.result(v)
}

const testSessionID = "0192a4c8-7f00-7000-8000-000000000001"

// skipWithEnterpriseTracing skips a Community-only test in a build that
// links the Enterprise implementation (make ent-link, -tags enterprise).
func skipWithEnterpriseTracing(t *testing.T) {
	t.Helper()
	if agenttracing.IsEnterpriseAvailable() {
		t.Skip("Community Edition behaviour; the Enterprise build answers 503 while tracing is off")
	}
}

func withTracing(t *testing.T, svc agenttracing.Service) *API {
	t.Helper()
	api, _ := setupTestAPI(t)
	api.service.AgentTracing = svc
	return api
}

// The Community stub answers every read route with 402 and the
// enterprise_required code; no route is reachable without the service.
func TestAgentTracingRoutes_CommunityAnswers402(t *testing.T) {
	skipWithEnterpriseTracing(t)
	api, _ := setupTestAPI(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/agent-tracing/sessions"},
		{"POST", "/api/v1/agent-tracing/sessions/query"},
		{"GET", "/api/v1/agent-tracing/sessions/" + testSessionID},
		{"GET", "/api/v1/agent-tracing/sessions/" + testSessionID + "/spans"},
		{"POST", "/api/v1/agent-tracing/sessions/" + testSessionID + "/search"},
		{"GET", "/api/v1/agent-tracing/traces/" + strings.Repeat("ab", 16)},
		{"GET", "/api/v1/agent-tracing/spans/" + strings.Repeat("cd", 8)},
	} {
		var body any
		if r.method == "POST" {
			body = map[string]any{"q": "find"}
		}
		w := performRequest(api.router, r.method, r.path, body)
		assert.Equal(t, http.StatusPaymentRequired, w.Code, "%s %s: %s", r.method, r.path, w.Body.String())
		assert.Contains(t, w.Body.String(), authz.CodeEnterpriseRequired)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}

// Community Edition through the real middleware: an administrator reaches
// the stub (402), a non-administrator is refused before it (403).
func TestAgentTracingRoutes_CommunityRBAC(t *testing.T) {
	skipWithEnterpriseTracing(t)
	api, admin, member := setupAuthzAPI(t)
	path := "/api/v1/agent-tracing/sessions"
	w := apitest.PerformAuthRequest(api.router, "GET", path, nil, admin.APIKey)
	assert.Equal(t, http.StatusPaymentRequired, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(api.router, "GET", path, nil, member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "traces:read")
}

func TestAgentTracingRoutes_AnnotatedWithTracesRead(t *testing.T) {
	api, _ := setupTestAPI(t)
	for _, key := range []string{
		"GET /api/v1/agent-tracing/sessions",
		"POST /api/v1/agent-tracing/sessions/query",
		"GET /api/v1/agent-tracing/sessions/:session_id",
		"GET /api/v1/agent-tracing/sessions/:session_id/spans",
		"POST /api/v1/agent-tracing/sessions/:session_id/search",
		"GET /api/v1/agent-tracing/traces/:trace_id",
		"GET /api/v1/agent-tracing/spans/:span_id",
	} {
		e, ok := api.routePerms[key]
		require.True(t, ok, key)
		assert.Equal(t, authz.Read("traces"), e.perm, key)
	}
}

func TestAgentTracingSessions_QueryParsing(t *testing.T) {
	svc := &recordingTracing{}
	api := withTracing(t, svc)

	w := performRequest(api.router, "GET", "/api/v1/agent-tracing/sessions", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	q := svc.sessionQ
	assert.Equal(t, "started_at", q.Sort)
	assert.True(t, q.Desc)
	assert.Equal(t, agenttracing.DefaultSessionPageSize, q.Limit)
	assert.WithinDuration(t, time.Now().Add(-24*time.Hour), q.From, time.Minute, "default range is the last 24 h")
	assert.True(t, svc.viewer.AllApps)

	w = performRequest(api.router, "GET", "/api/v1/agent-tracing/sessions?from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z"+
		"&app_id=7&user_id=3&status=error&source=inferred&weak_links=true&has_errors=true&has_gateway_events=false"+
		"&cost_min_micros=10&duration_max_ms=5000&model=gpt-4.1&tool=search&agent=planner&sort=cost&limit=200&cursor=abc", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	q = svc.sessionQ
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), q.From)
	assert.Equal(t, uint(7), q.AppID)
	assert.Equal(t, uint(3), q.UserID)
	assert.Equal(t, "error", q.Status)
	assert.Equal(t, "inferred", q.Source)
	assert.True(t, q.WeakLinks)
	assert.True(t, *q.HasErrors)
	assert.False(t, *q.HasGatewayEvents)
	assert.EqualValues(t, 10, *q.CostMinMicros)
	assert.EqualValues(t, 5000, *q.DurationMaxMs)
	assert.Equal(t, "gpt-4.1", q.Model)
	assert.Equal(t, "cost", q.Sort)
	assert.False(t, q.Desc)
	assert.Equal(t, 200, q.Limit)
	assert.Equal(t, "abc", q.Cursor)

	for _, bad := range []string{
		"limit=201", "limit=0", "limit=x", "sort=name", "status=open", "source=guess",
		"from=yesterday", "from=2026-10-02T00:00:00Z&to=2026-10-01T00:00:00Z",
		"session_id=not-a-uuid", "cost_min_micros=-1", "has_errors=maybe", "app_id=-2",
		"agent=" + strings.Repeat("a", 256),
	} {
		calls := svc.calls
		w = performRequest(api.router, "GET", "/api/v1/agent-tracing/sessions?"+bad, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", bad, w.Body.String())
		assert.Equal(t, calls, svc.calls, "%s reached the service", bad)
	}
}

// end_user and external_id may be personal data: refused in the URL,
// accepted in the POST body.
func TestAgentTracingSessions_PersonalFiltersOnlyInBody(t *testing.T) {
	svc := &recordingTracing{}
	api := withTracing(t, svc)
	for _, p := range []string{"end_user=alice@example.com", "external_id=alice@example.com"} {
		w := performRequest(api.router, "GET", "/api/v1/agent-tracing/sessions?"+p, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "alice@example.com")
	}
	assert.Zero(t, svc.calls)

	w := performRequest(api.router, "POST", "/api/v1/agent-tracing/sessions/query",
		map[string]any{"end_user": "alice@example.com", "external_id": "conv-9", "sort": "-tokens", "limit": 20})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "alice@example.com", svc.sessionQ.EndUser)
	assert.Equal(t, "conv-9", svc.sessionQ.ExternalID)
	assert.Equal(t, "tokens", svc.sessionQ.Sort)
	assert.True(t, svc.sessionQ.Desc)

	w = performRequest(api.router, "POST", "/api/v1/agent-tracing/sessions/query", map[string]any{"limit": 500})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentTracingDetailRoutes(t *testing.T) {
	svc := &recordingTracing{}
	api := withTracing(t, svc)

	w := performRequest(api.router, "GET", "/api/v1/agent-tracing/sessions/"+testSessionID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, testSessionID, svc.sessionID)

	w = performRequest(api.router, "GET", "/api/v1/agent-tracing/sessions/"+testSessionID+"/spans?kind=tool_call&include=previews&limit=1000", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "tool_call", svc.spanQ.Kind)
	assert.True(t, svc.spanQ.Previews)
	assert.Equal(t, 1000, svc.spanQ.Limit)

	w = performRequest(api.router, "GET", "/api/v1/agent-tracing/traces/"+strings.Repeat("ab", 16), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, svc.id, 16)
	w = performRequest(api.router, "GET", "/api/v1/agent-tracing/spans/"+strings.Repeat("cd", 8), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, svc.id, 8)

	calls := svc.calls
	for _, bad := range []string{
		"/api/v1/agent-tracing/sessions/NOT-A-UUID",
		"/api/v1/agent-tracing/sessions/" + strings.ToUpper(testSessionID),
		"/api/v1/agent-tracing/sessions/" + testSessionID + "/spans?limit=1001",
		"/api/v1/agent-tracing/sessions/" + testSessionID + "/spans?include=content",
		"/api/v1/agent-tracing/traces/" + strings.Repeat("ab", 8),
		"/api/v1/agent-tracing/traces/" + strings.Repeat("AB", 16),
		"/api/v1/agent-tracing/spans/zz" + strings.Repeat("cd", 7),
	} {
		w = performRequest(api.router, "GET", bad, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, bad)
	}
	assert.Equal(t, calls, svc.calls)
}

func TestAgentTracingSearch_TermInBodyAndCapped(t *testing.T) {
	svc := &recordingTracing{}
	api := withTracing(t, svc)
	path := "/api/v1/agent-tracing/sessions/" + testSessionID + "/search"

	w := performRequest(api.router, "POST", path, map[string]any{"q": "  web_search  "})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "web_search", svc.searchQ.Term)
	assert.Equal(t, agenttracing.DefaultSearchResults, svc.searchQ.Limit)

	for _, body := range []map[string]any{
		{"q": ""}, {"q": strings.Repeat("x", agenttracing.MaxSearchTermLength+1)}, {"q": "x", "limit": 201},
	} {
		w = performRequest(api.router, "POST", path, body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	}
	// A GET with ?q= is not a route at all.
	w = performRequest(api.router, "GET", path+"?q=secret", nil)
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestAgentTracingErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{agenttracing.ErrNotFound, http.StatusNotFound},
		{agenttracing.BadRequest("cursor is not valid"), http.StatusBadRequest},
		{agenttracing.ErrDisabled, http.StatusServiceUnavailable},
		{agenttracing.ErrEnterpriseFeature, http.StatusPaymentRequired},
		{assert.AnError, http.StatusInternalServerError},
	} {
		api := withTracing(t, &recordingTracing{err: tc.err})
		w := performRequest(api.router, "GET", "/api/v1/agent-tracing/sessions/"+testSessionID, nil)
		assert.Equal(t, tc.code, w.Code, tc.err.Error())
		var body ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotEmpty(t, body.Errors)
		if tc.code == http.StatusInternalServerError {
			assert.NotContains(t, w.Body.String(), assert.AnError.Error(), "internal errors are not echoed")
		}
	}
}
