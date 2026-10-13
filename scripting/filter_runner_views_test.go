package scripting

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// A tool result whose text is JSON is also classified decoded: a pattern
// over the raw text sees \u escapes and \n where the model reads characters.

// escapedToolBody is an OpenAI request whose tool message content is JSON
// text hiding "SECRET" behind a \u escape.
const escapedToolBody = `{"messages":[{"role":"user","content":"read it"},` +
	`{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},` +
	`{"role":"tool","tool_call_id":"c1","content":"{\"note\":\"key=\\u0053ECRET\"}"}]}`

func TestGuardrailRunner_DecodedViewOfJSONToolResults(t *testing.T) {
	// The provider sees the raw tool text and its decoded view.
	p := &fakeProvider{}
	out := runTools(t, p, nil, guardrails.ActionLog, "openai", escapedToolBody)
	got := strings.Join(sawTexts(p), "|")
	if len(sawTexts(p)) != 3 || !strings.Contains(got, `\u0053ECRET`) || !strings.Contains(got, "note\nkey=SECRET") {
		t.Fatalf("provider saw %q", sawTexts(p))
	}
	if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].Metadata["count"] != 1 {
		t.Errorf("events %+v", out.ComplianceEvents)
	}

	// Under block the tool result is quarantined.
	out = runTools(t, &fakeProvider{}, map[string]any{"quarantine": map[string]any{"placeholder": "[gone]"}}, guardrails.ActionBlock, "openai", escapedToolBody)
	if out.Block || !strings.Contains(out.Payload, `"content":"[gone]"`) {
		t.Errorf("block %v payload %s", out.Block, out.Payload)
	}

	// A redaction found only in the decoded view cannot be applied to the
	// escaped text: the tool result is quarantined instead.
	out = runTools(t, &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}, map[string]any{"quarantine": map[string]any{"placeholder": "[gone]"}}, guardrails.ActionRedact, "openai", escapedToolBody)
	if out.Block || !strings.Contains(out.Payload, `"content":"[gone]"`) || strings.Contains(out.Payload, "ECRET") {
		t.Errorf("redact: block %v payload %s", out.Block, out.Payload)
	}
}

// A chat tool output that is JSON with a repeated key cannot be read as a
// document, but its decoded strings, every copy included, are still
// classified: the repeated key does not buy a raw-text scan.
func TestGuardrailRunner_ToolOutputWithRepeatedKey(t *testing.T) {
	p := &fakeProvider{}
	out := runToolOutput(t, p, guardrails.ActionBlock, map[string]any{"quarantine": map[string]any{"placeholder": "[gone]"}},
		`{"a":1,"a":2,"note":"key=\u0053ECRET"}`)
	if got := sawTexts(p); len(got) != 2 || !strings.Contains(strings.Join(got, "|"), "a\na\nnote\nkey=SECRET") {
		t.Errorf("provider saw %q", got)
	}
	if out.Block || out.Payload != "[gone]" {
		t.Errorf("block %v payload %q", out.Block, out.Payload)
	}
}

// Escapes are read even when the tool's JSON is broken: prose around it,
// trailing text or a cut-off end.
func TestGuardrailRunner_EscapesInBrokenJSON(t *testing.T) {
	broken := map[string]string{
		"trailing text": `{\"note\":\"key=\\u0053ECRET\"} .`,
		"prose prefix":  `Result: {\"note\":\"key=\\u0053ECRET\"}`,
		"truncated":     `{\"note\":\"key=\\u0053ECRET\", \"more\": \"abc`,
	}
	for name, content := range broken {
		body := strings.Replace(toolOnlyBodies["openai"], `"key=SECRET"`, `"`+content+`"`, 1)
		out := runTools(t, &fakeProvider{}, nil, guardrails.ActionLog, "openai", body)
		if len(out.ComplianceEvents) != 1 {
			t.Errorf("proxy %s: not detected: %+v", name, out.ComplianceEvents)
		}
	}
	for name, text := range map[string]string{
		"trailing text": `{"note":"key=\u0053ECRET"} .`,
		"prose prefix":  `Result: {"note":"key=\u0053ECRET"}`,
		"truncated":     `{"note":"key=\u0053ECRET", "more": "abc`,
	} {
		out := runToolOutput(t, &fakeProvider{}, guardrails.ActionLog, nil, text)
		if len(out.ComplianceEvents) != 1 {
			t.Errorf("chat %s: not detected: %+v", name, out.ComplianceEvents)
		}
	}
}
