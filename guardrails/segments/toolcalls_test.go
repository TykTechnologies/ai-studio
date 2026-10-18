package segments

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// callShape is what a test checks of a tool call unit.
type callShape struct {
	Name, ID, Text, Goal string
}

func toolCalls(units []Unit) []callShape {
	var out []callShape
	for _, u := range units {
		if !u.IsToolCall() {
			continue
		}
		s := callShape{Name: u.ToolName, ID: u.ToolCallID, Text: u.Text}
		if u.Context != nil {
			s.Goal = u.Context.UserGoal
		}
		out = append(out, s)
	}
	return out
}

// Every format's tool calls become units with the tool's name, the call id
// and the arguments, when asked for; the goal is the latest user text
// before the call.
func TestExtractToolCalls(t *testing.T) {
	cases := []struct {
		name, format, body string
		want               []callShape
	}{
		{
			name:   "openai parallel calls and an empty argument string",
			format: FormatOpenAI,
			body: `{"messages":[{"role":"system","content":"be helpful"},{"role":"user","content":"mail the report"},
				{"role":"assistant","content":null,"tool_calls":[
					{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"r.txt\"}"}},
					{"id":"c2","type":"function","function":{"name":"list","arguments":""}}]},
				{"role":"tool","tool_call_id":"c1","content":"report"},
				{"role":"tool","tool_call_id":"c2","content":"a b"}]}`,
			want: []callShape{
				{"read_file", "c1", `{"path":"r.txt"}`, "mail the report"},
				{"list", "c2", "{}", "mail the report"},
			},
		},
		{
			name:   "openai legacy function_call",
			format: FormatOpenAI,
			body:   `{"messages":[{"role":"user","content":"weather?"},{"role":"assistant","function_call":{"name":"get_weather","arguments":"{\"city\":\"Oslo\"}"}}]}`,
			want:   []callShape{{"get_weather", "", `{"city":"Oslo"}`, "weather?"}},
		},
		{
			name:   "ollama arguments object",
			format: FormatOllama,
			body:   `{"messages":[{"role":"user","content":"go"},{"role":"assistant","content":"","tool_calls":[{"function":{"name":"run","arguments":{"cmd":"ls"}}}]}]}`,
			want:   []callShape{{"run", "", `{"cmd":"ls"}`, "go"}},
		},
		{
			name:   "anthropic tool_use, server_tool_use and mcp_tool_use",
			format: FormatAnthropic,
			body: `{"system":"sys","messages":[{"role":"user","content":"first"},
				{"role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"t1","name":"send","input":{"to":"a@b.c"}},
					{"type":"server_tool_use","id":"s1","name":"web_search","input":{"query":"x"}},
					{"type":"mcp_tool_use","id":"m1","name":"lookup","server_name":"crm","input":{}}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"sent"},{"type":"text","text":"now delete it"}]},
				{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"delete","input":{"id":7}}]}]}`,
			want: []callShape{
				{"send", "t1", `{"to":"a@b.c"}`, "first"},
				{"web_search", "s1", `{"query":"x"}`, "first"},
				{"lookup", "m1", `{}`, "first"},
				{"delete", "t2", `{"id":7}`, "now delete it"},
			},
		},
		{
			name:   "google functionCall in both spellings",
			format: FormatGoogleAI,
			body: `{"contents":[{"role":"user","parts":[{"text":"book it"}]},
				{"role":"model","parts":[{"functionCall":{"name":"book","args":{"seat":"1A"},"id":"g1"}},{"function_call":{"name":"pay","args":{}}}]}]}`,
			want: []callShape{{"book", "g1", `{"seat":"1A"}`, "book it"}, {"pay", "", `{}`, "book it"}},
		},
		{
			name:   "bedrock toolUse",
			format: FormatBedrock,
			body: `{"messages":[{"role":"user","content":[{"text":"look up"}]},
				{"role":"assistant","content":[{"toolUse":{"toolUseId":"b1","name":"search","input":{"q":"x"}}}]}]}`,
			want: []callShape{{"search", "b1", `{"q":"x"}`, "look up"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			units, err := ExtractUnitsWith(tc.format, []byte(tc.body), Options{ToolCalls: true})
			if err != nil {
				t.Fatal(err)
			}
			if got := toolCalls(units); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tool calls = %+v\nwant %+v", got, tc.want)
			}
			for i, u := range units {
				if u.Index != i {
					t.Errorf("unit %d has Index %d", i, u.Index)
				}
				if u.IsToolCall() && u.Role != guardrails.RoleAssistant {
					t.Errorf("tool call role = %q", u.Role)
				}
			}
			// Without the option the units are exactly those of ExtractUnits.
			plain, err := ExtractUnits(tc.format, []byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			for _, u := range plain {
				if u.IsToolCall() {
					t.Fatalf("tool call unit without the option: %+v", u.Segment)
				}
			}
			var withoutCalls []guardrails.Segment
			for _, u := range units {
				if !u.IsToolCall() {
					s := u.Segment
					s.Index = 0
					withoutCalls = append(withoutCalls, s)
				}
			}
			var plainSegs []guardrails.Segment
			for _, u := range plain {
				s := u.Segment
				s.Index = 0
				plainSegs = append(plainSegs, s)
			}
			if !reflect.DeepEqual(withoutCalls, plainSegs) {
				t.Fatalf("other units changed:\n%+v\n%+v", withoutCalls, plainSegs)
			}
		})
	}
}

// A tool call's arguments are never rewritten: the client has already run
// the call, and quarantine or redaction cannot apply to it.
func TestToolCallUnitsAreNotRewritable(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"x"},{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":"{\"a\":1}"}}]}]}`)
	units, err := ExtractUnitsWith(FormatOpenAI, body, Options{ToolCalls: true})
	if err != nil {
		t.Fatal(err)
	}
	var call Unit
	for _, u := range units {
		if u.IsToolCall() {
			call = u
		}
	}
	if _, ok := call.Quarantine("[removed]"); ok {
		t.Error("Quarantine accepted a tool call")
	}
	if _, ok := call.MapRewrite("{}"); ok {
		t.Error("MapRewrite accepted a tool call")
	}
}

// The goal context carries the system prompt and is clipped.
func TestToolCallGoalContext(t *testing.T) {
	long := strings.Repeat("a", guardrails.MaxContextChars) + "TAIL"
	body := []byte(`{"messages":[{"role":"system","content":"S1"},{"role":"system","content":"S2"},{"role":"user","content":"` + long + `"},
		{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":"{}"}}]}]}`)
	units, err := ExtractUnitsWith(FormatOpenAI, body, Options{ToolCalls: true})
	if err != nil {
		t.Fatal(err)
	}
	c := units[len(units)-1].Context
	if c == nil || c.System != "S1\n\nS2" {
		t.Fatalf("context = %+v", c)
	}
	if n := len([]rune(c.UserGoal)); n != guardrails.MaxContextChars || !strings.HasSuffix(c.UserGoal, "TAIL") || !strings.Contains(c.UserGoal, "[…]") {
		t.Fatalf("goal not clipped head+tail: %d runes", n)
	}
}

// A case variant of a field the extractor now reads is refused, as for
// every other field it reads.
func TestToolCallCaseVariantsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"anthropic input": `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"f","input":{},"Input":{"x":1}}]}]}`,
		"google args":     `{"contents":[{"role":"model","parts":[{"functionCall":{"name":"f","args":{},"Args":{"x":1}}}]}]}`,
	} {
		format := FormatAnthropic
		if strings.HasPrefix(name, "google") {
			format = FormatGoogleAI
		}
		if _, err := ExtractUnitsWith(format, []byte(body), Options{ToolCalls: true}); err == nil {
			t.Errorf("%s: accepted a case-variant key", name)
		}
	}
}

