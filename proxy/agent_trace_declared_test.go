package proxy

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/declared"
)

const (
	ccSession   = "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"
	codexSessID = "01a158d5-cb58-7a20-9c50-a33ff56877da"
	codexTurnID = "01a158d5-cc44-78c1-84bd-67edbc4e3b88"
)

func withDeclaredExtractor(t *testing.T) {
	t.Helper()
	declared.SetDefault(declared.New(declared.Options{
		CompatFamilies: []string{declared.FamilyHelicone, declared.FamilyMCPSession},
		Harness:        []string{declared.HarnessClaudeCode, declared.HarnessCodexCLI, declared.HarnessOpenCode},
	}))
	t.Cleanup(func() { declared.SetDefault(nil) })
}

// The context a client declares in its headers reaches the capture event
// through the real proxy (jyx.113): X-Tyk-* headers, a verified coding
// harness's session, sub-agent and turn ids, X-Tyk-Trace-Capture, and the
// body's session field when no header named one. The headers never reach
// the vendor.
func TestAgentTraceDeclared_ReachesTheCaptureEvent(t *testing.T) {
	chat := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`

	for _, tc := range []struct {
		name    string
		body    string
		headers []string
		check   func(t *testing.T, d agenttrace.DeclaredContext)
	}{
		{"tyk headers and capture mode", chat, []string{
			"X-Tyk-Session-Id", "tyk-s1", "X-Tyk-Agent-Name", "planner", "X-Tyk-Trace-Capture", "off",
		}, func(t *testing.T, d agenttrace.DeclaredContext) {
			assert.Equal(t, "tyk-s1", d.SessionID)
			assert.Equal(t, "x-tyk-session-id", d.SessionSource)
			assert.Equal(t, "planner", d.AgentName)
			assert.Equal(t, "off", d.CaptureMode)
		}},
		{"claude code", chat, []string{
			"User-Agent", "claude-cli/2.1.304 (external, cli)", "X-Claude-Code-Session-Id", ccSession,
			"X-Claude-Code-Agent-Id", "a1b2c3d4e5f6a7b8c",
		}, func(t *testing.T, d agenttrace.DeclaredContext) {
			assert.Equal(t, declared.HarnessClaudeCode, d.Harness)
			assert.Equal(t, ccSession, d.SessionID)
			assert.Equal(t, declared.HarnessClaudeCode, d.SessionSource)
			assert.Equal(t, "a1b2c3d4e5f6a7b8c", d.SubAgentID)
		}},
		{"codex turn", chat, []string{
			"User-Agent", "codex_exec/0.160.1 (Mac OS)",
			"X-Codex-Turn-Metadata", `{"session_id":"` + codexSessID + `","turn_id":"` + codexTurnID + `"}`,
		}, func(t *testing.T, d agenttrace.DeclaredContext) {
			assert.Equal(t, declared.HarnessCodexCLI, d.Harness)
			assert.Equal(t, codexSessID, d.SessionID)
			assert.Equal(t, codexTurnID, d.TurnID)
		}},
		{"body session", `{"model":"gpt-4o","metadata":{"session_id":"body-s1"},"user":"u-7","messages":[{"role":"user","content":"hi"}]}`, nil,
			func(t *testing.T, d agenttrace.DeclaredContext) {
				assert.Equal(t, "body-s1", d.SessionID)
				assert.Equal(t, "u-7", d.EndUserID)
			}},
		{"compat family", chat, []string{"Helicone-Session-Id", "h-1", "Traceparent", clientTraceparent},
			func(t *testing.T, d agenttrace.DeclaredContext) {
				assert.Equal(t, "h-1", d.SessionID)
				assert.Equal(t, "helicone", d.SessionSource)
				assert.Equal(t, clientTraceID, d.TraceID.String())
				assert.Equal(t, "traceparent", d.TraceSource)
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := withCapture(t)
			withDeclaredExtractor(t)
			var upstream http.Header
			h := newFailoverHarness(t, func(w http.ResponseWriter, r *http.Request) {
				upstream = r.Header.Clone()
				serveToolCall(w, r)
			}, serveOpenAIText("never"), nil)

			resp, body := h.post("/llm/rest/primary/v1/chat/completions", tc.body, tc.headers...)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			ev := waitLLMEvent(t, sink)
			tc.check(t, ev.Declared)
			assert.Empty(t, upstream.Get("X-Tyk-Session-Id"), "tracing headers never reach the vendor")
			assert.Empty(t, upstream.Get("X-Tyk-Trace-Capture"))
		})
	}
}

// With no declared trace context the gateway mints a trace id; the event's
// declared context stays empty.
func TestAgentTraceDeclared_NothingDeclared(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveToolCall, serveOpenAIText("never"), nil)
	resp, body := h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	ev := waitLLMEvent(t, sink)
	assert.False(t, ev.TraceID.IsZero(), "a minted trace id")
	assert.True(t, ev.Declared.TraceID.IsZero())
	assert.Empty(t, ev.Declared.SessionID)
}

// An App with tracing switched off is not captured at all: no event of any
// kind, while the request itself is served (jyx.112).
func TestAgentTraceAppTracingOffCapturesNothing(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveToolCall, serveOpenAIText("never"), nil)
	off := false
	h.app.Tracing = models.AppTracingSettings{Enabled: &off}
	require.NoError(t, h.db.Model(h.app).Select("tracing").Updates(h.app).Error)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest, "X-Tyk-Session-Id", "s1")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	waitForLogs(t, h, 1) // the analytics ran
	time.Sleep(100 * time.Millisecond)
	sink.mu.Lock()
	assert.Empty(t, sink.events, "an App with tracing off leaves no capture")
	sink.mu.Unlock()

	// Control: switched back on, the same App is captured.
	on := true
	h.app.Tracing = models.AppTracingSettings{Enabled: &on}
	require.NoError(t, h.db.Model(h.app).Select("tracing").Updates(h.app).Error)
	resp, body = h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	waitLLMEvent(t, sink)
}

// Tool and MCP calls carry the declared context too.
func TestAgentTraceDeclared_ToolCalls(t *testing.T) {
	sink := withCapture(t)
	withDeclaredExtractor(t)
	f := newToolFilterFixture(t, `{"message":"ok"}`)
	defer f.teardown()

	session := newMCPSession(t, f)
	rr := session.postWith(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getTestData","arguments":{}}}`,
		map[string]string{"X-Tyk-Session-Id": "tool-s1", "X-Tyk-Agent-Name": "researcher"})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	ev := waitKindEvent(t, sink, agenttrace.KindToolCall)
	assert.Equal(t, "tool-s1", ev.Declared.SessionID)
	assert.Equal(t, "researcher", ev.Declared.AgentName)
	assert.Equal(t, session.sessionID, ev.Declared.MCPSessionID)
}
