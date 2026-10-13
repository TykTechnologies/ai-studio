package scripting

import (
	"slices"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// A vendor may set a unit's text blocks side by side, so the runner also
// classifies them joined with nothing between: a word split across two
// blocks is whole there.

// wordProvider flags "SECRET" as a whole word anywhere, and rewrites it.
func wordProvider() *fakeProvider {
	return &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
}

func runJoined(t *testing.T, action, body string, extra map[string]any) (*ScriptOutput, *fakeProvider) {
	t.Helper()
	p := wordProvider()
	cfg := installFake(t, "fake_joined", p, action)
	cfg["scope"] = guardrails.ScopeAllMessages
	for k, v := range extra {
		cfg[k] = v
	}
	out, err := NewFilterRunner(guardrailFilter("joined", false, cfg)).RunScript(&ScriptInput{RawInput: body, Format: "anthropic"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out, p
}

const splitWordUser = `{"messages":[{"role":"user","content":[{"type":"text","text":"key SEC"},{"type":"text","text":"RET here"}]}]}`

const splitWordTool = `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"read","input":{}}]},` +
	`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"key SEC"},{"type":"text","text":"RET here"}]},{"type":"text","text":"thanks"}]}]}`

func TestGuardrailRunner_JoinedTextCatchesSplitWords(t *testing.T) {
	out, p := runJoined(t, guardrails.ActionBlock, splitWordUser, nil)
	if !out.Block {
		t.Fatal("a word split across two text blocks was not caught")
	}
	if got := sawTexts(p); len(got) != 2 || !slices.Contains(got, "key SECRET here") {
		t.Errorf("provider saw %q, want the unit text and its joined form", got)
	}
	if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].Metadata["count"] != 1 || out.ComplianceEvents[0].Metadata["roles"].([]string)[0] != "user" {
		t.Errorf("compliance events = %+v", out.ComplianceEvents)
	}
}

// A finding the unit's own text also has is not counted twice, and its
// redaction lands as usual.
func TestGuardrailRunner_JoinedTextDuplicatesAreDropped(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"text","text":"my SECRET"},{"type":"text","text":"ok"}]}]}`
	out, _ := runJoined(t, guardrails.ActionRedact, body, nil)
	if out.Block || out.Payload != strings.Replace(body, "SECRET", "[REDACTED]", 1) {
		t.Fatalf("block %v payload %s", out.Block, out.Payload)
	}
	if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].Metadata["count"] != 1 {
		t.Errorf("compliance events = %+v", out.ComplianceEvents)
	}
}

// A split word cannot be redacted in place: user text blocks, a tool result
// is quarantined.
func TestGuardrailRunner_JoinedTextOnlyFindingIsUnredactable(t *testing.T) {
	out, _ := runJoined(t, guardrails.ActionRedact, splitWordUser, map[string]any{"fail_mode": guardrails.FailOpen})
	if !out.Block {
		t.Errorf("a redaction of a split word in user text let the request through: %s", out.Payload)
	}

	out, _ = runJoined(t, guardrails.ActionRedact, splitWordTool, map[string]any{"fail_mode": guardrails.FailOpen, "quarantine": map[string]any{"placeholder": "[gone]"}})
	if out.Block {
		t.Fatalf("a split word in a tool result should be quarantined: %+v", out.ComplianceEvents)
	}
	jsonEqual(t, out.Payload, `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"read","input":{}}]},`+
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"[gone]"},{"type":"text","text":"thanks"}]}]}`)
}

// Quarantine replaces a unit once, however many of its segments flagged it.
func TestGuardrailRunner_JoinedTextQuarantinesOnce(t *testing.T) {
	out, _ := runJoined(t, guardrails.ActionQuarantine, splitWordTool, map[string]any{"quarantine": map[string]any{"placeholder": "[gone]"}})
	if out.Block {
		t.Fatalf("blocked: %+v", out.ComplianceEvents)
	}
	if !strings.Contains(out.Payload, `"content":"[gone]"`) || strings.Contains(out.Payload, "SEC") {
		t.Errorf("payload %s", out.Payload)
	}

	// A finding that names no segment flags every unit, not the joined
	// copies a second time.
	p := &fakeProvider{verdict: func(in guardrails.Input) guardrails.Verdict {
		return guardrails.Verdict{Flagged: true, Findings: []guardrails.Finding{{Detector: "x", SegmentIndex: -1}}}
	}}
	cfg := installFake(t, "fake_joined_unplaced", p, guardrails.ActionQuarantine)
	cfg["scope"] = guardrails.ScopeAllMessages
	cfg["quarantine"] = map[string]any{"placeholder": "[gone]"}
	out, err := NewFilterRunner(guardrailFilter("joined", false, cfg)).RunScript(&ScriptInput{RawInput: splitWordTool, Format: "anthropic"}, nil)
	if err != nil || out.Block {
		t.Fatalf("err %v block %v events %+v", err, out.Block, out.ComplianceEvents)
	}
}

// Edits that cannot be written into the body (a tool_result nested in
// another, so the outer replacement overlaps the inner one) block the
// request whatever the fail mode: the content was flagged.
func TestGuardrailRunner_RewriteFailureBlocks(t *testing.T) {
	nested := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[` +
		`{"type":"text","text":"a SECRET"},{"type":"text","text":"b"},` +
		`{"type":"tool_result","tool_use_id":"t2","content":"SECRET x"}]}]}]}`
	for _, action := range []string{guardrails.ActionQuarantine, guardrails.ActionRedact} {
		p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}, verdict: func(in guardrails.Input) guardrails.Verdict {
			v := guardrails.Verdict{Rewritten: map[int]string{}}
			for _, s := range in.Segments {
				if strings.Contains(s.Text, "SECRET") {
					v.Flagged = true
					v.Findings = append(v.Findings, guardrails.Finding{Detector: "x", SegmentIndex: s.Index})
					v.Rewritten[s.Index] = "[X]"
				}
			}
			return v
		}}
		cfg := installFake(t, "fake_rewrite_fail", p, action)
		cfg["scope"] = guardrails.ScopeAllMessages
		cfg["fail_mode"] = guardrails.FailOpen
		out, err := NewFilterRunner(guardrailFilter("nested", false, cfg)).RunScript(&ScriptInput{RawInput: nested, Format: "anthropic"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !out.Block || out.Payload != "" {
			t.Errorf("%s: an edit that could not be applied let flagged content through: block %v payload %s", action, out.Block, out.Payload)
		}
		for _, e := range out.ComplianceEvents {
			if a, ok := e.Metadata["action"]; ok && a != guardrails.ActionBlock {
				t.Errorf("%s: event %s records action %v", action, e.EventType, a)
			}
		}
	}
}
