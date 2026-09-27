package anthropicVendor

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestAnalyzeStreamingResponse(t *testing.T) {
	v := &Anthropic{}

	// Simulate a streaming response with message_start containing cache tokens
	streamResponse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01WJ8k1rmgryTgbA2JWKWQfb","type":"message","role":"assistant","model":"claude-3-sonnet-20240229","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":20,"output_tokens":1,"cache_creation_input_tokens":5,"cache_read_input_tokens":15}}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":null,"stop_sequence":null},"usage":{"output_tokens":10}}
`

	// Create a mock request
	req, _ := http.NewRequest("POST", "/v1/messages", nil)

	// Test the streaming response analysis
	llm := &models.LLM{}
	app := &models.App{}
	llmResult, appResult, tokenResp, err := v.AnalyzeStreamingResponse(llm, app, 200, []byte(streamResponse), req, nil)
	assert.NoError(t, err)
	assert.Equal(t, llm, llmResult)
	assert.Equal(t, app, appResult)

	// Verify that cache tokens are properly tracked
	assert.Equal(t, 20, tokenResp.GetPromptTokens())
	// message_delta's output_tokens is cumulative (it already includes the
	// token message_start reported), so the start value must not be added.
	assert.Equal(t, 10, tokenResp.GetResponseTokens())
	assert.Equal(t, 5, tokenResp.GetCacheWritePromptTokens())
	assert.Equal(t, 15, tokenResp.GetCacheReadPromptTokens())
	assert.Equal(t, "claude-3-sonnet-20240229", tokenResp.GetModel())
}

func TestAnalyzeStreamingResponse_OutputTokens(t *testing.T) {
	v := &Anthropic{}
	req, _ := http.NewRequest("POST", "/v1/messages", nil)
	start := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":8,"output_tokens":3}}}
`

	tests := []struct {
		name   string
		stream string
		want   int
	}{
		{
			name: "several cumulative deltas: the last one is the total",
			stream: start + `
event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":null,"stop_sequence":null},"usage":{"output_tokens":7}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":12}}
`,
			want: 12,
		},
		{
			name:   "stream cut before any delta keeps message_start's count",
			stream: start,
			want:   3,
		},
		{
			name: "empty turn at max_tokens",
			stream: start + `
event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":1}}
`,
			want: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, tokenResp, err := v.AnalyzeStreamingResponse(&models.LLM{}, &models.App{}, 200, []byte(tc.stream), req, nil)
			assert.NoError(t, err)
			assert.Equal(t, 8, tokenResp.GetPromptTokens())
			assert.Equal(t, tc.want, tokenResp.GetResponseTokens())
		})
	}
}

func TestAnalyzeResponse(t *testing.T) {
	v := &Anthropic{}

	// Simulate a REST response with cache tokens
	restResponse := `{
		"id": "msg_01WJ8k1rmgryTgbA2JWKWQfb",
		"type": "message",
		"role": "assistant",
		"model": "claude-3-sonnet-20240229",
		"content": [],
		"stop_reason": null,
		"stop_sequence": null,
		"usage": {
			"input_tokens": 4,
			"output_tokens": 356,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens": 17476
		}
	}`

	// Create a mock request
	req, _ := http.NewRequest("POST", "/v1/messages", nil)

	// Test the REST response analysis
	llm := &models.LLM{}
	app := &models.App{}
	llmResult, appResult, tokenResp, err := v.AnalyzeResponse(llm, app, 200, []byte(restResponse), req)
	assert.NoError(t, err)
	assert.Equal(t, llm, llmResult)
	assert.Equal(t, app, appResult)

	// Verify that cache tokens are properly tracked
	assert.Equal(t, 4, tokenResp.GetPromptTokens())
	assert.Equal(t, 356, tokenResp.GetResponseTokens())
	assert.Equal(t, 0, tokenResp.GetCacheWritePromptTokens())
	assert.Equal(t, 17476, tokenResp.GetCacheReadPromptTokens())
	assert.Equal(t, "claude-3-sonnet-20240229", tokenResp.GetModel())
}
