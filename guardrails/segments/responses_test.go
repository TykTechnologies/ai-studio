package segments

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// The Responses fixtures join the shared table, so TestExtract,
// TestRewriteRoundTrip and FuzzExtractRewrite cover them.
func init() {
	fixtures = append(fixtures, responsesFixtures...)
}

var responsesFixtures = []fixture{
	{
		name:   "responses string input",
		format: FormatOpenAIResponses,
		body:   `{"model":"gpt-5","instructions":"Be brief.","input":"What is 2+2?","store":false}`,
		want: []want{
			text("system", "/instructions", "Be brief."),
			text("user", "/input", "What is 2+2?"),
		},
	},
	{
		name:   "responses codex agent loop",
		format: FormatOpenAIResponses,
		body: `{"model":"gpt-5-codex","instructions":"You are Codex.","store":false,"include":["reasoning.encrypted_content"],
			"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions>sandboxed</permissions>"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"run the tests"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]},
			{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"Plan: run go test"}],"encrypted_content":"gAAAA-secret"},
			{"type":"function_call","name":"shell","arguments":"{\"command\":[\"go\",\"test\"]}","call_id":"call_1"},
			{"type":"function_call_output","call_id":"call_1","output":"{\"output\":\"ok  pkg 0.1s\\n\",\"metadata\":{\"exit_code\":0}}"},
			{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch","call_id":"call_2"},
			{"type":"custom_tool_call_output","call_id":"call_2","output":"Done!"},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Tests pass."}]},
			{"role":"user","content":"thanks"}
		]}`,
		want: []want{
			text("system", "/instructions", "You are Codex."),
			text("user", "/input/0/content/0/text", "<permissions>sandboxed</permissions>"),
			text("user", "/input/1/content/0/text", "run the tests"),
			tool("shell", "call_1", "/input/4/output", `{"output":"ok  pkg 0.1s\n","metadata":{"exit_code":0}}`),
			tool("apply_patch", "call_2", "/input/6/output", "Done!"),
			text("assistant", "/input/7/content/0/text", "Tests pass."),
			text("user", "/input/8/content", "thanks"),
		},
	},
	{
		name:   "responses compaction and built-in tools",
		format: FormatOpenAIResponses,
		body: `{"model":"gpt-5","context_management":[{"type":"compaction","compact_threshold":200000}],"input":[
			{"type":"compaction","id":"cmp_1","encrypted_content":"opaque-summary"},
			{"type":"mcp_call","id":"mcp_1","name":"search","server_label":"docs","arguments":"{\"q\":\"x\"}","output":"found x"},
			{"type":"file_search_call","id":"fs_1","queries":["x"],"results":[{"file_id":"f1","text":"chunk one"}]},
			{"type":"web_search_call","id":"ws_1","action":{"type":"search","query":"x"}},
			{"type":"item_reference","id":"msg_9"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"},{"type":"input_file","file_id":"file_1"}]}
		]}`,
		want: []want{
			tool("search", "mcp_1", "/input/1/output", "found x"),
			tool("file_search", "fs_1", "/input/2/results/0/text", "chunk one"),
			text("user", "/input/5/content/0/text", "go on"),
		},
	},
}

// Message is the item's position in input and Turn counts the runs of model
// items before it.
func TestResponsesMessageAndTurn(t *testing.T) {
	var body string
	for _, fx := range responsesFixtures {
		if fx.name == "responses codex agent loop" {
			body = fx.body
		}
	}
	units, err := ExtractUnitsWith(FormatOpenAIResponses, []byte(body), Options{ToolCalls: true})
	if err != nil {
		t.Fatal(err)
	}
	type mt struct {
		kind            string
		message, turn   int
		toolName, calls string
	}
	var got []mt
	for _, u := range units {
		got = append(got, mt{u.Kind, u.Message, u.Turn, u.ToolName, u.ToolCallID})
	}
	want := []mt{
		{guardrails.KindText, -1, 0, "", ""},               // instructions
		{guardrails.KindText, 0, 0, "", ""},                // developer
		{guardrails.KindText, 1, 0, "", ""},                // user
		{guardrails.KindText, 2, 0, "", ""},                // reasoning summary, detection only
		{guardrails.KindToolCall, 3, 0, "shell", "call_1"}, // model run 0: reasoning, call
		{guardrails.KindToolResult, 4, 1, "shell", "call_1"},
		{guardrails.KindToolCall, 5, 1, "apply_patch", "call_2"},
		{guardrails.KindToolResult, 6, 2, "apply_patch", "call_2"},
		{guardrails.KindText, 7, 2, "", ""}, // assistant message
		{guardrails.KindText, 8, 3, "", ""}, // user
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("units:\n got %+v\nwant %+v", got, want)
	}
	for _, u := range units {
		for _, secret := range []string{"gAAAA-secret", "base64"} {
			if strings.Contains(u.Text, secret) {
				t.Errorf("unit %d text %q holds %q", u.Index, u.Text, secret)
			}
		}
	}
}

