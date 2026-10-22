package declared

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

var allFamilies = []string{FamilyHelicone, FamilyPortkey, FamilyOpenRouter, FamilyBifrost,
	FamilyAgentRouter, FamilyCloudflare, FamilyAgentCore, FamilyMCPSession}

func defaultExtractor() *Extractor {
	return New(Options{
		CompatFamilies:   allFamilies,
		Harness:          []string{HarnessClaudeCode, HarnessCodexCLI, HarnessOpenCode},
		BaggageAllowlist: []string{"session.id"},
	})
}

func hdr(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i < len(kv); i += 2 {
		h.Add(kv[i], kv[i+1])
	}
	return h
}

const (
	tp       = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	tpTrace  = "4bf92f3577b34da6a3ce929d0e0e4736"
	tpParent = "00f067aa0ba902b7"
)

func TestTraceparent(t *testing.T) {
	cases := []struct {
		name, value        string
		wantTrace, wantPar string
		sampled            bool
		invalid            bool
	}{
		{"valid sampled", tp, tpTrace, tpParent, true, false},
		{"valid unsampled", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00", tpTrace, tpParent, false, false},
		{"future version with extra field", "01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-xyz", tpTrace, tpParent, true, false},
		{"version 00 with extra field", tp + "-xyz", "", "", false, true},
		{"version ff", "ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "", "", false, true},
		{"uppercase", strings.ToUpper(tp), "", "", false, true},
		{"zero trace", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", "", "", false, true},
		{"zero parent", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", "", "", false, true},
		{"short", "00-4bf9-00f0-01", "", "", false, true},
		{"garbage", "hello", "", "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := defaultExtractor().Extract(hdr("Traceparent", tc.value, "Tracestate", "rojo=00f067aa0ba902b7"), "", nil)
			d := r.Declared
			if d.TraceID.String() != tc.wantTrace || d.ParentSpanID.String() != tc.wantPar || d.Sampled != tc.sampled {
				t.Fatalf("got trace %q parent %q sampled %v", d.TraceID, d.ParentSpanID, d.Sampled)
			}
			if (len(r.Invalid) > 0) != tc.invalid {
				t.Fatalf("invalid = %v", r.Invalid)
			}
			if !tc.invalid && (d.TraceSource != "traceparent" || d.TraceState != "rojo=00f067aa0ba902b7") {
				t.Fatalf("source %q state %q", d.TraceSource, d.TraceState)
			}
		})
	}
	r := defaultExtractor().Extract(hdr("Traceparent", tp, "Traceparent", tp), "", nil)
	if !r.Declared.TraceID.IsZero() || len(r.Invalid) != 1 {
		t.Fatalf("repeated traceparent accepted: %+v", r)
	}
}

func TestTykHeadersAndLimits(t *testing.T) {
	h := hdr(
		"X-Tyk-Session-Id", "conv-42",
		"X-Tyk-Session-Name", "support-triage",
		"X-Tyk-Trace-Id", "4BF92F3577B34DA6A3CE929D0E0E4736",
		"X-Tyk-Span-Id", "00f067aa0ba90201",
		"X-Tyk-Parent-Span-Id", "00f067aa0ba90202",
		"X-Tyk-Agent-Name", "Researcher ✨",
		"X-Tyk-Agent-Id", "agent-researcher-v2",
		"X-Tyk-Agent-Path", "/root/researcher/",
		"X-Tyk-User-Id", "user@example.com",
		"X-Tyk-Trace-Capture", "REDACTED",
	)
	r := defaultExtractor().Extract(h, "", nil)
	d := r.Declared
	want := agenttrace.DeclaredContext{
		TraceID: d.TraceID, ParentSpanID: d.ParentSpanID, TraceSource: "x-tyk",
		SessionID: "conv-42", SessionSource: "x-tyk-session-id", SessionName: "support-triage",
		AgentName: "Researcher ✨", AgentID: "agent-researcher-v2", AgentPath: "/root/researcher",
		ClientSpanID: "00f067aa0ba90201", EndUserID: "user@example.com", CaptureMode: "redacted",
		ConversationID: "conv-42",
	}
	if !reflect.DeepEqual(d, want) || len(r.Invalid) != 0 {
		t.Fatalf("got %+v\ninvalid %v", d, r.Invalid)
	}
	if d.TraceID.String() != tpTrace || d.ParentSpanID.String() != "00f067aa0ba90202" {
		t.Fatalf("ids %s %s", d.TraceID, d.ParentSpanID)
	}

	long := strings.Repeat("a", 201)
	bad := []struct{ header, value string }{
		{"X-Tyk-Session-Id", long},
		{"X-Tyk-Session-Id", "tab\tinside"},
		{"X-Tyk-Session-Id", "ünïcode"},
		{"X-Tyk-Session-Name", strings.Repeat("n", 121)},
		{"X-Tyk-Trace-Id", "xyz"},
		{"X-Tyk-Trace-Id", strings.Repeat("0", 32)},
		{"X-Tyk-Span-Id", "123"},
		{"X-Tyk-Parent-Span-Id", "zzzzzzzzzzzzzzzz"},
		{"X-Tyk-Agent-Name", "line\nbreak"},
		{"X-Tyk-Agent-Id", long},
		{"X-Tyk-Agent-Path", "root/researcher"},
		{"X-Tyk-Agent-Path", "/a//b"},
		{"X-Tyk-Agent-Path", "/" + strings.Repeat("s/", 17)},
		{"X-Tyk-User-Id", strings.Repeat("u", 201)},
		{"X-Tyk-Trace-Capture", "everything"},
	}
	for _, b := range bad {
		r := defaultExtractor().Extract(hdr(b.header, b.value), "", nil)
		if len(r.Invalid) != 1 || !strings.HasPrefix(r.Invalid[0], strings.ToLower(b.header)+":") {
			t.Errorf("%s=%q: invalid %v", b.header, b.value, r.Invalid)
		}
		if !reflect.DeepEqual(r.Declared, agenttrace.DeclaredContext{}) {
			t.Errorf("%s=%q leaked into %+v", b.header, b.value, r.Declared)
		}
	}
	// Exactly at the limits is accepted.
	r = defaultExtractor().Extract(hdr("X-Tyk-Session-Id", strings.Repeat("a", 200), "X-Tyk-Agent-Name", strings.Repeat("ä", 120)), "", nil)
	if len(r.Declared.SessionID) != 200 || r.Declared.AgentName == "" || len(r.Invalid) != 0 {
		t.Fatalf("limits: %v", r.Invalid)
	}
	// A repeated single-value header is ambiguous.
	r = defaultExtractor().Extract(hdr("X-Tyk-Session-Id", "a", "X-Tyk-Session-Id", "b"), "", nil)
	if r.Declared.SessionID != "" || len(r.Invalid) != 1 {
		t.Fatalf("repeated header: %+v", r)
	}
}

func TestPrecedence(t *testing.T) {
	// traceparent wins trace and parent over every other trace source; Tyk
	// headers win the session over compat, baggage, body and harness.
	h := hdr(
		"Traceparent", tp,
		"X-Tyk-Trace-Id", "11111111111111111111111111111111",
		"X-Tyk-Parent-Span-Id", "2222222222222222",
		"X-Portkey-Trace-Id", "pk-trace",
		"Cf-Aig-Otel-Trace-Id", "33333333333333333333333333333333",
		"X-Tyk-Session-Id", "tyk-session",
		"Helicone-Session-Id", "helicone-session",
		"X-Session-Id", "openrouter-session",
		"Baggage", "session.id=bag-session,user.id=bag-user,gen_ai.agent.name=bag-agent",
		"X-Tyk-Agent-Name", "tyk-agent",
		"Helicone-Session-Path", "/helicone/path",
		"X-Tyk-Agent-Path", "/tyk/path",
	)
	body := []byte(`{"user":"body-user","metadata":{"session_id":"body-session"}}`)
	d := defaultExtractor().Extract(h, BodyLLM, body).Declared
	if d.TraceID.String() != tpTrace || d.ParentSpanID.String() != tpParent || d.TraceSource != "traceparent" {
		t.Fatalf("trace precedence: %s %s %s", d.TraceID, d.ParentSpanID, d.TraceSource)
	}
	if d.SessionID != "tyk-session" || d.SessionSource != "x-tyk-session-id" || d.AgentName != "tyk-agent" || d.AgentPath != "/tyk/path" {
		t.Fatalf("session/agent precedence: %+v", d)
	}
	if d.EndUserID != "bag-user" {
		t.Fatalf("end user: baggage beats body, got %q", d.EndUserID)
	}
	if d.ExternalTraceID != "" {
		t.Fatalf("portkey consulted although traceparent won: %q", d.ExternalTraceID)
	}

	// Peel layers off one at a time.
	steps := []struct {
		drop          []string
		session, from string
	}{
		{[]string{"X-Tyk-Session-Id"}, "helicone-session", "helicone"},
		{[]string{"Helicone-Session-Id"}, "openrouter-session", "openrouter"},
		{[]string{"X-Session-Id"}, "bag-session", "baggage"},
		{[]string{"Baggage"}, "body-session", "body.metadata.session_id"},
	}
	for _, s := range steps {
		for _, k := range s.drop {
			h.Del(k)
		}
		d := defaultExtractor().Extract(h, BodyLLM, body).Declared
		if d.SessionID != s.session || d.SessionSource != s.from {
			t.Fatalf("after dropping %v: session %q from %q", s.drop, d.SessionID, d.SessionSource)
		}
	}
	// Without traceparent, X-Tyk-Trace-Id is next, then Portkey, then Cloudflare.
	h.Del("Traceparent")
	if d := defaultExtractor().Extract(h, "", nil).Declared; d.TraceSource != "x-tyk" || d.ParentSpanID.String() != "2222222222222222" {
		t.Fatalf("x-tyk trace: %+v", d)
	}
	h.Del("X-Tyk-Trace-Id")
	if d := defaultExtractor().Extract(h, "", nil).Declared; d.TraceSource != "portkey" || d.ExternalTraceID != "pk-trace" {
		t.Fatalf("portkey trace: %+v", d)
	}
	h.Del("X-Portkey-Trace-Id")
	if d := defaultExtractor().Extract(h, "", nil).Declared; d.TraceSource != "cloudflare" || d.TraceID.String() != "33333333333333333333333333333333" {
		t.Fatalf("cloudflare trace: %+v", d)
	}
}

func TestCompatFamilies(t *testing.T) {
	cases := []struct {
		family  string
		h       http.Header
		check   func(agenttrace.DeclaredContext) bool
		summary string
	}{
		{FamilyHelicone, hdr("Helicone-Session-Id", "h1", "Helicone-Session-Path", "/parent/child", "Helicone-Session-Name", "Nightly"),
			func(d agenttrace.DeclaredContext) bool {
				return d.SessionID == "h1" && d.SessionSource == "helicone" && d.AgentPath == "/parent/child" && d.SessionName == "Nightly"
			}, "session, path, name"},
		{FamilyPortkey, hdr("X-Portkey-Trace-Id", "4BF92F3577B34DA6A3CE929D0E0E4736", "X-Portkey-Span-Id", "step-1", "X-Portkey-Parent-Span-Id", "00f067aa0ba902b7"),
			func(d agenttrace.DeclaredContext) bool {
				return d.TraceID.String() == tpTrace && d.ExternalTraceID == "" && d.ParentSpanID.String() == tpParent && len(d.ClientSpanID) == 16
			}, "W3C id used as is, non-W3C span hashed"},
		{FamilyOpenRouter, hdr("X-Session-Id", "or-1"),
			func(d agenttrace.DeclaredContext) bool {
				return d.SessionID == "or-1" && d.SessionSource == "openrouter"
			}, "x-session-id"},
		{FamilyBifrost, hdr("X-Bf-Session-Id", "bf-1"),
			func(d agenttrace.DeclaredContext) bool { return d.SessionID == "bf-1" && d.SessionSource == "bifrost" }, "x-bf-session-id"},
		{FamilyAgentRouter, hdr("Agent-Session-Id", "ar-1"),
			func(d agenttrace.DeclaredContext) bool {
				return d.SessionID == "ar-1" && d.SessionSource == "agent_router"
			}, "agent-session-id"},
		{FamilyCloudflare, hdr("Cf-Aig-Otel-Trace-Id", tpTrace, "Cf-Aig-Otel-Parent-Span-Id", tpParent),
			func(d agenttrace.DeclaredContext) bool {
				return d.TraceID.String() == tpTrace && d.ParentSpanID.String() == tpParent && d.TraceSource == "cloudflare"
			}, "trace and parent"},
		{FamilyAgentCore, hdr("X-Amzn-Bedrock-AgentCore-Runtime-Session-Id", "ac-1"),
			func(d agenttrace.DeclaredContext) bool {
				return d.SessionID == "ac-1" && d.SessionSource == "agentcore"
			}, "runtime session"},
		{FamilyMCPSession, hdr("Mcp-Session-Id", "mcp-1"),
			func(d agenttrace.DeclaredContext) bool { return d.MCPSessionID == "mcp-1" && d.SessionID == "" }, "weak hint only, never the session"},
	}
	for _, tc := range cases {
		t.Run(tc.family, func(t *testing.T) {
			on := New(Options{CompatFamilies: []string{tc.family}}).Extract(tc.h, "", nil)
			if !tc.check(on.Declared) || len(on.Invalid) != 0 {
				t.Fatalf("%s: %+v invalid %v", tc.summary, on.Declared, on.Invalid)
			}
			off := New(Options{}).Extract(tc.h, "", nil)
			if !reflect.DeepEqual(off.Declared, agenttrace.DeclaredContext{}) {
				t.Fatalf("family off still extracted: %+v", off.Declared)
			}
		})
	}
}

func TestPortkeyHashingIsDeterministic(t *testing.T) {
	x := New(Options{CompatFamilies: []string{FamilyPortkey}})
	a := x.Extract(hdr("X-Portkey-Trace-Id", "trace-abc", "X-Portkey-Span-Id", "span-abc"), "", nil).Declared
	b := x.Extract(hdr("X-Portkey-Trace-Id", "trace-abc", "X-Portkey-Span-Id", "span-abc"), "", nil).Declared
	c := x.Extract(hdr("X-Portkey-Trace-Id", "trace-abd", "X-Portkey-Span-Id", "span-abd"), "", nil).Declared
	if a.TraceID != b.TraceID || a.ClientSpanID != b.ClientSpanID || a.ExternalTraceID != "trace-abc" {
		t.Fatalf("not deterministic: %+v %+v", a, b)
	}
	if a.TraceID == c.TraceID || a.ClientSpanID == c.ClientSpanID {
		t.Fatal("different ids hash equal")
	}
	// Pinned value, so a change to the hash is a deliberate one.
	if a.TraceID.String() != pinnedPortkeyTrace {
		t.Fatalf("hash changed: %s", a.TraceID)
	}
}

func TestBaggage(t *testing.T) {
	x := New(Options{BaggageAllowlist: []string{"session.id", "team"}})
	r := x.Extract(hdr("Baggage", "session.id=s%201;prop=1, team=blue ,user.id=u1,gen_ai.agent.name=writer,tyk.agent.parent=planner,other=x"), "", nil)
	d := r.Declared
	if d.SessionID != "s 1" || d.SessionSource != "baggage" || d.EndUserID != "u1" || d.AgentName != "writer" || d.AgentPath != "/planner/writer" {
		t.Fatalf("correlation keys: %+v", d)
	}
	if !reflect.DeepEqual(d.Baggage, map[string]string{"session.id": "s 1", "team": "blue"}) {
		t.Fatalf("allowlist: %v", d.Baggage)
	}
	if r := x.Extract(hdr("Baggage", "gen_ai.conversation.id=c-9"), "", nil); r.Declared.ConversationID != "c-9" || r.Declared.SessionID != "c-9" {
		t.Fatalf("conversation id: %+v", r.Declared)
	}
	if r := x.Extract(hdr("Baggage", "session.id="+strings.Repeat("a", 8200)), "", nil); r.Declared.SessionID != "" || len(r.Invalid) != 1 {
		t.Fatalf("oversized baggage: %+v", r)
	}
	if r := x.Extract(hdr("Baggage", "bad key=1,session.id=%zz,ok=1"), "", nil); len(r.Invalid) != 2 || r.Declared.SessionID != "" {
		t.Fatalf("malformed members: %+v", r)
	}
	var many []string
	for i := 0; i < 70; i++ {
		many = append(many, fmt.Sprintf("k%d=v", i))
	}
	if r := x.Extract(hdr("Baggage", strings.Join(many, ",")), "", nil); len(r.Invalid) != 1 {
		t.Fatalf("too many members: %v", r.Invalid)
	}
}

func TestLLMBodyFields(t *testing.T) {
	x := defaultExtractor()
	cases := []struct {
		body, session, source, conv, user string
	}{
		{`{"conversation":"conv_123","input":"hi"}`, "conv_123", "body.conversation", "conv_123", ""},
		{`{"conversation":{"id":"conv_456"}}`, "conv_456", "body.conversation", "conv_456", ""},
		{`{"metadata":{"session_id":"m-1"},"user":"end-user-7"}`, "m-1", "body.metadata.session_id", "m-1", "end-user-7"},
		{`{"messages":[{"role":"user","content":"x"}]}`, "", "", "", ""},
		{`{"metadata":{"session_id":42}}`, "", "", "", ""},
		{`not json`, "", "", "", ""},
		{`[1,2]`, "", "", "", ""},
	}
	for _, tc := range cases {
		d := x.Extract(http.Header{}, BodyLLM, []byte(tc.body)).Declared
		if d.SessionID != tc.session || d.SessionSource != tc.source || d.ConversationID != tc.conv || d.EndUserID != tc.user {
			t.Errorf("%s: %+v", tc.body, d)
		}
	}
	// Body fields are read only for LLM bodies.
	if d := x.Extract(http.Header{}, "", []byte(`{"conversation":"c"}`)).Declared; d.SessionID != "" {
		t.Fatal("body read without a body kind")
	}
}

func TestMCP(t *testing.T) {
	x := defaultExtractor()
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"q":"x"},
	  "_meta":{"traceparent":"00-11111111111111111111111111111111-2222222222222222-01","tracestate":"a=b",
	  "io.modelcontextprotocol/clientInfo":{"name":"claude-code","version":"2.1.304"}}}}`)
	// _meta.traceparent is preferred over the HTTP header for MCP.
	r := x.Extract(hdr("Traceparent", tp), BodyMCP, body)
	d := r.Declared
	if d.TraceID.String() != "11111111111111111111111111111111" || d.TraceSource != "mcp_meta" || d.TraceState != "a=b" {
		t.Fatalf("mcp _meta: %+v", d)
	}
	if r.MCPMethod != "tools/call" || r.MCPName != "search" || d.ClientInfo != "claude-code/2.1.304" {
		t.Fatalf("classification: %+v", r)
	}
	// Mcp-Method / Mcp-Name headers (2026-07-28) win over the body.
	r = x.Extract(hdr("Mcp-Method", "resources/read", "Mcp-Name", "file:///a"), BodyMCP, []byte(`{"method":"tools/call","params":{"name":"x"}}`))
	if r.MCPMethod != "resources/read" || r.MCPName != "file:///a" {
		t.Fatalf("headers: %+v", r)
	}
	// Without _meta the HTTP traceparent applies.
	r = x.Extract(hdr("Traceparent", tp), BodyMCP, []byte(`{"method":"tools/list"}`))
	if r.Declared.TraceSource != "traceparent" || r.MCPMethod != "tools/list" {
		t.Fatalf("fallback: %+v", r)
	}
	// Batches and garbage are not classified and do not fail.
	for _, b := range []string{`[{"method":"tools/call"}]`, `{`, ``} {
		if r := x.Extract(http.Header{}, BodyMCP, []byte(b)); r.MCPMethod != "" {
			t.Fatalf("%q classified: %+v", b, r)
		}
	}
}

func TestHeadersThenApplyBodyEqualsExtract(t *testing.T) {
	x := defaultExtractor()
	cc := "claude-cli/2.1.304"
	meta := `{"metadata":{"user_id":"{\"session_id\":\"c8f8137c-8377-4e3d-81ad-c20c1b346bb2\"}"}}`
	cases := []struct {
		name          string
		h             http.Header
		body          string
		session, from string
	}{
		{"header beats body", hdr("X-Tyk-Session-Id", "tyk"), `{"metadata":{"session_id":"body"}}`, "tyk", "x-tyk-session-id"},
		{"body beats harness", hdr("User-Agent", cc, "X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"),
			`{"metadata":{"session_id":"body"}}`, "body", "body.metadata.session_id"},
		{"harness header, no body field", hdr("User-Agent", cc, "X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"),
			`{"messages":[]}`, "c8f8137c-8377-4e3d-81ad-c20c1b346bb2", HarnessClaudeCode},
		{"harness metadata fallback", hdr("User-Agent", cc), meta, "c8f8137c-8377-4e3d-81ad-c20c1b346bb2", HarnessClaudeCode},
		{"explicit conversation survives", hdr("X-Tyk-Session-Id", "s"), `{"conversation":"conv_1"}`, "s", "x-tyk-session-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			whole := x.Extract(tc.h, BodyLLM, []byte(tc.body))
			r := x.Headers(tc.h, BodyLLM, []byte(tc.body))
			x.ApplyBody(&r, []byte(tc.body))
			if !reflect.DeepEqual(whole, r) {
				t.Fatalf("split differs:\n%+v\n%+v", whole, r)
			}
			if r.Declared.SessionID != tc.session || r.Declared.SessionSource != tc.from {
				t.Fatalf("session %q from %q", r.Declared.SessionID, r.Declared.SessionSource)
			}
		})
	}
	// Headers alone never reads an LLM body.
	r := x.Headers(http.Header{}, BodyLLM, []byte(`{"metadata":{"session_id":"body"},"user":"u"}`))
	if r.Declared.SessionID != "" || r.Declared.EndUserID != "" {
		t.Fatalf("Headers read the body: %+v", r.Declared)
	}
	// The derived conversation id follows a session that ApplyBody replaced;
	// an explicit one stays.
	r = x.Extract(hdr("User-Agent", cc, "X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"), BodyLLM, []byte(`{"metadata":{"session_id":"body"}}`))
	if r.Declared.ConversationID != "body" {
		t.Fatalf("conversation id %q", r.Declared.ConversationID)
	}
	r = x.Extract(hdr("X-Tyk-Session-Id", "s"), BodyLLM, []byte(`{"conversation":"conv_1"}`))
	if r.Declared.ConversationID != "conv_1" {
		t.Fatalf("explicit conversation id %q", r.Declared.ConversationID)
	}
}

func TestHarnessExtraHeaders(t *testing.T) {
	x := New(Options{Harness: []string{"x-my-agent-session"}})
	d := x.Extract(hdr("X-My-Agent-Session", "s-1"), "", nil).Declared
	if d.SessionID != "s-1" || d.SessionSource != "header:x-my-agent-session" {
		t.Fatalf("%+v", d)
	}
}

func TestHarnessVersionGate(t *testing.T) {
	x := defaultExtractor()
	cases := []struct {
		ua      string
		h       http.Header
		harness string
		ok      bool
	}{
		{"claude-cli/2.1.304 (external, sdk-cli)", hdr("X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"), HarnessClaudeCode, true},
		{"claude-cli/3.0.0", hdr("X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"), HarnessClaudeCode, true},
		{"claude-cli/2.0.99", hdr("X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"), HarnessClaudeCode, false},
		{"claude-cli/garbage", hdr("X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"), HarnessClaudeCode, false},
		{"codex_exec/0.160.1 (Mac OS)", hdr("Session-Id", "01a158d5-cb58-7a20-9c50-a33ff56877da"), HarnessCodexCLI, true},
		{"codex_exec/0.159.3", hdr("Session-Id", "01a158d5-cb58-7a20-9c50-a33ff56877da"), HarnessCodexCLI, false},
		{"opencode/1.18.23 ai-sdk/x", hdr("X-Session-Affinity", "ses_ea7349613ffei4HdWLfrnwQOVH"), HarnessOpenCode, true},
		{"opencode/1.17.0", hdr("X-Session-Affinity", "ses_ea7349613ffei4HdWLfrnwQOVH"), HarnessOpenCode, false},
	}
	for _, tc := range cases {
		tc.h.Set("User-Agent", tc.ua)
		r := x.Extract(tc.h, "", nil)
		if got := r.Declared.SessionID != ""; got != tc.ok {
			t.Errorf("%s: session %q invalid %v", tc.ua, r.Declared.SessionID, r.Invalid)
		}
		if tc.ok && (r.Harness.Name != tc.harness || r.Declared.SessionSource != tc.harness) {
			t.Errorf("%s: harness %+v source %q", tc.ua, r.Harness, r.Declared.SessionSource)
		}
		if !tc.ok && (len(r.Invalid) != 1 || !strings.HasSuffix(r.Invalid[0], "unverified_version")) {
			t.Errorf("%s: invalid %v", tc.ua, r.Invalid)
		}
	}
	// Disabled extractors do nothing.
	off := New(Options{}).Extract(hdr("User-Agent", "claude-cli/2.1.304", "X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2"), "", nil)
	if off.Declared.SessionID != "" || off.Harness.Name != "" {
		t.Fatalf("disabled extractor ran: %+v", off)
	}
}

func TestHarnessShapes(t *testing.T) {
	x := defaultExtractor()
	cc := "claude-cli/2.1.304"
	// Claude Code without the header: the session_id inside metadata.user_id.
	body := []byte(`{"metadata":{"user_id":"{\"device_id\":\"` + strings.Repeat("ab", 32) + `\",\"account_uuid\":\"acc-1\",\"session_id\":\"C8F8137C-8377-4E3D-81AD-C20C1B346BB2\"}"}}`)
	r := x.Extract(hdr("User-Agent", cc), BodyLLM, body)
	if r.Declared.SessionID != "c8f8137c-8377-4e3d-81ad-c20c1b346bb2" || r.Declared.SessionSource != HarnessClaudeCode {
		t.Fatalf("metadata session: %+v", r)
	}
	assertNoIdentifiers(t, r, strings.Repeat("ab", 32), "acc-1")
	// Other metadata shapes are ignored without error (legacy, plain string, number).
	for _, b := range []string{
		`{"metadata":{"user_id":"user_abc_account_def_session_c8f8137c-8377-4e3d-81ad-c20c1b346bb2"}}`,
		`{"metadata":{"user_id":42}}`,
		`{"metadata":"x"}`,
	} {
		if r := x.Extract(hdr("User-Agent", cc), BodyLLM, []byte(b)); r.Declared.SessionID != "" || len(r.Invalid) != 0 {
			t.Errorf("%s: %+v", b, r)
		}
	}
	// A JSON user_id whose session_id is not a UUID is counted.
	if r := x.Extract(hdr("User-Agent", cc), BodyLLM, []byte(`{"metadata":{"user_id":"{\"session_id\":\"nope\"}"}}`)); len(r.Invalid) != 1 {
		t.Fatalf("bad inner session: %+v", r)
	}
	// Malformed header values are counted, not used.
	for _, h := range []http.Header{
		hdr("User-Agent", cc, "X-Claude-Code-Session-Id", "not-a-uuid"),
		hdr("User-Agent", cc, "X-Claude-Code-Agent-Id", "has space"),
		hdr("User-Agent", "codex_exec/0.160.1", "Session-Id", "nope"),
		hdr("User-Agent", "codex_exec/0.160.1", "X-Codex-Turn-Metadata", `{"turn_id":"nope"}`),
		hdr("User-Agent", "codex_exec/0.160.1", "X-Codex-Turn-Metadata", `{`+strings.Repeat(" ", 5000)+`}`),
		hdr("User-Agent", "opencode/1.18.23", "X-Session-Affinity", "not-opencode"),
	} {
		r := x.Extract(h, "", nil)
		if len(r.Invalid) != 1 || r.Declared.SessionID != "" || r.Harness.SubAgentID != "" || r.Harness.TurnID != "" {
			t.Errorf("%v: %+v", h, r)
		}
	}
}

// recorded is one line of testdata/agent_tracing/harness/*/*.jsonl.
type recorded struct {
	Run     string            `json:"run"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func loadRecorded(t *testing.T, path string) []recorded {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "testdata", "agent_tracing", "harness", path))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []recorded
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r recorded
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		t.Fatalf("%s: no records", path)
	}
	return out
}

