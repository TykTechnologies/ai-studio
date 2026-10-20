package declared

import (
	"net/http"
	"testing"
)

// FuzzExtract feeds hostile header values and bodies through every source:
// Extract must not panic, and every accepted id must respect its limit.
func FuzzExtract(f *testing.F) {
	f.Add(tp, "session.id=a,user.id=b", "conv", "claude-cli/2.1.304", `{"metadata":{"user_id":"{\"session_id\":\"c8f8137c-8377-4e3d-81ad-c20c1b346bb2\"}"}}`)
	f.Add("00-x-y-01", "%zz", "/a/b", "codex_exec/0.160.1", `{"method":"tools/call","params":{"_meta":{"traceparent":"bad"}}}`)
	f.Add("", "k=v;p", "", "opencode/1.18.23", `{"conversation":{"id":7}}`)
	x := defaultExtractor()
	f.Fuzz(func(t *testing.T, traceparent, baggage, value, ua, body string) {
		h := http.Header{}
		h.Set("Traceparent", traceparent)
		h.Set("Baggage", baggage)
		h.Set("User-Agent", ua)
		for _, k := range []string{"X-Tyk-Session-Id", "X-Tyk-Agent-Path", "X-Tyk-Trace-Id", "X-Portkey-Trace-Id",
			"Helicone-Session-Path", "X-Session-Id", "Session-Id", "X-Codex-Turn-Metadata", "X-Claude-Code-Session-Id",
			"X-Claude-Code-Agent-Id", "Mcp-Method", "Mcp-Name"} {
			h.Set(k, value)
		}
		for _, kind := range []string{BodyLLM, BodyMCP, ""} {
			r := x.Extract(h, kind, []byte(body))
			d := r.Declared
			if len(d.SessionID) > maxSessionID || len(d.AgentPath) > maxAgentPath || len(d.ExternalTraceID) > maxExternalID ||
				len(d.TraceState) > maxTraceState || len(r.Harness.TurnID) > 36 || len(r.Harness.SubAgentID) > 64 {
				t.Fatalf("limit exceeded: %+v", r)
			}
		}
	})
}
