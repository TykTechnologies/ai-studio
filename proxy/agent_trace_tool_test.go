package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *toolFilterFixture) lastUpstreamHeaders(t *testing.T) http.Header {
	t.Helper()
	f.upstream.mu.Lock()
	defer f.upstream.mu.Unlock()
	require.NotEmpty(t, f.upstream.ReceivedRequests, "the tool upstream was not called")
	return f.upstream.ReceivedRequests[len(f.upstream.ReceivedRequests)-1].Headers
}

// A REST tool call carries the request context to the tool upstream: with
// agent tracing on, the upstream sees the caller's trace with the gateway
// span as parent, and baggage cut to the allowlist.
func TestAgentTraceTool_RESTCallCarriesTraceparent(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	body, err := json.Marshal(map[string]interface{}{
		"operation_id": "getTestData",
		"parameters":   map[string][]string{},
		"payload":      map[string]interface{}{},
		"headers":      map[string][]string{"baggage": {"session.id=s1,secret=x"}},
	})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "/tools/"+testToolSlug, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Traceparent", clientTraceparent)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	h := f.lastUpstreamHeaders(t)
	tp := h.Get("Traceparent")
	assert.Equal(t, clientTraceID, traceIDOf(tp), "the tool call continues the caller's trace")
	assert.NotContains(t, tp, "00f067aa0ba902b7", "the gateway span, not the client's, is the parent")
	assert.Equal(t, "session.id=s1", h.Get("Baggage"))
}

// The MCP shim's tools/call reaches the REST upstream with a traceparent too.
func TestAgentTraceTool_MCPCallCarriesTraceparent(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	_, isErr := session.callTool(t, "getTestData", map[string]interface{}{})
	require.False(t, isErr)

	tp := f.lastUpstreamHeaders(t).Get("Traceparent")
	require.NotEmpty(t, tp, "the MCP tool call carries a traceparent")
	assert.Len(t, traceIDOf(tp), 32)
}