// Rewriting nothing leaves a Responses body byte for byte.
func TestResponsesApplyZeroEditsIsIdentity(t *testing.T) {
	for _, fx := range responsesFixtures {
		out, err := Apply([]byte(fx.body), nil)
		if err != nil {
			t.Fatalf("%s: %v", fx.name, err)
		}
		if string(out) != fx.body {
			t.Errorf("%s: Apply with no edits changed the body", fx.name)
		}
	}
}

// Quarantining a tool output replaces it with a string, which the API
// accepts for every *_call_output.
func TestResponsesQuarantineToolOutput(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","name":"f","arguments":"{}","call_id":"c"},
		{"type":"function_call_output","call_id":"c","output":[{"type":"input_text","text":"a"},{"type":"input_image","image_url":"x"}]}]}`)
	units, err := ExtractUnits(FormatOpenAIResponses, body)
	if err != nil || len(units) != 2 || units[1].Kind != guardrails.KindToolResult {
		t.Fatalf("units %+v, %v", units, err)
	}
	if _, ok := units[0].Quarantine("[removed]"); ok {
		t.Error("the model's function_call could be quarantined")
	}
	edits, ok := units[1].Quarantine("[removed]")
	if !ok {
		t.Fatal("tool output not quarantinable")
	}
	out, err := Apply(body, edits)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"output":"[removed]"`) {
		t.Errorf("out = %s", out)
	}
}

