package proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every input item type the extractor knows has its text fields screened,
// in body order, with the role the model reads them in.
func TestResponsesExtractor_BuiltInItemText(t *testing.T) {
	body := `{"model":"m","input":[
	  {"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"thought summary"}],"content":[{"type":"reasoning_text","text":"raw thought"}],"encrypted_content":"opaque"},
	  {"type":"mcp_call","id":"mcp_1","server_label":"s","name":"t","arguments":"{\"a\":1}","output":"mcp result","error":null},
	  {"type":"mcp_list_tools","id":"ml_1","server_label":"s","tools":[{"name":"t","description":"tool doc","input_schema":{}}]},
	  {"type":"mcp_approval_request","id":"ar_1","server_label":"s","name":"t","arguments":"{\"b\":2}"},
	  {"type":"mcp_approval_response","approval_request_id":"ar_1","approve":false,"reason":"user reason"},
	  {"type":"code_interpreter_call","id":"ci_1","container_id":"c","code":"print(1)","outputs":[{"type":"logs","logs":"ci logs"},{"type":"image","url":"https://x/y.png"}]},
	  {"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"search terms"}},
	  {"type":"file_search_call","id":"fs_1","queries":["q1","q2"],"results":[{"file_id":"f","text":"file text"}]},
	  {"type":"computer_call","id":"cc_1","call_id":"c1","action":{"type":"type","text":"typed text"}},
	  {"type":"local_shell_call","id":"ls_1","call_id":"c2","action":{"type":"exec","command":["ls","-la"]}},
	  {"type":"image_generation_call","id":"ig_1","result":"base64png"},
	  {"type":"item_reference","id":"msg_1"}
	]}`
	assert.Equal(t, [][2]string{
		{"ai", "thought summary"}, {"ai", "raw thought"},
		{"ai", `{"a":1}`}, {"tool", "mcp result"},
		{"tool", "tool doc"},
		{"ai", `{"b":2}`},
		{"human", "user reason"},
		{"ai", "print(1)"}, {"tool", "ci logs"},
		{"ai", "search terms"},
		{"ai", "q1"}, {"ai", "q2"}, {"tool", "file text"},
		{"ai", "typed text"},
		{"ai", "ls"}, {"ai", "-la"},
	}, responsesView(t, body))
}

// An item type the extractor does not know, or a known field of the wrong
// shape, fails extraction (and so the request, when filters are attached).
func TestResponsesExtractor_UnknownItemsFailClosed(t *testing.T) {
	for name, b := range map[string]string{
		"unknown type":         `{"input":[{"type":"brand_new_item","text":"x"}]}`,
		"no type or role":      `{"input":[{"text":"x"}]}`,
		"mcp output object":    `{"input":[{"type":"mcp_call","output":{"x":1}}]}`,
		"logs not string":      `{"input":[{"type":"code_interpreter_call","outputs":[{"type":"logs","logs":5}]}]}`,
		"summary not array":    `{"input":[{"type":"reasoning","summary":"x"}]}`,
		"command not strings":  `{"input":[{"type":"local_shell_call","action":{"command":[1]}}]}`,
		"action not an object": `{"input":[{"type":"web_search_call","action":"x"}]}`,
	} {
		_, err := (&OpenAIResponsesMessageExtractor{}).ExtractMessages(nil, []byte(b))
		assert.Error(t, err, name)
	}
}

// Redactions in built-in item fields are written back in place.
func TestResponsesReconstructor_BuiltInItemText(t *testing.T) {
	body := `{"input":[
	  {"type":"mcp_call","id":"m","arguments":"{}","output":"mcp SECRET"},
	  {"type":"code_interpreter_call","id":"c","code":"x","outputs":[{"type":"logs","logs":"log SECRET"}]},
	  {"type":"reasoning","id":"r","summary":[{"type":"summary_text","text":"sum SECRET"}]},
	  {"type":"local_shell_call","id":"l","action":{"type":"exec","command":["echo","SECRET"]}}
	]}`
	view := responsesView(t, body)
	msgs := make([]map[string]interface{}, len(view))
	for i, v := range view {
		msgs[i] = map[string]interface{}{"role": v[0], "content": v[1]}
	}
	for _, m := range msgs {
		m["content"] = strings.ReplaceAll(m["content"].(string), "SECRET", "[REDACTED]")
	}
	out, err := (&OpenAIResponsesMessageReconstructor{}).Reconstruct(msgs, []byte(body))
	require.NoError(t, err)
	var got struct {
		Input []map[string]any `json:"input"`
	}
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "mcp [REDACTED]", got.Input[0]["output"])
	assert.Equal(t, "log [REDACTED]", got.Input[1]["outputs"].([]any)[0].(map[string]any)["logs"])
	assert.Equal(t, "sum [REDACTED]", got.Input[2]["summary"].([]any)[0].(map[string]any)["text"])
	assert.Equal(t, []any{"echo", "[REDACTED]"}, got.Input[3]["action"].(map[string]any)["command"])
	assert.Equal(t, "m", got.Input[0]["id"])
}
