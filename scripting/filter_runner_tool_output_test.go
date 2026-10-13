package scripting

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// A Studio chat tool output (FormatToolResult) is one tool result: read as
// JSON when it is, so an escaped newline cannot split a phrase, replaced
// whole by a quarantine, and left alone when tool-result scanning is off.

func toolOutputInput(text string) *ScriptInput {
	return &ScriptInput{
		RawInput: text,
		Messages: []llms.MessageContent{{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{llms.TextPart(text)}}},
		Format:   FormatToolResult,
		IsChat:   true,
		Context:  map[string]interface{}{"tool_name": "fetch", "tool_call_id": "c1"},
	}
}

func runToolOutput(t *testing.T, p *fakeProvider, action string, extra map[string]any, text string) *ScriptOutput {
	t.Helper()
	cfg := installFake(t, "fake_tool_output", p, action)
	for k, v := range extra {
		cfg[k] = v
	}
	out, err := NewFilterRunner(guardrailFilter("tool output", false, cfg)).RunScript(toolOutputInput(text), nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A text document whose bytes cannot be decoded was not scanned, so it never
// passes as clean: under block its tool result is quarantined.
func TestGuardrailRunner_UndecodableDocumentIsUnscanned(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"text":"read it"}]},{"role":"assistant","content":[{"toolUse":{"toolUseId":"t","name":"read","input":{}}}]},` +
		`{"role":"user","content":[{"toolResult":{"toolUseId":"t","content":[{"document":{"format":"txt","name":"page","source":{"bytes":"%%%"}}}]}}]}]}`
	p := &fakeProvider{}
	cfg := installFake(t, "fake_undecodable", p, guardrails.ActionBlock)
	cfg["quarantine"] = map[string]any{"placeholder": "[gone]"}
	out, err := NewFilterRunner(guardrailFilter("undecodable", false, cfg)).RunScript(&ScriptInput{RawInput: body, Format: "bedrock"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Block || strings.Contains(out.Payload, "%%%") || !strings.Contains(out.Payload, `"content":[{"text":"[gone]"}]`) {
		t.Errorf("block %v payload %s", out.Block, out.Payload)
	}
	found := false
	for _, e := range out.ComplianceEvents {
		if e.Metadata["reason"] == "undecodable" {
			found = true
		}
	}
	if !found {
		t.Errorf("no unscanned event for the undecodable document: %+v", out.ComplianceEvents)
	}
}

func TestGuardrailRunner_ToolOutput(t *testing.T) {
	// JSON is decoded: the provider sees the newline, not "\n".
	p := &fakeProvider{}
	runToolOutput(t, p, guardrails.ActionLog, nil, `{"body":"key\nSECRET"}`)
	if got := sawTexts(p); len(got) != 1 || !strings.Contains(got[0], "key\nSECRET") {
		t.Errorf("provider saw %q", got)
	}
	if s := p.lastIn.Segments[0]; s.Role != "user" {
		t.Errorf("a provider without ToolSegments got role %q", s.Role)
	}

	// Quarantine (block on a tool result) replaces the whole output, JSON or not.
	for _, text := range []string{`{"body":"key SECRET"}`, "plain SECRET text"} {
		out := runToolOutput(t, &fakeProvider{}, guardrails.ActionBlock, map[string]any{"quarantine": map[string]any{"placeholder": "[gone: {{source}}]"}}, text)
		if out.Block || out.Payload != "[gone: tool result from fetch]" {
			t.Errorf("%q: block %v payload %q", text, out.Block, out.Payload)
		}
		md := out.ComplianceEvents[0].Metadata
		if !reflect.DeepEqual(md["tool_names"], []string{"fetch"}) || md["action"] != guardrails.ActionQuarantine {
			t.Errorf("%q: event metadata %+v", text, md)
		}
	}

	// Redaction keeps the output valid JSON.
	out := runToolOutput(t, &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}, guardrails.ActionRedact, nil, `{"body":"key SECRET","n":1}`)
	if out.Payload != `{"body":"key [REDACTED]","n":1}` {
		t.Errorf("redacted payload %q", out.Payload)
	}

	// Scanning off: not the filter's business.
	p = &fakeProvider{}
	out = runToolOutput(t, p, guardrails.ActionBlock, map[string]any{"tool_results": map[string]any{"scan": false}}, "SECRET")
	if out.Block || p.calls != 0 || out.Payload != "SECRET" {
		t.Errorf("scan false: block %v calls %d payload %q", out.Block, p.calls, out.Payload)
	}
}
