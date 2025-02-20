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
	assert.Equal(t, 11, tokenResp.GetResponseTokens()) // 1 from start + 10 from delta
	assert.Equal(t, 5, tokenResp.GetCacheWritePromptTokens())
	assert.Equal(t, 15, tokenResp.GetCacheReadPromptTokens())
	assert.Equal(t, "claude-3-sonnet-20240229", tokenResp.GetModel())
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
