package scripting

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// Tool results are scanned by default: in every format, under every scope,
// across the whole history. A tool result the filter would block is
// quarantined instead, so the agent loop survives.

// toolOnlyBodies hold "SECRET" only in a tool result.
var toolOnlyBodies = map[string]string{
	"openai": `{"messages":[{"role":"user","content":"read it"},` +
		`{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"c1","content":"key=SECRET"}]}`,
	"anthropic": `{"messages":[{"role":"user","content":"read it"},` +
		`{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"read_file","input":{}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"key=SECRET"}]}]}`,
	"google_ai": `{"contents":[{"role":"user","parts":[{"text":"read it"}]},` +
		`{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{}}}]},` +
		`{"role":"user","parts":[{"functionResponse":{"name":"read_file","response":{"content":"key=SECRET"}}}]}]}`,
}

func runTools(t *testing.T, p *fakeProvider, cfgExtra map[string]any, action, format, body string) *ScriptOutput {
	t.Helper()
	cfg := installFake(t, "fake_tools", p, action)
	for k, v := range cfgExtra {
		cfg[k] = v
	}
	out, err := NewFilterRunner(guardrailFilter("tools", false, cfg)).RunScript(&ScriptInput{RawInput: body, Format: format}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGuardrailRunner_ScansToolResultsByDefault(t *testing.T) {
	for format, body := range toolOnlyBodies {
		for _, scope := range []string{guardrails.ScopeLastUser, guardrails.ScopeAllUser, guardrails.ScopeSystemAndUser} {
			out := runTools(t, &fakeProvider{}, map[string]any{"scope": scope}, guardrails.ActionLog, format, body)
			if len(out.ComplianceEvents) != 1 {
				t.Errorf("%s/%s: a tool result was not scanned: events %+v", format, scope, out.ComplianceEvents)
				continue
			}
			md := out.ComplianceEvents[0].Metadata
			if !reflect.DeepEqual(md["roles"], []string{"tool"}) || !reflect.DeepEqual(md["kinds"], []string{"tool_result"}) ||
				!reflect.DeepEqual(md["tool_names"], []string{"read_file"}) {
				t.Errorf("%s/%s: event metadata %+v", format, scope, md)
			}
		}
		// scan: false is the selection by scope that applied before.
		out := runTools(t, &fakeProvider{}, map[string]any{"tool_results": map[string]any{"scan": false}}, guardrails.ActionLog, format, body)
		if len(out.ComplianceEvents) != 0 {
			t.Errorf("%s: scan false still scanned the tool result: %+v", format, out.ComplianceEvents)
		}
	}
}

// last_user is the current turn's user text; tool results are scanned in
// the whole history whatever the scope, because the client resends them.
func TestGuardrailRunner_LastUserIsTheCurrentTurn(t *testing.T) {
	body := `{"messages":[
		{"role":"user","content":"u0"},
		{"role":"assistant","content":[{"type":"text","text":"a0"},{"type":"tool_use","id":"t0","name":"f","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"t0","content":"r0"},{"type":"text","text":"u1"}]},
		{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"f","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"r1"}]}]}`
	cases := map[string][]string{
		guardrails.ScopeLastUser:    {"r0", "r1"}, // the current turn has no user text
		guardrails.ScopeAllUser:     {"u0", "u1", "r0", "r1"},
		guardrails.ScopeAllMessages: {"u0", "a0", "u1", "r0", "r1"},
	}
	for scope, want := range cases {
		p := &fakeProvider{}
		runTools(t, p, map[string]any{"scope": scope}, guardrails.ActionLog, "anthropic", body)
		if got := sawTexts(p); !reflect.DeepEqual(got, sortedTexts(want)) {
			t.Errorf("%s: provider saw %q, want %q", scope, got, want)
		}
	}

	// The current turn's user text, when there is some.
	p := &fakeProvider{}
	withText := strings.Replace(body, `"content":"r1"}]}]}`, `"content":"r1"},{"type":"text","text":"u2"}]}]}`, 1)
	runTools(t, p, map[string]any{"scope": guardrails.ScopeLastUser}, guardrails.ActionLog, "anthropic", withText)
	if got := sawTexts(p); !reflect.DeepEqual(got, sortedTexts([]string{"r0", "u2", "r1"})) {
		t.Errorf("last_user with current text: provider saw %q", got)
	}
}

func TestGuardrailRunner_BlockFilterQuarantinesToolResults(t *testing.T) {
	for format, body := range toolOnlyBodies {
		out := runTools(t, &fakeProvider{}, map[string]any{"quarantine": map[string]any{"placeholder": "[gone]"}}, guardrails.ActionBlock, format, body)
		if out.Block || strings.Contains(out.Payload, "SECRET") || !strings.Contains(out.Payload, "[gone]") {
			t.Errorf("%s: block %v payload %s", format, out.Block, out.Payload)
		}
		if out.ComplianceEvents[0].Metadata["action"] != guardrails.ActionQuarantine {
			t.Errorf("%s: event action %v", format, out.ComplianceEvents[0].Metadata["action"])
		}

		// An explicit tool action keeps its meaning.
		out = runTools(t, &fakeProvider{}, map[string]any{"tool_results": map[string]any{"on_detect": "block"}}, guardrails.ActionBlock, format, body)
		if !out.Block {
			t.Errorf("%s: tool_results.on_detect block did not block", format)
		}
	}
}

// A flagged user message under block still blocks the request, whatever
// happens to the tool results beside it.
func TestGuardrailRunner_UserTextUnderBlockStillBlocks(t *testing.T) {
	body := strings.Replace(toolOnlyBodies["openai"], `"read it"`, `"use SECRET"`, 1)
	out := runTools(t, &fakeProvider{}, nil, guardrails.ActionBlock, "openai", body)
	if !out.Block {
		t.Fatal("a flagged user message did not block")
	}
	for _, e := range out.ComplianceEvents {
		if e.Metadata["action"] != guardrails.ActionBlock {
			t.Errorf("event %s records action %v on a blocked request", e.EventType, e.Metadata["action"])
		}
	}
}

// Mixed actions: the user text is redacted and the tool result quarantined
// in one pass.
func TestGuardrailRunner_MixedActions(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	body := strings.Replace(toolOnlyBodies["openai"], `"read it"`, `"my SECRET"`, 1)
	out := runTools(t, p, map[string]any{"tool_results": map[string]any{"on_detect": "quarantine"}, "quarantine": map[string]any{"placeholder": "[gone]"}}, guardrails.ActionRedact, "openai", body)
	if out.Block {
		t.Fatalf("blocked: %+v", out.ComplianceEvents)
	}
	if !strings.Contains(out.Payload, `"my [REDACTED]"`) || !strings.Contains(out.Payload, `"content":"[gone]"`) {
		t.Errorf("payload %s", out.Payload)
	}
}

// A provider that does not declare ToolSegments gets a tool result as role
// user without tool metadata, and its findings land on the tool result; one
// that does gets role tool and the metadata.
func TestGuardrailRunner_ToolSegmentsCapability(t *testing.T) {
	for _, toolSegments := range []bool{false, true} {
		p := &fakeProvider{caps: guardrails.Capabilities{ToolSegments: toolSegments}}
		out := runTools(t, p, nil, guardrails.ActionLog, "openai", toolOnlyBodies["openai"])
		var tool guardrails.Segment
		for _, s := range p.seen() {
			if strings.Contains(s.Text, "SECRET") {
				tool = s
			}
		}
		want := guardrails.Segment{Role: "user", Text: "key=SECRET"}
		if toolSegments {
			want = guardrails.Segment{Role: "tool", Text: "key=SECRET", Kind: guardrails.KindToolResult, ToolName: "read_file", ToolCallID: "c1"}
		}
		want.Index = tool.Index
		if tool != want {
			t.Errorf("ToolSegments %v: provider got %+v, want %+v", toolSegments, tool, want)
		}
		if len(out.ComplianceEvents) != 1 || !reflect.DeepEqual(out.ComplianceEvents[0].Metadata["roles"], []string{"tool"}) {
			t.Errorf("ToolSegments %v: the finding did not land on the tool result: %+v", toolSegments, out.ComplianceEvents)
		}
	}
}