// Buffered responses: tool calls in every vendor's response shape.
func TestResponseToolCalls(t *testing.T) {
	cases := []struct {
		name, format, body string
		want               []callShape
	}{
		{"openai", FormatOpenAI, `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[
			{"id":"c1","type":"function","function":{"name":"send","arguments":"{\"to\":\"x@evil.example\"}"}},
			{"id":"c2","type":"function","function":{"name":"log","arguments":"{}"}}]}}]}`,
			[]callShape{{"send", "c1", `{"to":"x@evil.example"}`, ""}, {"log", "c2", "{}", ""}}},
		{"openai legacy", FormatOpenAI, `{"choices":[{"message":{"function_call":{"name":"f","arguments":"{\"a\":1}"}}}]}`,
			[]callShape{{"f", "", `{"a":1}`, ""}}},
		{"ollama", FormatOllama, `{"message":{"role":"assistant","tool_calls":[{"function":{"name":"run","arguments":{"cmd":"ls"}}}]}}`,
			[]callShape{{"run", "", `{"cmd":"ls"}`, ""}}},
		{"anthropic", FormatAnthropic, `{"content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"t1","name":"send","input":{"to":"a"}}]}`,
			[]callShape{{"send", "t1", `{"to":"a"}`, ""}}},
		{"bedrock converse", FormatBedrock, `{"output":{"message":{"role":"assistant","content":[{"toolUse":{"toolUseId":"b1","name":"q","input":{"x":1}}}]}}}`,
			[]callShape{{"q", "b1", `{"x":1}`, ""}}},
		{"bedrock invoke anthropic", FormatBedrock, `{"content":[{"type":"tool_use","id":"t1","name":"send","input":{}}]}`,
			[]callShape{{"send", "t1", `{}`, ""}}},
		{"google", FormatGoogleAI, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"book","args":{"s":1}}}]}}]}`,
			[]callShape{{"book", "", `{"s":1}`, ""}}},
		{"responses", FormatOpenAIResponses, `{"output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]},
			{"type":"function_call","call_id":"r1","name":"send","arguments":"{\"to\":\"x\"}"},
			{"type":"custom_tool_call","call_id":"r2","name":"shell","input":"rm -rf /"},
			{"type":"mcp_call","id":"m1","name":"crm_export","arguments":"{}"}]}`,
			[]callShape{{"send", "r1", `{"to":"x"}`, ""}, {"shell", "r2", "rm -rf /", ""}, {"crm_export", "m1", "{}", ""}}},
		{"responses line under the vendor name", FormatOpenAI, `{"output":[{"type":"function_call","call_id":"r1","name":"send","arguments":"{}"}]}`,
			[]callShape{{"send", "r1", "{}", ""}}},
		{"no calls", FormatOpenAI, `{"choices":[{"message":{"content":"hello"}}]}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			units, err := ResponseToolCalls(tc.format, []byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if got := toolCalls(units); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tool calls = %+v\nwant %+v", got, tc.want)
			}
		})
	}
	if _, err := ResponseToolCalls(FormatOpenAI, []byte("data: {}")); err == nil {
		t.Error("a non-JSON body was read")
	}
}