func (r recorded) header() http.Header {
	h := http.Header{}
	for k, v := range r.Headers {
		h.Set(k, v)
	}
	return h
}

// TestRecordedHarnessTraffic replays the jyx.52 recordings: every request
// yields the harness session id, the ids are stable across a resumed
// session and distinct across parallel ones, and no per-machine id leaks.
func TestRecordedHarnessTraffic(t *testing.T) {
	x := defaultExtractor()
	type expect struct {
		file, harness, source, header string
	}
	for _, e := range []expect{
		{"claude-code-2.1.304/anthropic-messages.jsonl", HarnessClaudeCode, HarnessClaudeCode, "X-Claude-Code-Session-Id"},
		{"opencode-1.18.23/anthropic-messages.jsonl", HarnessOpenCode, HarnessOpenCode, "X-Session-Id"},
		{"opencode-1.18.23/openai-responses.jsonl", HarnessOpenCode, HarnessOpenCode, "X-Session-Id"},
		{"codex-cli-0.160.1/openai-responses.jsonl", HarnessCodexCLI, HarnessCodexCLI, "Session-Id"},
	} {
		t.Run(e.file, func(t *testing.T) {
			sessionsByRun := map[string]map[string]bool{}
			turnsByRun := map[string]map[string]bool{}
			subAgents := 0
			for _, rec := range loadRecorded(t, e.file) {
				r := x.Extract(rec.header(), BodyLLM, rec.Body)
				if len(r.Invalid) != 0 {
					t.Fatalf("%s: invalid %v", rec.Run, r.Invalid)
				}
				if r.Harness.Name != e.harness || r.Declared.SessionSource != e.source {
					t.Fatalf("%s: harness %+v source %q", rec.Run, r.Harness, r.Declared.SessionSource)
				}
				if want := rec.Headers[e.header]; r.Declared.SessionID != want || want == "" {
					t.Fatalf("%s: session %q, header %q", rec.Run, r.Declared.SessionID, want)
				}
				if r.Declared.ConversationID != r.Declared.SessionID {
					t.Fatalf("%s: conversation id %q", rec.Run, r.Declared.ConversationID)
				}
				assertNoIdentifiers(t, r, "device_id", "account_uuid", "installation", "<device_id", "<installation_id>")
				if r.Harness.SubAgentID != "" {
					if r.Harness.SubAgentID != rec.Headers["X-Claude-Code-Agent-Id"] {
						t.Fatalf("sub-agent id %q", r.Harness.SubAgentID)
					}
					subAgents++
				}
				if sessionsByRun[rec.Run] == nil {
					sessionsByRun[rec.Run], turnsByRun[rec.Run] = map[string]bool{}, map[string]bool{}
				}
				sessionsByRun[rec.Run][r.Declared.SessionID] = true
				if r.Harness.TurnID != "" {
					turnsByRun[rec.Run][r.Harness.TurnID] = true
				}
			}
			// A2 (resume) and A3 (continue) carry on A1's session. (OpenCode's
			// Anthropic A1 also holds an earlier aborted attempt, so A1 may
			// hold more than one session; see the README.)
			var a2, a3 []string
			for run, ids := range sessionsByRun {
				for id := range ids {
					switch {
					case strings.Contains(run, "-A2-"):
						a2 = append(a2, id)
					case strings.Contains(run, "-A3-"):
						a3 = append(a3, id)
					}
				}
			}
			var a1 map[string]bool
			for run, ids := range sessionsByRun {
				if strings.Contains(run, "-A1-") {
					a1 = ids
				}
			}
			if len(a2) != 1 || len(a3) != 1 || a2[0] != a3[0] || !a1[a2[0]] {
				t.Fatalf("resumed session split: %v", sessionsByRun)
			}
			a := a2
			// The parallel runs hold two distinct sessions, neither of them A's.
			par := map[string]bool{}
			for run, ids := range sessionsByRun {
				if strings.Contains(run, "parallel") {
					for id := range ids {
						par[id] = true
					}
				}
			}
			if len(par) != 2 || par[a[0]] {
				t.Fatalf("parallel sessions: %v (A=%s)", par, a[0])
			}
			if e.harness == HarnessClaudeCode && subAgents == 0 {
				t.Fatal("the recorded sub-agent requests produced no sub-agent id")
			}
			if e.harness == HarnessCodexCLI {
				// One turn id per user turn: A1's two requests share one;
				// A1, A2 and A3 differ.
				var turns []string
				for _, run := range []string{"cx-A1-fresh", "cx-A2-resume", "cx-A3-last"} {
					if len(turnsByRun[run]) != 1 {
						t.Fatalf("%s: turn ids %v", run, turnsByRun[run])
					}
					for id := range turnsByRun[run] {
						turns = append(turns, id)
					}
				}
				if len(dedupe(turns)) != 3 {
					t.Fatalf("turn ids not per turn: %v", turns)
				}
			}
		})
	}
}

