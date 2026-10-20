package proxy

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The X-Tyk-* agent tracing headers never reach a vendor, on the REST and the
// streaming upstream paths, in every edition; compat headers and W3C trace
// context pass through (spec §4.3, bead jyx.13).
func TestAgentTracingHeadersAreStrippedUpstream(t *testing.T) {
	sent := []string{
		"X-Tyk-Session-Id", "conv-42",
		"X-Tyk-Session-Name", "triage",
		"X-Tyk-Trace-Id", "4bf92f3577b34da6a3ce929d0e0e4736",
		"X-Tyk-Span-Id", "00f067aa0ba90201",
		"X-Tyk-Parent-Span-Id", "00f067aa0ba90202",
		"X-Tyk-Agent-Name", "researcher",
		"X-Tyk-Agent-Id", "agent-1",
		"X-Tyk-Agent-Path", "/root/researcher",
		"X-Tyk-User-Id", "end-user",
		"X-Tyk-Trace-Capture", "full",
		"Helicone-Session-Id", "helicone-1",
		"Baggage", "session.id=s1",
	}
	for _, tc := range []struct{ name, path, body string }{
		{"rest", "/llm/rest/primary/v1/chat/completions", failoverChatBody},
		{"stream", "/llm/stream/primary/v1/chat/completions", failoverStreamBody},
		{"unified ingress", "/ai/primary/v1/chat/completions", failoverChatBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, serveOpenAIText("ok"), serveStatus(500), nil)
			resp, body := h.post(tc.path, tc.body, sent...)
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

			calls := h.primaryVendor.calls()
			require.Len(t, calls, 1)
			for _, k := range declared.TracingHeaders {
				assert.Empty(t, calls[0].Headers.Values(k), "%s reached the vendor", k)
			}
			if tc.name != "unified ingress" { // /ai/ forwards allowlisted headers only
				assert.Equal(t, "helicone-1", calls[0].Headers.Get("Helicone-Session-Id"), "compat headers pass through")
			}
		})
	}
}

func TestAgentTracingHeadersForwardedWhenConfigured(t *testing.T) {
	declared.SetForwardTykHeaders(true)
	t.Cleanup(func() { declared.SetForwardTykHeaders(false) })

	h := newFailoverHarness(t, serveOpenAIText("ok"), serveStatus(500), nil)
	resp, body := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody, "X-Tyk-Session-Id", "conv-42")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "conv-42", calls[0].Headers.Get("X-Tyk-Session-Id"))
}