// The goal of a response's tool calls comes from the request.
func TestGoal(t *testing.T) {
	cases := []struct {
		format, body string
		want         guardrails.SegmentContext
	}{
		{FormatOpenAI, `{"messages":[{"role":"system","content":"S"},{"role":"user","content":"one"},{"role":"assistant","content":"x"},{"role":"user","content":"two"}]}`,
			guardrails.SegmentContext{UserGoal: "two", System: "S"}},
		{FormatAnthropic, `{"system":"S","messages":[{"role":"user","content":"goal"},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"r"}]}]}`,
			guardrails.SegmentContext{UserGoal: "goal", System: "S"}},
		{FormatOpenAIResponses, `{"instructions":"S","input":"just a string"}`,
			guardrails.SegmentContext{UserGoal: "just a string", System: "S"}},
		{FormatOpenAIResponses, `{"input":[{"role":"developer","content":"D"},{"role":"user","content":[{"type":"input_text","text":"a"},{"type":"input_text","text":"b"}]},{"type":"function_call_output","call_id":"x","output":"o"}]}`,
			guardrails.SegmentContext{UserGoal: "a\nb", System: "D"}},
		{FormatOpenAI, `not json`, guardrails.SegmentContext{}},
	}
	for i, tc := range cases {
		if got := Goal(tc.format, []byte(tc.body)); got != tc.want {
			t.Errorf("case %d: Goal = %+v, want %+v", i, got, tc.want)
		}
	}
}

// FuzzToolCalls: reading tool calls never panics, never changes the other
// units, and every tool call unit is a whole, unrewritable unit of the
// model's; a response body's tool calls are read without panicking either.
func FuzzToolCalls(f *testing.F) {
	for _, fx := range fixtures {
		for i, format := range fuzzFormats {
			if format == fx.format {
				f.Add(uint8(i), []byte(fx.body))
			}
		}
	}
	f.Add(uint8(0), []byte(`{"messages":[{"role":"assistant","tool_calls":[{"id":"c","function":{"name":"f","arguments":"{\"a\":\"\\u0045\"}"}}]}]}`))
	f.Add(uint8(2), []byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"n","input":{"k":["v",{"x":null}]}}]}]}`))
	f.Add(uint8(3), []byte(`{"contents":[{"role":"model","parts":[{"functionCall":{"name":"f","args":{"":""}}}]}]}`))
	f.Add(uint8(4), []byte(`{"messages":[{"role":"assistant","content":[{"toolUse":{"toolUseId":"b","name":"n","input":[1,2]}}]}]}`))

	f.Fuzz(func(t *testing.T, fi uint8, body []byte) {
		format := fuzzFormats[int(fi)%len(fuzzFormats)]
		_, _ = ResponseToolCalls(format, body)
		_ = Goal(format, body)
		_ = Goal(FormatOpenAIResponses, body)
		with, err := ExtractUnitsWith(format, body, Options{ToolCalls: true})
		plain, plainErr := ExtractUnits(format, body)
		if err != nil {
			// Reading tool calls adds case-variant checks of the fields it
			// reads; it never makes a refused body readable.
			if plainErr == nil && !errors.Is(err, ErrDuplicateKey) {
				t.Fatalf("ExtractUnits read the body, ExtractUnitsWith failed: %v", err)
			}
			return
		}
		if plainErr != nil {
			t.Fatalf("ExtractUnitsWith read a body ExtractUnits refused: %v", plainErr)
		}
		n := 0
		for i, u := range with {
			if u.Index != i {
				t.Fatalf("unit %d has Index %d", i, u.Index)
			}
			if !u.IsToolCall() {
				if n >= len(plain) || u.Text != plain[n].Text || u.Role != plain[n].Role || u.Kind != plain[n].Kind {
					t.Fatalf("unit %d differs from ExtractUnits' unit %d", i, n)
				}
				n++
				continue
			}
			if u.Role != guardrails.RoleAssistant || u.Text == "" || u.Context == nil {
				t.Fatalf("tool call unit %+v", u.Segment)
			}
			if _, ok := u.Quarantine("x"); ok {
				t.Fatal("a tool call could be quarantined")
			}
			if _, ok := u.MapRewrite(u.Text + "x"); ok {
				t.Fatal("a tool call could be rewritten")
			}
		}
		if n != len(plain) {
			t.Fatalf("ExtractUnitsWith has %d other units, ExtractUnits %d", n, len(plain))
		}
	})
}