// TestRecordedOpenCodeWithoutExtractor: with only the OpenRouter compat
// family on, OpenCode's X-Session-Id still groups, labelled as OpenCode
// (bead jyx.59).
func TestRecordedOpenCodeWithoutExtractor(t *testing.T) {
	x := New(Options{CompatFamilies: []string{FamilyOpenRouter}})
	for _, rec := range loadRecorded(t, "opencode-1.18.23/anthropic-messages.jsonl") {
		d := x.Extract(rec.header(), BodyLLM, rec.Body).Declared
		if d.SessionID == "" || d.SessionSource != HarnessOpenCode {
			t.Fatalf("%s: %+v", rec.Run, d)
		}
	}
	// A real OpenRouter client keeps the openrouter label.
	d := x.Extract(hdr("User-Agent", "openai-python/1.0", "X-Session-Id", "s"), "", nil).Declared
	if d.SessionSource != "openrouter" {
		t.Fatalf("openrouter label: %q", d.SessionSource)
	}
}

// assertNoIdentifiers fails when any string in r contains one of needles.
func assertNoIdentifiers(t *testing.T, r Result, needles ...string) {
	t.Helper()
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.String:
			for _, n := range needles {
				if strings.Contains(v.String(), n) {
					t.Fatalf("result holds %q: %+v", n, r)
				}
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				walk(k)
				walk(v.MapIndex(k))
			}
		}
	}
	walk(reflect.ValueOf(r))
}

