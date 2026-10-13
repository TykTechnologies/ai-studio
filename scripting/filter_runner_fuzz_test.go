package scripting

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// FuzzGuardrailRunner runs request bodies and chat tool outputs of any shape
// through the guardrail runner with a provider that flags and rewrites
// "SECRET", under every action. Nothing panics past the runner's own
// recover, and the runner never reports an internal error (a recovered
// panic) for any input.
func FuzzGuardrailRunner(f *testing.F) {
	for _, body := range toolOnlyBodies {
		f.Add(uint8(0), body)
	}
	f.Add(uint8(1), `{"a":1,"a":2,"note":"key=SECRET"}`)
	f.Add(uint8(2), `Result: {"note":"SECRET", "more": "ab`)
	f.Add(uint8(3), `{"messages":[{"role":"user","content":[{"toolResult":{"toolUseId":"t","content":[{"document":{"format":"txt","source":{"bytes":"U0VDUkVU"}}}]}}]}]}`)

	formats := []string{"openai", "anthropic", "google_ai", "bedrock", "ollama", "huggingface", FormatToolResult}
	actions := []string{guardrails.ActionBlock, guardrails.ActionRedact, guardrails.ActionQuarantine, guardrails.ActionLog}
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	cfgs := make([]map[string]any, len(actions))
	for i, a := range actions {
		cfgs[i] = installFake(f, "fake_fuzz_"+a, p, a)
		cfgs[i]["quarantine"] = map[string]any{"placeholder": "[q]"}
	}
	f.Fuzz(func(t *testing.T, sel uint8, body string) {
		format := formats[int(sel)%len(formats)]
		cfg := cfgs[int(sel/8)%len(cfgs)]
		input := &ScriptInput{RawInput: body, Format: format}
		if format == FormatToolResult {
			input = toolOutputInput(body)
		}
		out, err := NewFilterRunner(guardrailFilter("fuzz", false, cfg)).RunScript(input, nil)
		if err != nil {
			t.Fatalf("RunScript error: %v", err)
		}
		for _, e := range out.ComplianceEvents {
			if e.Metadata["error_class"] == "panic" {
				t.Fatalf("the runner panicked on %q (%s)", body, format)
			}
		}
	})
}
