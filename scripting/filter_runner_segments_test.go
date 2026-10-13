package scripting

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

const anthropicAgentBody = `{"model":"m","max_tokens":5,"system":"sys SECRET",
	"messages":[
		{"role":"user","content":"first SECRET"},
		{"role":"assistant","content":[{"type":"text","text":"calling SECRET"},{"type":"tool_use","id":"toolu_1","name":"read","input":{}}]},
		{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"toolu_1","content":"tool SECRET"},
			{"type":"text","text":"second SECRET"}
		]},
		{"role":"assistant","content":[{"type":"tool_use","id":"toolu_2","name":"read","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_2","content":"tool2 SECRET"}]}
	]}`

// sawTexts is every text the provider was sent, sorted: the client's text
// and tool results go in separate calls, which run concurrently, so the
// order across calls is not fixed. Compare with sortedTexts(want).
func sawTexts(p *fakeProvider) []string {
	var got []string
	for _, s := range p.seen() {
		got = append(got, s.Text)
	}
	return sortedTexts(got)
}

func sortedTexts(texts []string) []string {
	out := slices.Clone(texts)
	slices.Sort(out)
	return out
}

// Proxy requests in a segmentable format are read block by block. With
// tool-result scanning off, selection is by role: tool results only under
// all_messages, last_user is the user text of the last message with any.
func TestGuardrailRunner_BodyScopeSelectsBlocks(t *testing.T) {
	cases := map[string][]string{
		guardrails.ScopeLastUser:      {"second SECRET"},
		guardrails.ScopeAllUser:       {"first SECRET", "second SECRET"},
		guardrails.ScopeSystemAndUser: {"sys SECRET", "first SECRET", "second SECRET"},
		// A message's text is one unit, each tool result another, in the
		// order they open: the message, then the tool results inside it.
		guardrails.ScopeAllMessages: {"sys SECRET", "first SECRET", "calling SECRET", "second SECRET", "tool SECRET", "tool2 SECRET"},
	}
	for scope, want := range cases {
		p := &fakeProvider{}
		cfg := installFake(t, "fake_body_scope_"+scope, p, guardrails.ActionLog)
		cfg["scope"] = scope
		cfg["tool_results"] = map[string]any{"scan": false}
		out, err := NewFilterRunner(guardrailFilter("scope", false, cfg)).RunScript(&ScriptInput{
			RawInput: anthropicAgentBody, Format: "anthropic",
			// The lossy extractor's view; body mode must not use it.
			Messages: []llms.MessageContent{userMsg("ignored")},
		}, nil)
		if err != nil {
			t.Fatalf("%s: RunScript() error = %v", scope, err)
		}
		if got := sawTexts(p); !reflect.DeepEqual(got, sortedTexts(want)) {
			t.Errorf("%s: provider saw %q, want %q", scope, got, want)
		}
		for _, s := range p.seen() {
			if s.Kind != "" || s.ToolName != "" || s.ToolCallID != "" {
				t.Errorf("%s: provider segment carries tool metadata before tool scanning is on: %+v", scope, s)
			}
		}
		if out.Payload != anthropicAgentBody || out.Messages != nil {
			t.Errorf("%s: log must leave the body alone", scope)
		}
	}
}

func TestGuardrailRunner_RedactsBodyInPlace(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	cfg := installFake(t, "fake_body_redact", p, guardrails.ActionRedact)
	cfg["scope"] = guardrails.ScopeAllMessages
	out, err := NewFilterRunner(guardrailFilter("redact", false, cfg)).RunScript(&ScriptInput{
		RawInput: anthropicAgentBody, Format: "anthropic",
	}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Block || out.Messages != nil {
		t.Fatalf("redact must hand back a payload: block=%v messages=%v", out.Block, out.Messages)
	}
	want := strings.ReplaceAll(anthropicAgentBody, "SECRET", "[REDACTED]")
	if out.Payload != want {
		t.Fatalf("payload:\n got %s\nwant %s", out.Payload, want)
	}
	if len(out.ComplianceEvents) != 1 || !reflect.DeepEqual(out.ComplianceEvents[0].Metadata["roles"], []string{"assistant", "system", "tool", "user"}) {
		t.Errorf("compliance events = %+v", out.ComplianceEvents)
	}
}

// A body whose keys parsers may read differently (a repeated key, or a case
// variant that Go struct decoding on the Bedrock bridges would read) is
// refused, whatever the action: the guardrail cannot know which copy the
// vendor reads.
func TestGuardrailRunner_RefusesAmbiguousKeys(t *testing.T) {
	for name, body := range map[string]string{
		"repeated key":       `{"messages":[{"role":"user","content":"hello","content":"SECRET"}]}`,
		"case variant":       `{"messages":[{"role":"user","content":"hello","Content":"SECRET"}]}`,
		"repeated deep down": `{"messages":[{"role":"user","content":"hello"}],"tools":[{"a":1,"a":2}]}`,
	} {
		p := &fakeProvider{}
		cfg := installFake(t, "fake_refuse", p, guardrails.ActionLog)
		out, err := NewFilterRunner(guardrailFilter("refuse", false, cfg)).RunScript(&ScriptInput{
			RawInput: body, Format: "openai",
			Messages: []llms.MessageContent{userMsg("SECRET")},
		}, nil)
		if err != nil {
			t.Fatalf("%s: RunScript() error = %v", name, err)
		}
		if !out.Block || p.calls != 0 {
			t.Errorf("%s: block=%v provider calls=%d, want a refusal before any provider call", name, out.Block, p.calls)
		}
		if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].EventType != "guardrail.unreadable_request" {
			t.Errorf("%s: compliance events = %+v", name, out.ComplianceEvents)
		}
	}
}

