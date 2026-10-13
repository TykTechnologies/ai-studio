//go:build enterprise

package scripting

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// Script filters read a proxy request the way guardrails do through
// input.segments (each message's text, each tool result), and can
// quarantine one segment with tyk.quarantine_segment. input.messages is
// unchanged.

// ScanToolResultsTemplate is the "Built-in library: scan tool results"
// script template: it quarantines every tool result the built-in injection
// heuristics flag.
const ScanToolResultsTemplate = `// Quarantine tool results that carry prompt injection, using the built-in
// pattern library. input.segments lists the request's messages and tool
// results; tyk.quarantine_segment replaces one of them in the request body.
// A request that cannot be read into segments, or a flagged tool result
// that cannot be quarantined, is blocked rather than let through.
// Guardrail filters cover more (decoded JSON escapes, every provider) and
// are the recommended way to screen tool results; this template is a
// starting point for custom logic.

tyk := import("tyk")

body := input.raw_input
quarantined := 0
failed := input.format != "" && !input.segments_readable
for seg in input.segments {
    if seg.kind == "tool_result" && len(tyk.detect(seg.text, ["injection"])) > 0 {
        out := tyk.quarantine_segment({raw_input: body, format: input.format}, seg.index, "")
        if is_error(out) {
            failed = true
        } else {
            body = out
            quarantined += 1
        }
    }
}

output := {
    block: failed,
    payload: body,
    message: failed ? "Request blocked: a tool result could not be screened" : "",
    compliance_events: quarantined > 0 ? [
        {
            event_type: "tool_result_quarantined",
            severity: "warning",
            description: "Quarantined tool results carrying prompt injection",
            metadata: {count: quarantined}
        }
    ] : []
}`

func runScriptFilter(t *testing.T, script string, input *ScriptInput) *ScriptOutput {
	t.Helper()
	out, err := NewFilterRunner(&models.Filter{Name: "script", Script: []byte(script)}).RunScript(input, nil)
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	return out
}

func TestScriptSegmentsListToolResults(t *testing.T) {
	script := `
json := import("json")
kinds := []
for s in input.segments { kinds = append(kinds, s.role + ":" + s.kind + ":" + s.tool_name) }
output := {block: false, payload: string(json.encode(kinds))}`
	for format, body := range toolOnlyBodies {
		out := runScriptFilter(t, script, &ScriptInput{RawInput: body, Format: format})
		var kinds []string
		if err := json.Unmarshal([]byte(out.Payload), &kinds); err != nil {
			t.Fatalf("%s: %v (%s)", format, err, out.Payload)
		}
		if !reflect.DeepEqual(kinds, []string{"user:text:", "tool:tool_result:read_file"}) {
			t.Errorf("%s: segments %v", format, kinds)
		}
	}
}

// input.messages is what it always was; segments are an addition.
func TestScriptMessagesUnchanged(t *testing.T) {
	script := `
json := import("json")
output := {block: false, payload: string(json.encode(input.messages))}`
	msgs := []llms.MessageContent{
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart("read it")}},
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart("")}},
	}
	out := runScriptFilter(t, script, &ScriptInput{RawInput: toolOnlyBodies["anthropic"], Format: "anthropic", Messages: msgs})
	var got []map[string]string
	if err := json.Unmarshal([]byte(out.Payload), &got); err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"role": "user", "content": "read it"}, {"role": "user", "content": ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("input.messages = %s", out.Payload)
	}
}

func TestScriptQuarantineSegment(t *testing.T) {
	script := `
tyk := import("tyk")
out := tyk.quarantine_segment(input, 1, "[gone]")
bad := tyk.quarantine_segment(input, 9, "")
output := {block: !is_error(bad), payload: is_error(out) ? input.raw_input : out}`
	for format, body := range toolOnlyBodies {
		out := runScriptFilter(t, script, &ScriptInput{RawInput: body, Format: format})
		if out.Block {
			t.Errorf("%s: an out-of-range index did not return an error", format)
		}
		if strings.Contains(out.Payload, "SECRET") || !strings.Contains(out.Payload, "[gone]") || !strings.Contains(out.Payload, "read it") {
			t.Errorf("%s: payload %s", format, out.Payload)
		}
	}

	// A chat message has no segments to quarantine.
	out := runScriptFilter(t, script, &ScriptInput{RawInput: "hello", IsChat: true})
	if out.Payload != "hello" {
		t.Errorf("chat input: payload %q", out.Payload)
	}
}

func TestScanToolResultsTemplate(t *testing.T) {
	for format := range toolOnlyBodies {
		body := injectionInToolResult(format)
		out := runScriptFilter(t, ScanToolResultsTemplate, &ScriptInput{RawInput: body, Format: format})
		if out.Block || strings.Contains(out.Payload, "Ignore all previous") || !strings.Contains(out.Payload, "[content removed by policy]") {
			t.Errorf("%s: payload %s", format, out.Payload)
		}
		if len(out.ComplianceEvents) != 1 {
			t.Errorf("%s: events %+v", format, out.ComplianceEvents)
		}
		// What cannot be screened is blocked: a body the guardrail cannot
		// read (here a repeated key).
		unreadable := strings.Replace(body, `{"role":"user"`, `{"role":"user","role":"user"`, 1)
		if out := runScriptFilter(t, ScanToolResultsTemplate, &ScriptInput{RawInput: unreadable, Format: format}); !out.Block {
			t.Errorf("%s: an unreadable body was let through", format)
		}
		// A clean request passes as it came.
		clean := runScriptFilter(t, ScanToolResultsTemplate, &ScriptInput{RawInput: toolOnlyBodies[format], Format: format})
		if clean.Payload != toolOnlyBodies[format] || len(clean.ComplianceEvents) != 0 {
			t.Errorf("%s: a clean request was changed: %s", format, clean.Payload)
		}
	}
}