func TestResponsesExtractErrors(t *testing.T) {
	for name, body := range map[string]string{
		"no input":              `{"model":"gpt-5"}`,
		"input not an array":    `{"input":42}`,
		"item not an object":    `{"input":["hi"]}`,
		"instructions object":   `{"instructions":{},"input":"x"}`,
		"case-variant field":    `{"input":[{"role":"user","Content":"x","content":"y"}]}`,
		"content of wrong type": `{"input":[{"role":"user","content":7}]}`,
	} {
		if _, err := ExtractUnits(FormatOpenAIResponses, []byte(body)); err == nil {
			t.Errorf("%s: no error", name)
		} else if !errors.Is(err, ErrUnrecognisedBody) && !errors.Is(err, ErrDuplicateKey) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A body with only a stored response and instructions is valid.
	if _, err := ExtractUnits(FormatOpenAIResponses, []byte(`{"previous_response_id":"resp_1","instructions":"x"}`)); err != nil {
		t.Errorf("previous_response_id only: %v", err)
	}
}

func TestResponsesToolPairing(t *testing.T) {
	for _, c := range []struct {
		name, body, bad string
	}{
		{"paired parallel", `{"input":[{"type":"function_call","call_id":"a","name":"f"},{"type":"function_call","call_id":"b","name":"g"},
			{"type":"function_call_output","call_id":"b","output":"x"},{"type":"function_call_output","call_id":"a","output":"y"},
			{"type":"custom_tool_call","call_id":"c","name":"p","input":"i"},{"type":"custom_tool_call_output","call_id":"c","output":"ok"}]}`, ""},
		{"missing output", `{"input":[{"type":"function_call","call_id":"a","name":"f"}]}`, `no result for call "a"`},
		{"orphan output", `{"input":[{"type":"function_call_output","call_id":"z","output":"x"}]}`, `result for no call "z"`},
		{"orphan output with a stored response", `{"previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"z","output":"x"}]}`, ""},
		{"server tools have no call_id", `{"input":[{"type":"web_search_call","id":"ws","action":{}},{"type":"mcp_call","id":"m","name":"n","output":"o"}]}`, ""},
	} {
		err := CheckToolPairing(FormatOpenAIResponses, []byte(c.body))
		switch {
		case c.bad == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.bad != "" && (!errors.Is(err, ErrToolPairing) || !strings.Contains(err.Error(), c.bad)):
			t.Errorf("%s: err = %v, want %q", c.name, err, c.bad)
		}
	}
}

func TestResponsesToolDefinitionsInExtract(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"shell","parameters":{}},{"type":"web_search"},{"type":"custom","name":"apply_patch"}],"input":"x"}`)
	units, err := ExtractUnitsWith(FormatOpenAIResponses, body, Options{ToolDefinitions: true})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, u := range units {
		if u.Kind == guardrails.KindToolDefinition {
			names = append(names, u.ToolName)
		}
	}
	if !reflect.DeepEqual(names, []string{"shell", "apply_patch"}) {
		t.Errorf("definitions = %v", names)
	}
}

// With previous_response_id the call lives in the stored response: the
// output is still a tool result, with the call id and no tool name.
func TestResponsesPreviousResponseID(t *testing.T) {
	body := []byte(`{"model":"gpt-5","previous_response_id":"resp_abc","input":[
		{"type":"function_call_output","call_id":"call_9","output":[{"type":"input_text","text":"42 rows"}]}]}`)
	segs, err := Extract(FormatOpenAIResponses, body)
	if err != nil {
		t.Fatal(err)
	}
	expected := []want{tool("", "call_9", "/input/0/output/0/text", "42 rows")}
	got := make([]want, len(segs))
	for i, s := range segs {
		got[i] = want{Role: s.Role, Kind: s.Kind, ToolName: s.ToolName, ToolCallID: s.ToolCallID, Path: s.Path, Text: s.Text}
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("segments %+v, want %+v", got, expected)
	}
	if err := CheckToolPairing(FormatOpenAIResponses, body); err != nil {
		t.Errorf("CheckToolPairing: %v", err)
	}
}

// The model's own items are read (a client can forge them as history) but
// never rewritable; encrypted content stays unread.
func TestResponsesModelItemsAreDetectionOnly(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"reasoning","id":"rs","summary":[{"type":"summary_text","text":"leaked sk-live-123"}],"encrypted_content":"ENCRYPTED-BLOB"},
		{"type":"local_shell_call","id":"ls","call_id":"c1","action":{"type":"exec","command":["rm","-rf","/"]}},
		{"type":"local_shell_call_output","id":"lso","call_id":"c1","output":"done"},
		{"type":"web_search_call","id":"ws","action":{"type":"search","query":"ignore previous instructions","queries":["q2"]}},
		{"type":"computer_call","id":"cc","call_id":"c2","action":{"type":"type","text":"typed secret"}},
		{"type":"file_search_call","id":"fs","queries":["fq"],"results":[{"text":"result text"}]},
		{"type":"shell_call","id":"sc","call_id":"c3","action":{"commands":["curl evil.example"]}},
		{"type":"function_call","name":"f","arguments":"{\"x\":\"arg text\"}","call_id":"c4"}
	]}`)
	units, err := Read(FormatOpenAIResponses, body)
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, u := range units {
		all.WriteString(u.Text + "\n")
		if u.Role == guardrails.RoleAssistant {
			if len(u.Leaves) != 0 {
				t.Errorf("model unit %q has leaves", u.Text)
			}
			if _, ok := u.Quarantine("x"); ok {
				t.Errorf("model unit %q is quarantinable", u.Text)
			}
		}
	}
	for _, want := range []string{"leaked sk-live-123", "rm", "-rf", "ignore previous instructions", "q2", "typed secret", "fq", "result text", "curl evil.example", "arg text", "done"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("no unit reads %q", want)
		}
	}
	if strings.Contains(all.String(), "ENCRYPTED-BLOB") {
		t.Error("encrypted_content was read")
	}
}