func dedupe(s []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return m
}

func TestStripTracingHeaders(t *testing.T) {
	h := hdr("X-Tyk-Session-Id", "s", "X-Tyk-Agent-Path", "/a", "X-Tyk-Trace-Capture", "full",
		"X-Tyk-AI-Profile-ID", "keep", "Helicone-Session-Id", "keep", "Traceparent", tp, "Authorization", "keep")
	h["x-tyk-user-id"] = []string{"non-canonical"}
	StripTracingHeaders(h)
	for _, k := range TracingHeaders {
		if h.Get(k) != "" {
			t.Errorf("%s survived", k)
		}
	}
	if _, ok := h["x-tyk-user-id"]; ok {
		t.Error("non-canonical tracing header survived")
	}
	for _, k := range []string{"X-Tyk-AI-Profile-ID", "Helicone-Session-Id", "Traceparent", "Authorization"} {
		if h.Get(k) == "" {
			t.Errorf("%s was stripped", k)
		}
	}
	SetForwardTykHeaders(true)
	defer SetForwardTykHeaders(false)
	h = hdr("X-Tyk-Session-Id", "s")
	StripTracingHeaders(h)
	if h.Get("X-Tyk-Session-Id") == "" {
		t.Error("forwarding on, header stripped")
	}
}

