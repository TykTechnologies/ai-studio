package proxy

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
)

const (
	clientTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	clientTraceparent = "00-" + clientTraceID + "-00f067aa0ba902b7-01"
)

// withW3CPropagation installs the propagator pkg/tracing.Init installs in
// production (tests do not run Init).
func withW3CPropagation(t *testing.T) {
	t.Helper()
	requireSerial(t)
	tracing.Use(nil, propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { tracing.Use(nil, nil) })
}

func withAgentTracing(t *testing.T, on bool) {
	t.Helper()
	requireSerial(t)
	agenttrace.SetEnabled(on)
	t.Cleanup(func() { agenttrace.SetEnabled(false) })
}

func traceIDOf(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		return ""
	}
	return parts[1]
}

// assertVendorHygiene checks what a vendor received: the caller's trace,
// allowlisted baggage only, and none of the gateway's own hop headers.
func assertVendorHygiene(t *testing.T, call vendorCall) {
	t.Helper()
	assert.Equal(t, clientTraceID, traceIDOf(call.Headers.Get("Traceparent")), "the vendor call continues the caller's trace")
	assert.Equal(t, "session.id=s1", call.Headers.Get("Baggage"), "only allowlisted baggage reaches the vendor")
	for _, hdr := range []string{hdrInternalHop, hdrInternalSpan, hdrFailoverOrigin, hdrFailoverAttempt, hdrFailoverToken} {
		assert.Empty(t, call.Headers.Get(hdr), "leaked %s to the vendor", hdr)
	}
}

func postTraced(h *failoverHarness, path, body string) (*http.Response, []byte) {
	return h.post(path, body, "Traceparent", clientTraceparent, "Baggage", "session.id=s1,user.email=leak%40example.com")
}

func waitForLogs(t *testing.T, h *failoverHarness, n int) []models.ProxyLog {
	t.Helper()
	var logs []models.ProxyLog
	require.Eventually(t, func() bool { logs = h.proxyLogs(); return len(logs) == n }, 5*time.Second, 20*time.Millisecond)
	return logs
}

// With agent tracing on, an /ai/ call that fails over is one logical call:
// every rung's ProxyLog carries the outer call's gateway span id, and every
// hop stays on the caller's trace.
func TestAgentTraceIdentity_AIFailoverSharesOneSpan(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("from the fallback"), nil)

	resp, body := postTraced(h, "/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	for _, calls := range [][]vendorCall{h.primaryVendor.calls(), h.fallbackVendor.calls()} {
		require.Len(t, calls, 1)
		assertVendorHygiene(t, calls[0])
	}

	logs := waitForLogs(t, h, 2)
	span := logs[0].TraceSpanID
	assert.Len(t, span, 16, "trace_span_id is a W3C span id")
	assert.Equal(t, span, logs[1].TraceSpanID, "every rung of one /ai/ call shares the outer call's span id")
	_, ok := agenttrace.ParseSpanID(span)
	assert.True(t, ok)
}

// The same request with tracing off: no span id is minted, yet the caller's
// trace still crosses the /ai/ hop (it used to stop at the loopback), and the
// hygiene fixes hold in every edition.
func TestAgentTraceIdentity_TracingOffKeepsTraceAndHygiene(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, false)
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("from the fallback"), nil)

	resp, body := postTraced(h, "/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	for _, calls := range [][]vendorCall{h.primaryVendor.calls(), h.fallbackVendor.calls()} {
		require.Len(t, calls, 1)
		assertVendorHygiene(t, calls[0])
	}
	for _, l := range waitForLogs(t, h, 2) {
		assert.Empty(t, l.TraceSpanID, "no trace link while tracing is off")
	}
}

// The streaming path clones the request headers for the vendor: the hop
// token and the outer span id must not ride along.
func TestAgentTraceIdentity_StreamingHygiene(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("streamed by the fallback"), nil)

	resp, body := postTraced(h, "/ai/primary/v1/chat/completions", failoverStreamBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	calls := h.fallbackVendor.calls()
	require.Len(t, calls, 1)
	assertVendorHygiene(t, calls[0])
}

// A client that sends the gateway's hop headers on the native /llm/ route
// gets them stripped before the vendor, like the loopback's own.
func TestAgentTraceIdentity_ClientHopHeadersStrippedOnLLMRoute(t *testing.T) {
	withW3CPropagation(t)
	h := newFailoverHarness(t, serveOpenAIText("direct"), serveOpenAIText("never"), nil)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody,
		"Traceparent", clientTraceparent, "Baggage", "session.id=s1,secret=x",
		hdrInternalHop, "forged", hdrInternalSpan, "00f067aa0ba902b7")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assertVendorHygiene(t, calls[0])
}

// An LLM that opted out of propagation gets no trace context at all; the
// other LLMs keep today's behaviour.
func TestAgentTraceIdentity_PerLLMPropagationOptOut(t *testing.T) {
	withW3CPropagation(t)
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("from the fallback"), func(primary, fallback *models.LLM) {
		fallback.TracingPropagationOptOut = true
	})

	resp, body := postTraced(h, "/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	primary := h.primaryVendor.calls()
	require.Len(t, primary, 1)
	assertVendorHygiene(t, primary[0])

	fallback := h.fallbackVendor.calls()
	require.Len(t, fallback, 1)
	for _, hdr := range []string{"Traceparent", "Tracestate", "Baggage"} {
		assert.Empty(t, fallback[0].Headers.Get(hdr), "opted-out LLM received %s", hdr)
	}
}

// With tracing off the request path allocates nothing for capture.
func TestAgentTraceIdentity_OffPathAllocatesNothing(t *testing.T) {
	withAgentTracing(t, false)
	r, _ := http.NewRequest(http.MethodPost, "http://x/llm/rest/a", nil)
	r.RemoteAddr = "127.0.0.1:1"
	if n := testing.AllocsPerRun(200, func() {
		r2, rec := beginRequestCapture(r)
		_ = traceSpanIDFor(r2.Context())
		rec.Finish()
	}); n != 0 {
		t.Fatalf("beginRequestCapture allocates %.0f with tracing off", n)
	}
}
