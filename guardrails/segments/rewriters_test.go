package segments

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

func TestCheckToolPairing(t *testing.T) {
	type tc struct {
		name, format, body string
		bad                string // "" when the body pairs; else part of the error
	}
	cases := []tc{
		// OpenAI
		{"openai paired", FormatOpenAI, `{"messages":[{"role":"user","content":"hi"},
			{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"c1","content":"ok"}]}`, ""},
		{"openai parallel", FormatOpenAI, `{"messages":[
			{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f"}},{"id":"c2","function":{"name":"g"}}]},
			{"role":"tool","tool_call_id":"c2","content":""},{"role":"tool","tool_call_id":"c1","content":"ok"}]}`, ""},
		{"openai orphan result", FormatOpenAI, `{"messages":[{"role":"tool","tool_call_id":"c9","content":"ok"}]}`, `result for no call "c9"`},
		{"openai missing result", FormatOpenAI, `{"messages":[
			{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f"}},{"id":"c2","function":{"name":"g"}}]},
			{"role":"tool","tool_call_id":"c1","content":"ok"}]}`, `no result for call "c2"`},
		{"openai duplicate result", FormatOpenAI, `{"messages":[
			{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f"}}]},
			{"role":"tool","tool_call_id":"c1","content":"a"},{"role":"tool","tool_call_id":"c1","content":"b"}]}`, `2 results for call "c1"`},
		{"openai result without id", FormatOpenAI, `{"messages":[{"role":"tool","content":"ok"}]}`, "without tool_call_id"},
		{"ollama id-less", FormatOllama, `{"messages":[
			{"role":"assistant","tool_calls":[{"function":{"name":"f","arguments":{}}}]},
			{"role":"tool","tool_name":"f","content":"ok"}]}`, ""},

		// Anthropic
		{"anthropic paired", FormatAnthropic, `{"messages":[
			{"role":"assistant","content":[{"type":"text","text":"x"},{"type":"tool_use","id":"t1","name":"f","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`, ""},
		{"anthropic parallel and server tools", FormatAnthropic, `{"messages":[
			{"role":"assistant","content":[
				{"type":"server_tool_use","id":"s1","name":"web_search","input":{}},
				{"type":"web_search_tool_result","tool_use_id":"s1","content":[]},
				{"type":"mcp_tool_use","id":"m1","name":"x","server_name":"s","input":{}},
				{"type":"mcp_tool_result","tool_use_id":"m1","content":[]},
				{"type":"tool_use","id":"t1","name":"f","input":{}},{"type":"tool_use","id":"t2","name":"g","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2"},{"type":"tool_result","tool_use_id":"t1"}]}]}`, ""},
		{"anthropic orphan result", FormatAnthropic, `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t9","content":"ok"}]}]}`, `result for no call "t9"`},
		{"anthropic missing result", FormatAnthropic, `{"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"f","input":{}},{"type":"tool_use","id":"t2","name":"g","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"}]}]}`, `no result for call "t2"`},
		{"anthropic duplicate call id", FormatAnthropic, `{"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"f","input":{}},{"type":"tool_use","id":"t1","name":"g","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"}]}]}`, `2 calls with id "t1"`},
		{"bedrock invoke anthropic body", FormatBedrock, `{"anthropic_version":"bedrock-2023-05-31","messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"f","input":{}}]}]}`, `no result for call "t1"`},

		// Google
		{"google paired by id", FormatGoogleAI, `{"contents":[
			{"role":"model","parts":[{"functionCall":{"id":"g1","name":"f","args":{}}}]},
			{"role":"user","parts":[{"functionResponse":{"id":"g1","name":"f","response":{}}}]}]}`, ""},
		{"google parallel by position", FormatVertex, `{"contents":[
			{"role":"user","parts":[{"text":"hi"}]},
			{"role":"model","parts":[{"functionCall":{"name":"f","args":{}}},{"function_call":{"name":"g","args":{}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"f","response":{}}},{"function_response":{"name":"g","response":{}}}]},
			{"role":"model","parts":[{"text":"done"}]}]}`, ""},
		{"google position missing response", FormatGoogleAI, `{"contents":[
			{"role":"model","parts":[{"functionCall":{"name":"f","args":{}}},{"functionCall":{"name":"g","args":{}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"f","response":{}}}]}]}`, "no response for 1 id-less call"},
		{"google position orphan response", FormatGoogleAI, `{"contents":[
			{"role":"user","parts":[{"functionResponse":{"name":"f","response":{}}}]}]}`, "answers no call"},
		{"google id orphan", FormatGoogleAI, `{"contents":[
			{"role":"user","parts":[{"functionResponse":{"id":"g9","name":"f","response":{}}}]}]}`, `result for no call "g9"`},
		{"google calls of an earlier turn left unanswered", FormatGoogleAI, `{"contents":[
			{"role":"model","parts":[{"functionCall":{"name":"f","args":{}}}]},
			{"role":"model","parts":[{"functionCall":{"name":"g","args":{}}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"g","response":{}}}]}]}`, "content 0"},

		// Bedrock Converse
		{"bedrock paired", FormatBedrock, `{"messages":[
			{"role":"assistant","content":[{"toolUse":{"toolUseId":"b1","name":"f","input":{}}},{"toolUse":{"toolUseId":"b2","name":"g","input":{}}}]},
			{"role":"user","content":[{"toolResult":{"toolUseId":"b1","content":[{"text":"x"}]}},{"toolResult":{"toolUseId":"b2","content":[]}}]}]}`, ""},
		{"bedrock orphan", FormatBedrock, `{"messages":[{"role":"user","content":[{"toolResult":{"toolUseId":"b9","content":[]}}]}]}`, `result for no call "b9"`},
		{"bedrock missing", FormatBedrock, `{"messages":[{"role":"assistant","content":[{"toolUse":{"toolUseId":"b1","name":"f","input":{}}}]}]}`, `no result for call "b1"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckToolPairing(c.format, []byte(c.body))
			if c.bad == "" {
				if err != nil {
					t.Fatalf("CheckToolPairing: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrToolPairing) || !strings.Contains(err.Error(), c.bad) {
				t.Fatalf("err = %v, want ErrToolPairing mentioning %q", err, c.bad)
			}
		})
	}
}

func TestCheckToolPairingErrors(t *testing.T) {
	if err := CheckToolPairing("huggingface", []byte(`{}`)); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("unsupported format: %v", err)
	}
	if err := CheckToolPairing(FormatOpenAI, []byte(`[1]`)); !errors.Is(err, ErrUnrecognisedBody) {
		t.Errorf("array body: %v", err)
	}
	if err := CheckToolPairing(FormatOpenAI, []byte(`{"messages":[],"messages":[]}`)); !errors.Is(err, ErrDuplicateKey) {
		t.Errorf("duplicate key: %v", err)
	}
	// Many problems are summarised, not all listed.
	var b strings.Builder
	b.WriteString(`{"messages":[`)
	for i := 0; i < 8; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"role":"tool","tool_call_id":"x` + string(rune('a'+i)) + `","content":"r"}`)
	}
	b.WriteString(`]}`)
	err := CheckToolPairing(FormatOpenAI, []byte(b.String()))
	if err == nil || !strings.Contains(err.Error(), "and 3 more") {
		t.Errorf("err = %v, want a summary", err)
	}
}

func TestCacheBreakpoints(t *testing.T) {
	type want struct {
		path            string
		section, msg, k int
	}
	for _, c := range []struct {
		name, format, body string
		want               []want
	}{
		{"system level", FormatAnthropic, `{"system":[{"type":"text","text":"a"},{"type":"text","text":"b","cache_control":{"type":"ephemeral"}}],
			"messages":[{"role":"user","content":"hi"}]}`,
			[]want{{"/system/1", SectionSystem, -1, 1}}},
		{"block level", FormatAnthropic, `{"messages":[{"role":"user","content":[{"type":"text","text":"a"},{"type":"text","text":"b","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
			[]want{{"/messages/0/content/1", SectionMessages, 0, 1}}},
		{"message level", FormatAnthropic, `{"messages":[{"role":"user","content":"a","cache_control":{"type":"ephemeral"}},{"role":"assistant","content":"b"}]}`,
			[]want{{"/messages/0", SectionMessages, 0, math.MaxInt}}},
		{"multiple, in prompt order whatever the key order", FormatAnthropic, `{
			"messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}}]},
				{"role":"assistant","content":"b"},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":[{"type":"text","text":"r","cache_control":{"type":"ephemeral"}}]}]}],
			"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],
			"tools":[{"name":"f","input_schema":{},"cache_control":{"type":"ephemeral"}}]}`,
			[]want{{"/tools/0", SectionTools, -1, 0}, {"/system/0", SectionSystem, -1, 0},
				{"/messages/0/content/0", SectionMessages, 0, 0}, {"/messages/2/content/0/content/0", SectionMessages, 2, 0}}},
		{"request level", FormatAnthropic, `{"cache_control":{"type":"ephemeral"},"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`,
			[]want{{"/cache_control", SectionMessages, 1, math.MaxInt}}},
		{"bedrock invoke anthropic", FormatBedrock, `{"anthropic_version":"bedrock-2023-05-31","messages":[{"role":"user","content":[{"type":"text","text":"a","cache_control":{"type":"ephemeral"}}]}]}`,
			[]want{{"/messages/0/content/0", SectionMessages, 0, 0}}},
		{"bedrock cachePoint", FormatBedrock, `{"system":[{"text":"s"},{"cachePoint":{"type":"default"}}],
			"messages":[{"role":"user","content":[{"text":"a"},{"cachePoint":{"type":"default"}},{"text":"b"}]}],
			"toolConfig":{"tools":[{"toolSpec":{"name":"f"}},{"cachePoint":{"type":"default"}}]}}`,
			[]want{{"/toolConfig/tools/1", SectionTools, -1, 1}, {"/system/1", SectionSystem, -1, 1}, {"/messages/0/content/1", SectionMessages, 0, 1}}},
		{"none", FormatAnthropic, `{"messages":[{"role":"user","content":"a"}]}`, nil},
		{"openai has none", FormatOpenAI, `{"messages":[{"role":"user","content":"a"}]}`, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := CacheBreakpoints(c.format, []byte(c.body))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %+v, want %d breakpoints", got, len(c.want))
			}
			for i, w := range c.want {
				g := got[i]
				if g.Path != w.path || g.Section != w.section || g.Message != w.msg || g.Block != w.k {
					t.Errorf("breakpoint %d = %+v, want %+v", i, g, w)
				}
			}
		})
	}
	if _, err := CacheBreakpoints("systemone", []byte(`{}`)); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("unsupported format: %v", err)
	}
}