func TestStripAllocatesNothing(t *testing.T) {
	h := hdr("Content-Type", "application/json", "Authorization", "Bearer x", "X-Tyk-AI-Profile-ID", "p", "User-Agent", "u")
	if n := testing.AllocsPerRun(1000, func() { StripTracingHeaders(h) }); n != 0 {
		t.Fatalf("%v allocs", n)
	}
}

func BenchmarkStripTracingHeaders(b *testing.B) {
	h := hdr("Content-Type", "application/json", "Authorization", "Bearer x", "X-Tyk-AI-Profile-ID", "p",
		"User-Agent", "u", "Accept", "*/*", "Traceparent", tp)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		StripTracingHeaders(h)
	}
}

// benchRequest is a Claude Code request with a realistic 32 KiB chat body:
// many short messages, so many quotes.
func benchRequest() (http.Header, []byte) {
	h := hdr("User-Agent", "claude-cli/2.1.304", "X-Claude-Code-Session-Id", "c8f8137c-8377-4e3d-81ad-c20c1b346bb2",
		"Traceparent", tp, "Content-Type", "application/json", "Authorization", "Bearer x")
	var msgs []string
	for len(strings.Join(msgs, ",")) < 32<<10 {
		msgs = append(msgs, `{"role":"user","content":[{"type":"text","text":"Please read the file and summarise the second section in two lines."}]}`,
			`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/src/main.go"}}]}`)
	}
	body := []byte(`{"model":"m","messages":[` + strings.Join(msgs, ",") + `],"metadata":{"user_id":"{\"session_id\":\"c8f8137c-8377-4e3d-81ad-c20c1b346bb2\"}"}}`)
	return h, body
}

// BenchmarkHeaders is the request-path cost.
func BenchmarkHeaders(b *testing.B) {
	x := defaultExtractor()
	h, body := benchRequest()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		x.Headers(h, BodyLLM, body)
	}
}

// BenchmarkApplyBody is the deferred body pass, off the latency path.
func BenchmarkApplyBody(b *testing.B) {
	x := defaultExtractor()
	h, body := benchRequest()
	r := x.Headers(h, BodyLLM, body)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for i := 0; i < b.N; i++ {
		rr := r
		x.ApplyBody(&rr, body)
	}
}

// pinnedPortkeyTrace is portkeyTraceID("trace-abc").
const pinnedPortkeyTrace = "e6db4e53879095654ca2f90e8b5d29a4"