// A JSON body no segmenter reads (another format's shape, or a format with
// no segmenter and no extractor, such as Bedrock Converse) is classified
// with every string decoded, so a JSON escape cannot hide a phrase, and a
// redaction still lands in place. Only a body that is not JSON is
// classified as raw bytes.
func TestGuardrailRunner_GenericFallback(t *testing.T) {
	converse := `{"messages":[{"role":"user","content":[{"toolResult":{"toolUseId":"t1","content":[{"text":"key\nSECRET"}]}}]}]}`
	for name, tc := range map[string]struct{ format, body string }{
		"converse, no segmenter":     {"huggingface", converse},
		"converse under openai":      {"openai", converse},
		"openai body under google":   {"google_ai", `{"messages":[{"role":"tool","content":"key\nSECRET"}]}`},
		"no extractor, no segmenter": {"huggingface", `{"inputs":"key\nSECRET"}`},
	} {
		p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
		cfg := installFake(t, "fake_generic", p, guardrails.ActionRedact)
		out, err := NewFilterRunner(guardrailFilter("generic", false, cfg)).RunScript(&ScriptInput{RawInput: tc.body, Format: tc.format}, nil)
		if err != nil {
			t.Fatalf("%s: RunScript() error = %v", name, err)
		}
		if got := sawTexts(p); len(got) != 1 || !strings.Contains(got[0], "key\nSECRET") {
			t.Errorf("%s: provider saw %q, want the decoded strings", name, got)
		}
		if want := strings.Replace(tc.body, "SECRET", "[REDACTED]", 1); out.Payload != want {
			t.Errorf("%s: payload %s, want %s", name, out.Payload, want)
		}
	}

	p := &fakeProvider{}
	cfg := installFake(t, "fake_raw", p, guardrails.ActionBlock)
	out, _ := NewFilterRunner(guardrailFilter("raw", false, cfg)).RunScript(&ScriptInput{RawInput: "--b\r\nSECRET", Format: "openai"}, nil)
	if got := sawTexts(p); len(got) != 1 || got[0] != "--b\r\nSECRET" || !out.Block {
		t.Errorf("non-JSON body: provider saw %q, block %v", got, out.Block)
	}
}

// Detection is per message and per tool result: a phrase split across two
// blocks of one message is one text to the provider, and the redaction
// still lands in the right block.
func TestGuardrailRunner_DetectsAcrossBlocks(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}, verdict: func(in guardrails.Input) guardrails.Verdict {
		v := guardrails.Verdict{Rewritten: map[int]string{}}
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "SEC\n\f\nRET") {
				v.Flagged = true
				v.Findings = append(v.Findings, guardrails.Finding{Detector: "split", SegmentIndex: s.Index})
			}
			if strings.Contains(s.Text, "mail a@b.c") && in.Redact != nil {
				v.Flagged = true
				v.Findings = append(v.Findings, guardrails.Finding{Detector: "email", SegmentIndex: s.Index})
				v.Rewritten[s.Index] = strings.Replace(s.Text, "a@b.c", "[EMAIL]", 1)
			}
		}
		return v
	}}
	split := `{"messages":[{"role":"user","content":[{"type":"text","text":"SEC"},{"type":"text","text":"RET"}]}]}`
	cfg := installFake(t, "fake_split_detect", p, guardrails.ActionBlock)
	out, err := NewFilterRunner(guardrailFilter("split", false, cfg)).RunScript(&ScriptInput{RawInput: split, Format: "anthropic"}, nil)
	if err != nil || !out.Block {
		t.Fatalf("a phrase split across two blocks was not caught: block=%v err=%v", out.Block, err)
	}

	mail := `{"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":"mail a@b.c"}]}]}`
	cfg = installFake(t, "fake_split_redact", p, guardrails.ActionRedact)
	out, err = NewFilterRunner(guardrailFilter("redact", false, cfg)).RunScript(&ScriptInput{RawInput: mail, Format: "anthropic"}, nil)
	if err != nil || out.Payload != strings.Replace(mail, "a@b.c", "[EMAIL]", 1) {
		t.Fatalf("payload %s err %v", out.Payload, err)
	}
}

