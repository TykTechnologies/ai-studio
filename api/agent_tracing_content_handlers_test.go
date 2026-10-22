package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/agenttracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The content half of recordingTracing (agent_tracing_handlers_test.go).

func (r *recordingTracing) GetPayload(_ context.Context, v agenttracing.Viewer, _ uint, id []byte, q agenttracing.PayloadQuery, m agenttracing.AccessMeta) (any, error) {
	r.id, r.payloadQ, r.meta = id, q, m
	return r.result(v)
}

func (r *recordingTracing) PurgeSession(_ context.Context, v agenttracing.Viewer, sessionID string, m agenttracing.AccessMeta) (any, error) {
	r.sessionID, r.meta = sessionID, m
	return r.result(v)
}

func (r *recordingTracing) PurgeEndUser(_ context.Context, v agenttracing.Viewer, endUser string, m agenttracing.AccessMeta) (any, error) {
	r.endUser, r.meta = endUser, m
	return r.result(v)
}

func (r *recordingTracing) GetConfig(_ context.Context, v agenttracing.Viewer) (any, error) {
	return r.result(v)
}

func TestAgentTracingContentRoutes_Annotated(t *testing.T) {
	api, _ := setupTestAPI(t)
	for key, perm := range map[string]authz.Permission{
		"GET /api/v1/agent-tracing/spans/:span_id/payloads/:part": authz.Read("trace-content"),
		"DELETE /api/v1/agent-tracing/sessions/:session_id":       authz.Delete("traces"),
		"POST /api/v1/agent-tracing/purge":                        authz.Delete("traces"),
		"GET /api/v1/agent-tracing/config":                        authz.Read("traces"),
		"GET /api/v1/agent-tracing/sessions/:session_id/export":   authz.Execute("traces"),
		"GET /api/v1/agent-tracing/traces/:trace_id/export":       authz.Execute("traces"),
	} {
		e, ok := api.routePerms[key]
		require.True(t, ok, key)
		assert.Equal(t, perm, e.perm, key)
	}
}

func TestAgentTracingContentRoutes_CommunityAnswers402(t *testing.T) {
	skipWithEnterpriseTracing(t)
	api, _ := setupTestAPI(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/agent-tracing/spans/" + strings.Repeat("cd", 8) + "/payloads/request?app_id=7"},
		{"DELETE", "/api/v1/agent-tracing/sessions/" + testSessionID},
		{"POST", "/api/v1/agent-tracing/purge"},
		{"GET", "/api/v1/agent-tracing/config"},
		{"GET", "/api/v1/agent-tracing/sessions/" + testSessionID + "/export?format=json"},
		{"GET", "/api/v1/agent-tracing/traces/" + strings.Repeat("ab", 16) + "/export?app_id=7&format=markdown"},
	} {
		var body any
		if r.method == "POST" {
			body = map[string]any{"end_user": "x"}
		}
		w := performRequest(api.router, r.method, r.path, body)
		assert.Equal(t, http.StatusPaymentRequired, w.Code, "%s %s: %s", r.method, r.path, w.Body.String())
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}

func TestAgentTracingPayload_Parsing(t *testing.T) {
	svc := &recordingTracing{}
	api := withTracing(t, svc)
	span := strings.Repeat("cd", 8)

	w := performRequest(api.router, "GET", "/api/v1/agent-tracing/spans/"+span+"/payloads/tool_result?app_id=7&max_bytes=4096", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, agenttracing.PayloadQuery{Part: "tool_result", MaxBytes: 4096}, svc.payloadQ)

	for _, bad := range []string{
		"/api/v1/agent-tracing/spans/zz/payloads/request?app_id=7",
		"/api/v1/agent-tracing/spans/" + span + "/payloads/request",
		"/api/v1/agent-tracing/spans/" + span + "/payloads/request?app_id=7&max_bytes=0",
		"/api/v1/agent-tracing/spans/" + span + "/payloads/request?app_id=7&max_bytes=lots",
	} {
		calls := svc.calls
		w = performRequest(api.router, "GET", bad, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, bad)
		assert.Equal(t, calls, svc.calls, bad)
	}
}

// Content the audit trail cannot record is refused (503), and a viewer the
// implementation refuses gets the standard 403 envelope.
func TestAgentTracingPayload_ErrorMapping(t *testing.T) {
	span := strings.Repeat("cd", 8)
	for err, code := range map[error]int{
		agenttracing.ErrAuditUnavailable: http.StatusServiceUnavailable,
		agenttracing.ErrForbidden:        http.StatusForbidden,
		agenttracing.ErrNotFound:         http.StatusNotFound,
	} {
		api := withTracing(t, &recordingTracing{err: err})
		w := performRequest(api.router, "GET", "/api/v1/agent-tracing/spans/"+span+"/payloads/request?app_id=7", nil)
		assert.Equal(t, code, w.Code, "%v: %s", err, w.Body.String())
	}
}

// The erased end user is accepted only in a body and never echoed back.
func TestAgentTracingPurge(t *testing.T) {
	svc := &recordingTracing{}
	api := withTracing(t, svc)

	w := performRequest(api.router, "DELETE", "/api/v1/agent-tracing/sessions/"+testSessionID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, testSessionID, svc.sessionID)

	w = performRequest(api.router, "DELETE", "/api/v1/agent-tracing/sessions/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = performRequest(api.router, "POST", "/api/v1/agent-tracing/purge", map[string]any{"end_user": " jane.doe@example.com "})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "jane.doe@example.com", svc.endUser)
	assert.NotContains(t, w.Body.String(), "jane.doe")

	w = performRequest(api.router, "POST", "/api/v1/agent-tracing/purge", "not json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

type exportingTracing struct {
	agenttracing.Service
	q   agenttracing.ExportQuery
	err error
}

func (e *exportingTracing) ExportSession(_ context.Context, _ agenttracing.Viewer, _ string, q agenttracing.ExportQuery, _ agenttracing.AccessMeta) (*agenttracing.ExportResult, error) {
	e.q = q
	if e.err != nil {
		return nil, e.err
	}
	return &agenttracing.ExportResult{ContentType: "text/markdown; charset=utf-8", Filename: "agent-session-x.md", Body: []byte("# x"), ContentIncluded: q.Content}, nil
}

// Exports are attachments that browsers must not sniff or cache; bad
// parameters never reach the service, and an unaudited export is refused.
func TestAgentTracingExport_Handler(t *testing.T) {
	svc := &exportingTracing{}
	api := withTracing(t, svc)
	path := "/api/v1/agent-tracing/sessions/" + testSessionID + "/export"

	w := performRequest(api.router, "GET", path+"?format=markdown&content=true", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, agenttracing.ExportQuery{Format: "markdown", Content: true}, svc.q)
	assert.Equal(t, `attachment; filename="agent-session-x.md"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "included", w.Header().Get("X-Tyk-Export-Content"))

	for _, bad := range []string{"?format=pdf", "?content=yes", "?format=json&content=1"} {
		svc.q = agenttracing.ExportQuery{}
		w = performRequest(api.router, "GET", path+bad, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, bad)
		assert.Equal(t, agenttracing.ExportQuery{}, svc.q, bad)
	}

	svc.err = agenttracing.ErrAuditUnavailable
	w = performRequest(api.router, "GET", path, nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "requires the audit trail")
}
