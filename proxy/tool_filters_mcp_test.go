//go:build enterprise

package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPToolCall_InputFilterBlocksBeforeUpstream(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"should never be reached"}`)
	defer f.teardown()

	f.attachFilter(t, "deny-input", `output := {block: true, message: "no PII to tools"}`, false)

	text, isError := newMCPSession(t, f).callTool(t, "getTestData", map[string]interface{}{})

	assert.True(t, isError, "a blocked MCP tool call must come back as a tool error")
	assert.Equal(t, toolBlockedMessage, text)
	assert.NotContains(t, text, "no PII to tools", "the filter's reason must not leak to the caller")
	assert.Empty(t, f.upstream.ReceivedRequests,
		"a blocked MCP tool call must never reach the downstream tool")
}

func TestMCPToolCall_OutputFilterBlocksResponse(t *testing.T) {
	f := newToolFilterFixture(t, `{"ssn":"123-45-6789"}`)
	defer f.teardown()

	f.attachFilter(t, "deny-output", `output := {block: true, message: "sensitive"}`, true)

	text, isError := newMCPSession(t, f).callTool(t, "getTestData", map[string]interface{}{})

	assert.True(t, isError)
	assert.Equal(t, toolBlockedMessage, text)
	assert.NotContains(t, text, "123-45-6789", "a blocked tool response must not reach the caller")
}

func TestMCPToolCall_OutputFilterRedactsResponse(t *testing.T) {
	f := newToolFilterFixture(t, `{"ssn":"123-45-6789"}`)
	defer f.teardown()

	f.attachFilter(t, "redact-output", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "123-45-6789", "[REDACTED]", -1)}`, true)

	text, isError := newMCPSession(t, f).callTool(t, "getTestData", map[string]interface{}{})

	assert.False(t, isError)
	assert.Contains(t, text, "[REDACTED]")
	assert.NotContains(t, text, "123-45-6789")
}

func TestMCPToolCall_FilterErrorFailsClosed(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "broken", `output := input.nope.deeper`, false)

	text, isError := newMCPSession(t, f).callTool(t, "getTestData", map[string]interface{}{})

	assert.True(t, isError)
	assert.Equal(t, toolBlockedMessage, text)
	assert.Empty(t, f.upstream.ReceivedRequests)
}

func TestMCPToolCall_UnfilteredToolIsUnaffected(t *testing.T) {
	f := newToolFilterFixture(t, `{"message":"GET success"}`)
	defer f.teardown()

	text, isError := newMCPSession(t, f).callTool(t, "getTestData", map[string]interface{}{})

	assert.False(t, isError)
	assert.Contains(t, text, "GET success")
	require.Len(t, f.upstream.ReceivedRequests, 1)
}

// A filter attached after the MCP server was cached must still apply: the
// cached handler closure captured the tool as it was, so the tool loaded for
// the current request has to win.
func TestMCPToolCall_FilterAttachedAfterServerCached(t *testing.T) {
	f := newToolFilterFixture(t, `{"ssn":"123-45-6789"}`)
	defer f.teardown()

	// First call with no filters primes the per-tool MCP server cache.
	session := newMCPSession(t, f)
	text, isError := session.callTool(t, "getTestData", map[string]interface{}{})
	require.False(t, isError)
	require.Contains(t, text, "123-45-6789")

	f.attachFilter(t, "deny-output-late", `output := {block: true, message: "sensitive"}`, true)

	text, isError = newMCPSession(t, f).callTool(t, "getTestData", map[string]interface{}{})

	assert.True(t, isError, "a newly attached filter must apply to a cached MCP server")
	assert.Equal(t, toolBlockedMessage, text)
}

// The input rewrite has to reach the downstream tool, not just be computed.
func TestMCPToolCall_InputFilterRedactsBeforeUpstream(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "redact-input", `
json := import("json")
decoded := json.decode(input.raw_input)
decoded.payload = {note: "[REDACTED]"}
output := {block: false, payload: string(json.encode(decoded))}`, false)

	_, isError := newMCPSession(t, f).callTool(t, "submitTestData", map[string]interface{}{})

	require.False(t, isError)
	require.Len(t, f.upstream.ReceivedRequests, 1)
	assert.Contains(t, string(f.upstream.ReceivedRequests[0].Body), "[REDACTED]")
}

// Both directions guarding the same tool, over MCP.
func TestMCPToolCall_BothDirectionsOnOneTool(t *testing.T) {
	f := newToolFilterFixture(t, `{"reply":"secret-out"}`)
	defer f.teardown()

	f.attachFilter(t, "mark-in", `
json := import("json")
decoded := json.decode(input.raw_input)
decoded.payload = {marker: "IN"}
output := {block: false, payload: string(json.encode(decoded))}`, false)
	f.attachFilter(t, "redact-out", `
text := import("text")
output := {block: false, payload: text.replace(input.raw_input, "secret-out", "[OUT]", -1)}`, true)

	text, isError := newMCPSession(t, f).callTool(t, "submitTestData", map[string]interface{}{})

	require.False(t, isError)
	require.Len(t, f.upstream.ReceivedRequests, 1)
	assert.Contains(t, string(f.upstream.ReceivedRequests[0].Body), "IN")
	assert.Contains(t, text, "[OUT]")
	assert.NotContains(t, text, "secret-out")
}

// The SSE transport is a separate handler pair from StreamableHTTP, and it
// carries the resolved tool onto the context by the same mechanism. Its
// message endpoint must reject an unknown session rather than serve a tool
// call that skipped the handshake.
func TestMCPToolSSE_MessageEndpointIsReachableAndGoverned(t *testing.T) {
	f := newToolFilterFixture(t, `{"ok":true}`)
	defer f.teardown()

	f.attachFilter(t, "deny-input", `output := {block: true, message: "nope"}`, false)

	req, err := http.NewRequest(http.MethodPost,
		"/tools/"+testToolSlug+"/mcp/message?sessionId=does-not-exist",
		bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"getTestData","arguments":{}}}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)

	// The tool resolves and the handler runs (not a 404/401 on the route
	// itself), and no tool call escapes to the upstream.
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "the SSE message route must resolve the tool")
	assert.NotEqual(t, http.StatusUnauthorized, rr.Code)
	assert.Empty(t, f.upstream.ReceivedRequests,
		"an unhandshaken SSE message must not reach the downstream tool")
}