func TestLastCacheBreakpoint(t *testing.T) {
	for _, c := range []struct {
		body string
		msg  int
		ok   bool
	}{
		{`{"messages":[{"role":"user","content":"a"}]}`, 0, false},
		{`{"tools":[{"name":"f","cache_control":{"type":"ephemeral"}}],"messages":[]}`, -2, true},
		{`{"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"messages":[]}`, -1, true},
		{`{"system":[{"type":"text","text":"s","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"a"},
			{"role":"assistant","content":[{"type":"text","text":"b","cache_control":{"type":"ephemeral"}}]}]}`, 1, true},
		{`not json`, 0, false},
	} {
		msg, ok := LastCacheBreakpoint(FormatAnthropic, []byte(c.body))
		if msg != c.msg || ok != c.ok {
			t.Errorf("LastCacheBreakpoint(%s) = %d, %v; want %d, %v", c.body, msg, ok, c.msg, c.ok)
		}
	}
}

// Unit.Cached: units wholly before the last breakpoint are cached; a unit
// with a leaf after it, or in a later message, is not.
func TestUnitCached(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"sys"}],"messages":[
		{"role":"user","content":[{"type":"text","text":"a"},{"type":"text","text":"b","cache_control":{"type":"ephemeral"}},{"type":"text","text":"c"}]},
		{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"f","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"out"}]}]}`)
	units, err := ExtractUnits(FormatAnthropic, body)
	if err != nil {
		t.Fatal(err)
	}
	bps, err := CacheBreakpoints(FormatAnthropic, body)
	if err != nil || len(bps) != 1 {
		t.Fatalf("breakpoints %+v, %v", bps, err)
	}
	got := map[string]bool{}
	for _, u := range units {
		got[u.Text] = u.Cached(bps[0])
	}
	// The first user message spans the breakpoint (block c is after it).
	want := map[string]bool{"sys": true, "out": false}
	for text, cached := range want {
		if got[text] != cached {
			t.Errorf("unit %q cached = %v, want %v", text, got[text], cached)
		}
	}
	for text, cached := range got {
		if strings.HasPrefix(text, "a") && cached {
			t.Errorf("unit %q spans the breakpoint but is cached", text)
		}
	}

	// A breakpoint on the last block of that message covers it.
	body2 := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"a"},{"type":"text","text":"b","cache_control":{"type":"ephemeral"}}]},
		{"role":"assistant","content":"later"}]}`)
	units, _ = ExtractUnits(FormatAnthropic, body2)
	bps, _ = CacheBreakpoints(FormatAnthropic, body2)
	if !units[0].Cached(bps[0]) || units[1].Cached(bps[0]) {
		t.Errorf("cached = %v, %v; want true, false", units[0].Cached(bps[0]), units[1].Cached(bps[0]))
	}

	// Bedrock: a cachePoint after the first message's text.
	body3 := []byte(`{"messages":[{"role":"user","content":[{"text":"a"},{"cachePoint":{"type":"default"}}]},
		{"role":"assistant","content":[{"text":"b"}]}]}`)
	units, _ = ExtractUnits(FormatBedrock, body3)
	bps, _ = CacheBreakpoints(FormatBedrock, body3)
	var a, b bool
	for _, u := range units {
		switch u.Text {
		case "a":
			a = u.Cached(bps[0])
		case "b":
			b = u.Cached(bps[0])
		}
	}
	if !a || b {
		t.Errorf("bedrock cached a=%v b=%v; want true, false", a, b)
	}
}

func TestNativeContextManagement(t *testing.T) {
	for _, c := range []struct {
		format, body string
		want         bool
	}{
		{FormatAnthropic, `{"messages":[{"role":"user","content":"a"}]}`, false},
		{FormatAnthropic, `{"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]},"messages":[]}`, true},
		{FormatAnthropic, `{"context_management":null,"messages":[]}`, false},
		{FormatAnthropic, `{"messages":[{"role":"assistant","content":[{"type":"compaction","content":"summary"}]}]}`, true},
		{FormatBedrock, `{"anthropic_version":"bedrock-2023-05-31","context_management":{},"messages":[]}`, true},
		{FormatBedrock, `{"messages":[],"additionalModelRequestFields":{"context_management":{"edits":[]}}}`, true},
		{FormatBedrock, `{"messages":[]}`, false},
		{FormatOpenAIResponses, `{"input":[{"type":"compaction","encrypted_content":"x"},{"role":"user","content":"hi"}]}`, true},
		{FormatOpenAIResponses, `{"context_management":[{"type":"compaction","compact_threshold":1000}],"input":"hi"}`, true},
		{FormatOpenAIResponses, `{"input":"hi"}`, false},
		{FormatOpenAI, `{"context_management":{},"messages":[]}`, false},
		{FormatAnthropic, `not json`, false},
	} {
		if got := NativeContextManagement(c.format, []byte(c.body)); got != c.want {
			t.Errorf("NativeContextManagement(%s, %s) = %v, want %v", c.format, c.body, got, c.want)
		}
	}
}

