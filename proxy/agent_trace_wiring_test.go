package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// params._meta io.modelcontextprotocol/clientInfo is the declared MCP client
// (jyx.137).
func TestAgentTraceTool_MCPClientInfo(t *testing.T) {
	sink := withCapture(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	rr := session.postWith(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getTestData","arguments":{},`+
		`"_meta":{"io.modelcontextprotocol/clientInfo":{"name":"claude-code","version":"2.1.0"}}}}`, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assert.Equal(t, "claude-code/2.1.0", ev.Declared.ClientInfo)
}

// A streamed upstream's Content-Type reaches its capture while tracing is
// on; with tracing off the request is untouched (jyx.137).
func TestAgentTraceLLM_UpstreamContentType(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Content-Type": {"application/vnd.amazon.eventstream"}}}
	r := httptest.NewRequest(http.MethodPost, "/llm/stream/x", nil)

	withAgentTracing(t, false)
	if n := testing.AllocsPerRun(100, func() { _ = withUpstreamContentType(r, resp) }); n != 0 {
		t.Fatalf("tracing off allocates %v", n)
	}
	assert.Same(t, r, withUpstreamContentType(r, resp))

	withAgentTracing(t, true)
	ctx, _ := agenttrace.Begin(r.Context())
	r = r.WithContext(ctx)
	got := withUpstreamContentType(r, resp)
	assert.Equal(t, "application/vnd.amazon.eventstream", got.Context().Value(upstreamContentTypeKey{}))
}