// A redaction that cannot be mapped back onto the body's strings (it ran
// across a block boundary, or touched a functionResponse key) is something
// whoever wrote the text can arrange, so it never falls to the fail mode,
// not even fail_mode open: user text blocks the request, a tool result is
// quarantined, and the redactions that did map are still applied.
func TestGuardrailRunner_UnmappableRedactionNeverPasses(t *testing.T) {
	// The provider rewrites every flagged unit to one token, which only maps
	// back onto a single-leaf unit.
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}, verdict: func(in guardrails.Input) guardrails.Verdict {
		v := guardrails.Verdict{Rewritten: map[int]string{}}
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "SECRET") {
				v.Flagged = true
				v.Findings = append(v.Findings, guardrails.Finding{Detector: "name", SegmentIndex: s.Index})
				v.Rewritten[s.Index] = "[NAME]"
			}
		}
		return v
	}}
	run := func(body string) *ScriptOutput {
		t.Helper()
		cfg := installFake(t, "fake_unmappable", p, guardrails.ActionRedact)
		cfg["scope"] = guardrails.ScopeAllMessages
		cfg["fail_mode"] = guardrails.FailOpen
		cfg["quarantine"] = map[string]any{"placeholder": "[gone: {{source}}]"}
		out, err := NewFilterRunner(guardrailFilter("unmappable", false, cfg)).RunScript(&ScriptInput{RawInput: body, Format: "anthropic"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	eventTypes := func(out *ScriptOutput) []string {
		var types []string
		for _, e := range out.ComplianceEvents {
			types = append(types, e.EventType)
		}
		return types
	}

	// User text over two blocks, redacted as a whole: blocked.
	out := run(`{"messages":[{"role":"user","content":[{"type":"text","text":"my SECRET"},{"type":"text","text":"John"}]}]}`)
	if !out.Block || out.Payload != "" {
		t.Fatalf("an unmappable redaction of user text let the request through: %+v", out)
	}
	if got := eventTypes(out); !reflect.DeepEqual(got, []string{"guardrail.name", "guardrail.unredactable"}) {
		t.Errorf("compliance events = %v", got)
	}

	// A tool result split across two blocks is quarantined; the user message
	// beside it is redacted as usual.
	body := `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"read","input":{}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"a SECRET"},{"type":"text","text":"b"}]},{"type":"text","text":"my SECRET"}]}]}`
	out = run(body)
	if out.Block {
		t.Fatalf("a tool result that could not be redacted should be quarantined, not blocked: %+v", out.ComplianceEvents)
	}
	jsonEqual(t, out.Payload, `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"read","input":{}}]},`+
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"[gone: tool result from read]"},{"type":"text","text":"[NAME]"}]}]}`)
	last := out.ComplianceEvents[len(out.ComplianceEvents)-1]
	if last.EventType != "guardrail.unredactable" || last.Metadata["action"] != guardrails.ActionQuarantine || !reflect.DeepEqual(last.Metadata["tool_names"], []string{"read"}) {
		t.Errorf("unredactable event = %+v", last)
	}
}

// Formats the segmenter does not cover keep the extracted messages and the
// reconstructor path.
func TestGuardrailRunner_OtherFormatsKeepMessages(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	cfg := installFake(t, "fake_other_format", p, guardrails.ActionRedact)
	out, err := NewFilterRunner(guardrailFilter("other", false, cfg)).RunScript(&ScriptInput{
		RawInput: `{"input":"SECRET"}`, Format: "openai_decisions",
		Messages: []llms.MessageContent{systemMsg("q"), userMsg("evidence SECRET")},
	}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if len(out.Messages) != 2 || out.Messages[1]["content"] != "evidence [REDACTED]" || out.Payload != "" {
		t.Errorf("RunScript() Messages = %v Payload = %q", out.Messages, out.Payload)
	}
}

// The segment path is what reaches the body: the rewrite keeps the other
// fields of the block it lands in.
func TestGuardrailRunner_BodyRedactKeepsSiblings(t *testing.T) {
	p := &fakeProvider{caps: guardrails.Capabilities{Redacts: true}}
	cfg := installFake(t, "fake_body_siblings", p, guardrails.ActionRedact)
	body := `{"contents":[{"role":"user","parts":[{"text":"key SECRET","thought":false},{"inlineData":{"mimeType":"image/png","data":"AAAA"}}]}]}`
	out, err := NewFilterRunner(guardrailFilter("siblings", false, cfg)).RunScript(&ScriptInput{RawInput: body, Format: "google_ai"}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	var got, want any
	_ = json.Unmarshal([]byte(out.Payload), &got)
	_ = json.Unmarshal([]byte(strings.Replace(body, "SECRET", "[REDACTED]", 1)), &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("payload = %s", out.Payload)
	}
}