// cache_control on a block type this package does not know used to join the
// unit's text ("ephemeral", "1h"). It is metadata, not text.
func TestUnknownBlockCacheControlIsNotText(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"},
		{"type":"future_block","note":"visible","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`)
	units, err := ExtractUnits(FormatAnthropic, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("units: %+v", units)
	}
	text := units[0].Text
	if !strings.Contains(text, "hello") || !strings.Contains(text, "visible") {
		t.Errorf("unit text %q lost the block's text", text)
	}
	if strings.Contains(text, "ephemeral") || strings.Contains(text, "1h") {
		t.Errorf("unit text %q includes cache_control", text)
	}
}

// A Gemini thoughtSignature beside a functionCall or text part is opaque: it
// never joins a unit's text, with or without tool call units.
func TestGoogleThoughtSignatureIsNotText(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"list files"}]},
		{"role":"model","parts":[{"functionCall":{"name":"ls","args":{"dir":"."}},"thoughtSignature":"SIGNATURE-AAA"},
			{"text":"ok","thought_signature":"SIGNATURE-BBB"}]},
		{"role":"user","parts":[{"functionResponse":{"name":"ls","response":{"out":"a.go"}}}]}]}`)
	for _, opts := range []Options{{}, {ToolCalls: true}} {
		units, err := ExtractUnitsWith(FormatGoogleAI, body, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range units {
			if strings.Contains(u.Text, "SIGNATURE") {
				t.Errorf("unit %d (%s) text %q holds a thought signature", u.Index, u.Kind, u.Text)
			}
		}
	}
	if err := CheckToolPairing(FormatGoogleAI, body); err != nil {
		t.Errorf("CheckToolPairing: %v", err)
	}
}

// Tool results the vendor produced (and the client replays) are marked
// ServerTool, with the type that carried them; the client's own tool
// results are not.
func TestServerToolResults(t *testing.T) {
	for _, tc := range []struct {
		name, format, body string
		server             map[string]string // tool call id -> ServerToolType, "" for the client's
	}{
		{"anthropic", FormatAnthropic, `{"messages":[
			{"role":"assistant","content":[
				{"type":"server_tool_use","id":"s1","name":"web_search","input":{}},
				{"type":"web_search_tool_result","tool_use_id":"s1","content":[{"type":"web_search_result","title":"T","url":"u","encrypted_content":"E"}]},
				{"type":"server_tool_use","id":"s2","name":"web_fetch","input":{}},
				{"type":"web_fetch_tool_result","tool_use_id":"s2","content":{"type":"web_fetch_result","url":"u","content":{"type":"document","source":{"type":"text","media_type":"text/plain","data":"page"}}}},
				{"type":"server_tool_use","id":"s3","name":"code_execution","input":{}},
				{"type":"code_execution_tool_result","tool_use_id":"s3","content":{"type":"code_execution_result","stdout":"out"}},
				{"type":"mcp_tool_use","id":"m1","name":"x","server_name":"s","input":{}},
				{"type":"mcp_tool_result","tool_use_id":"m1","content":[{"type":"text","text":"mcp"}]},
				{"type":"tool_use","id":"c1","name":"f","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":"client"}]}]}`,
			map[string]string{"s1": "web_search_tool_result", "s2": "web_fetch_tool_result", "s3": "code_execution_tool_result", "m1": "mcp_tool_result", "c1": ""}},
		{"responses", FormatOpenAIResponses, `{"input":[
			{"type":"file_search_call","id":"fs","queries":["q"],"results":[{"text":"r"}]},
			{"type":"code_interpreter_call","id":"ci","code":"x","outputs":[{"type":"logs","logs":"l"}]},
			{"type":"mcp_call","id":"mc","name":"n","arguments":"{}","output":"o"},
			{"type":"function_call","name":"f","call_id":"c1","arguments":"{}"},
			{"type":"function_call_output","call_id":"c1","output":"client"}]}`,
			map[string]string{"fs": "file_search_call", "ci": "code_interpreter_call", "mc": "mcp_call", "c1": ""}},
		{"google", FormatGoogleAI, `{"contents":[{"role":"model","parts":[{"codeExecutionResult":{"outcome":"OUTCOME_OK","output":"o"}}]},
			{"role":"user","parts":[{"functionResponse":{"name":"f","response":{"r":"client"}}}]}]}`,
			map[string]string{"": "codeExecutionResult", "f": ""}},
		{"bedrock and openai results are the client's", FormatBedrock, `{"messages":[{"role":"user","content":[{"toolResult":{"toolUseId":"b1","content":[{"text":"client"}]}}]}]}`,
			map[string]string{"b1": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			units, err := ExtractUnits(tc.format, []byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, u := range units {
				if u.Kind != guardrails.KindToolResult {
					if u.ServerTool {
						t.Errorf("a %s unit is marked ServerTool", u.Kind)
					}
					continue
				}
				key := u.ToolCallID
				if tc.format == FormatGoogleAI {
					key = map[string]string{"code_execution": "", "f": "f"}[u.ToolName]
				}
				want, ok := tc.server[key]
				if !ok {
					continue
				}
				seen[key] = true
				if u.ServerTool != (want != "") || u.ServerToolType != want {
					t.Errorf("result %q (%s): ServerTool = %v (%q), want %q", key, u.Text, u.ServerTool, u.ServerToolType, want)
				}
			}
			for key := range tc.server {
				if !seen[key] {
					t.Errorf("no tool result unit for %q", key)
				}
			}
		})
	}
}
