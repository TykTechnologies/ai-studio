package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
)

func withPropagation(t *testing.T, llm, httpUpstreams, mcp bool) {
	t.Helper()
	tracing.SetOutboundPropagation(llm, httpUpstreams, mcp)
	t.Cleanup(func() { tracing.SetOutboundPropagation(true, true, true) })
}

func recordUpstreamHeaders(dst *http.Header, serve http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*dst = r.Header.Clone()
		serve(w, r)
	}
}

// AGENT_TRACING_PROPAGATE_LLM_PROVIDERS=false keeps the trace context away
// from LLM upstreams, the client's own included (jyx.110).
func TestAgentTraceSettings_LLMPropagationOff(t *testing.T) {
	withW3CPropagation(t)
	var upstream http.Header
	h := newFailoverHarness(t, recordUpstreamHeaders(&upstream, serveToolCall), serveOpenAIText("never"), nil)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest, "Traceparent", clientTraceparent)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.NotEmpty(t, upstream.Get("Traceparent"), "on by default")

	withPropagation(t, false, true, true)
	resp, body = h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest, "Traceparent", clientTraceparent)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Empty(t, upstream.Get("Traceparent"))
	assert.Empty(t, upstream.Get("Tracestate"))
}

// The caller's sampling decision reaches the upstream unchanged, a trace the
// gateway mints is sampled, and the caller's tracestate is not forwarded
// (jyx.89).
func TestAgentTraceSettings_SamplingAndTracestate(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	var upstream http.Header
	h := newFailoverHarness(t, recordUpstreamHeaders(&upstream, serveToolCall), serveOpenAIText("never"), nil)

	unsampled := "00-" + clientTraceID + "-00f067aa0ba902b7-00"
	resp, body := h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest,
		"Traceparent", unsampled, "Tracestate", "acme=c2VjcmV0LXRva2Vu")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	tp := upstream.Get("Traceparent")
	assert.Equal(t, clientTraceID, traceIDOf(tp))
	assert.True(t, strings.HasSuffix(tp, "-00"), "the caller's not-sampled flag stands: %s", tp)
	assert.Empty(t, upstream.Get("Tracestate"), "the caller's tracestate is not forwarded")

	resp, body = h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.True(t, strings.HasSuffix(upstream.Get("Traceparent"), "-01"), "a minted trace is sampled")
}

// AGENT_TRACING_PROPAGATE_HTTP_UPSTREAMS=false: tool upstreams get no
// trace context.
func TestAgentTraceSettings_HTTPPropagationOff(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	withPropagation(t, true, false, true)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	_, isErr := session.callTool(t, "getTestData", map[string]interface{}{})
	require.False(t, isErr)
	assert.Empty(t, f.lastUpstreamHeaders(t).Get("Traceparent"))
}

// With tracing on, an OpenAI upstream gets the gateway span id as
// X-Client-Request-Id unless AGENT_TRACING_OPENAI_CLIENT_REQUEST_ID is off.
func TestAgentTraceSettings_OpenAIClientRequestID(t *testing.T) {
	sink := withCapture(t)
	var upstream http.Header
	h := newFailoverHarness(t, recordUpstreamHeaders(&upstream, serveToolCall), serveOpenAIText("never"), nil)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	ev := waitLLMEvent(t, sink)
	assert.Equal(t, ev.SpanID.String(), upstream.Get("X-Client-Request-Id"))

	SetOpenAIClientRequestID(false)
	t.Cleanup(func() { SetOpenAIClientRequestID(true) })
	resp, body = h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Empty(t, upstream.Get("X-Client-Request-Id"))

	// The client's own id is never replaced.
	SetOpenAIClientRequestID(true)
	resp, body = h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest, "X-Client-Request-Id", "client-1")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, "client-1", upstream.Get("X-Client-Request-Id"))

	// With propagation to LLM providers off the span id stays home too; a
	// client-set value still passes (jyx.145).
	withPropagation(t, false, true, true)
	resp, body = h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Empty(t, upstream.Get("X-Client-Request-Id"))
	resp, body = h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest, "X-Client-Request-Id", "client-2")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, "client-2", upstream.Get("X-Client-Request-Id"))
}

// AGENT_TRACING_MAX_STREAM_BUFFER_BYTES sets the streamed capture's
// argument cap.
func TestAgentTraceSettings_StreamBufferBytes(t *testing.T) {
	t.Cleanup(func() { SetAgentTraceStreamBufferBytes(0) })
	assert.Equal(t, 1<<20, newStreamCapture("text/event-stream", "openai", 0, nil).maxArgs)
	SetAgentTraceStreamBufferBytes(4096)
	assert.Equal(t, 4096, newStreamCapture("text/event-stream", "openai", 0, nil).maxArgs)
	assert.Equal(t, 10, newStreamCapture("text/event-stream", "openai", 10, nil).maxArgs, "an explicit cap wins")
}
