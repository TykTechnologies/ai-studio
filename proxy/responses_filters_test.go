package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
)

func responsesView(t *testing.T, body string) [][2]string {
	t.Helper()
	msgs, err := (&OpenAIResponsesMessageExtractor{}).ExtractMessages(httptest.NewRequest("POST", "/v1/responses", nil), []byte(body))
	require.NoError(t, err)
	out := make([][2]string, len(msgs))
	for i, m := range msgs {
		out[i] = [2]string{string(m.Role), m.Parts[0].(llms.TextContent).Text}
	}
	return out
}

func TestResponsesExtractor(t *testing.T) {
	assert.Equal(t, requestFilterFormatOpenAIResponses, openaiVendor.ResponsesFilterFormat, "the vendor names the proxy's format")

	assert.Equal(t, [][2]string{{"system", "be brief"}, {"human", "hello"}},
		responsesView(t, `{"model":"m","instructions":"be brief","input":"hello"}`))

	body := `{"model":"m","prompt":{"id":"pmpt_1","variables":{"topic":"cats","extra":{"type":"input_text","text":"more"}}},
	"input":[
	  {"role":"developer","content":"dev rules"},
	  {"type":"message","role":"user","content":[{"type":"input_text","text":"part one"},{"type":"input_image","image_url":"https://x/y.png"},{"type":"input_text","text":"part two"}]},
	  {"type":"message","role":"assistant","content":[{"type":"output_text","text":"earlier answer"},{"type":"refusal","refusal":"no"}]},
	  {"type":"function_call","call_id":"c1","name":"lookup","arguments":"{\"q\":\"x\"}"},
	  {"type":"function_call_output","call_id":"c1","output":"tool says hi"},
	  {"type":"custom_tool_call_output","call_id":"c2","output":[{"type":"input_text","text":"custom out"}]},
	  {"type":"reasoning","id":"rs_1","summary":[]},
	  {"type":"item_reference","id":"msg_1"}
	]}`
	assert.Equal(t, [][2]string{
		{"human", "more"}, {"human", "cats"}, // variables in name order
		{"system", "dev rules"},
		{"human", "part one\npart two"},
		{"ai", "earlier answer\nno"},
		{"ai", `{"q":"x"}`},
		{"tool", "tool says hi"},
		{"tool", "custom out"},
	}, responsesView(t, body))

	assert.Empty(t, responsesView(t, `{"model":"m","previous_response_id":"resp_1"}`))

	for name, b := range map[string]string{
		"not json":            `nope`,
		"input number":        `{"input":5}`,
		"item not object":     `{"input":["hi"]}`,
		"content number":      `{"input":[{"role":"user","content":5}]}`,
		"part not object":     `{"input":[{"role":"user","content":["hi"]}]}`,
		"text not string":     `{"input":[{"role":"user","content":[{"type":"input_text","text":5}]}]}`,
		"unknown role":        `{"input":[{"role":"tool","content":"x"}]}`,
		"instructions object": `{"instructions":{"x":1},"input":"hi"}`,
		"variables array":     `{"prompt":{"variables":["x"]}}`,
		"arguments object":    `{"input":[{"type":"function_call","arguments":{}}]}`,
	} {
		_, err := (&OpenAIResponsesMessageExtractor{}).ExtractMessages(nil, []byte(b))
		assert.Error(t, err, name)
	}

	reg := NewMessageExtractorRegistry()
	reg.Register(&OpenAIResponsesMessageExtractor{})
	reg.Register(&OpenAIMessageExtractor{})
	assert.True(t, reg.FailsClosed(requestFilterFormatOpenAIResponses))
	assert.False(t, reg.FailsClosed("openai"), "chat completions keep their lenient extraction")
	assert.False(t, reg.FailsClosed("nope"))
}

func TestResponsesReconstructor(t *testing.T) {
	body := `{"model":"m","instructions":"rules","store":true,"max_output_tokens":1024,
	"input":[
	  {"type":"message","role":"user","content":[{"type":"input_text","text":"my SECRET"},{"type":"input_image","image_url":"u"}]},
	  {"type":"function_call_output","call_id":"c1","output":"tool SECRET"},
	  {"type":"message","role":"assistant","content":"kept"}
	]}`
	msgs := []map[string]interface{}{
		{"role": "system", "content": "rules"},
		{"role": "user", "content": "my [REDACTED]"},
		{"role": "tool", "content": "tool [REDACTED]"},
		{"role": "assistant", "content": "kept"},
	}
	out, err := (&OpenAIResponsesMessageReconstructor{}).Reconstruct(msgs, []byte(body))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "rules", got["instructions"])
	assert.Equal(t, true, got["store"])
	assert.EqualValues(t, 1024, got["max_output_tokens"])
	in := got["input"].([]any)
	assert.Equal(t, []any{
		map[string]any{"type": "input_text", "text": "my [REDACTED]"},
		map[string]any{"type": "input_image", "image_url": "u"},
	}, in[0].(map[string]any)["content"], "the image part is kept after the rewritten text")
	assert.Equal(t, "tool [REDACTED]", in[1].(map[string]any)["output"])
	assert.Equal(t, "c1", in[1].(map[string]any)["call_id"])
	assert.Equal(t, "kept", in[2].(map[string]any)["content"])

	// A string input stays a string.
	out, err = (&OpenAIResponsesMessageReconstructor{}).Reconstruct(
		[]map[string]interface{}{{"role": "user", "content": "x [REDACTED]"}}, []byte(`{"input":"x SECRET"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"input":"x [REDACTED]"}`, string(out))

	// Output that adds a message cannot be mapped onto the request.
	_, err = (&OpenAIResponsesMessageReconstructor{}).Reconstruct(append(msgs, map[string]interface{}{"role": "system", "content": "extra"}), []byte(body))
	assert.Error(t, err)
}
