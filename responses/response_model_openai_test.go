package responses

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openAIUsageWithCache is the usage block from the report on #678: prompt_tokens
// (6020) already contains the 6004 cached tokens and the 14 cache writes, and
// the vendor's own total_tokens (6026) confirms it.
const openAIUsageWithCache = `{
	"prompt_tokens": 6020,
	"completion_tokens": 6,
	"total_tokens": 6026,
	"completion_tokens_details": {"reasoning_tokens": 0},
	"prompt_tokens_details": {"audio_tokens": 0, "cache_write_tokens": 14, "cached_tokens": 6004}
}`

func TestOpenAIResponseCacheTokens(t *testing.T) {
	var resp OpenAIResponse

	require.NoError(t, json.Unmarshal(
		[]byte(`{"model":"openai.gpt-5.6-luna","usage":`+openAIUsageWithCache+`}`), &resp))

	assert.Equal(t, 6020, resp.GetPromptTokens())
	assert.Equal(t, 6004, resp.GetCacheReadPromptTokens())
	assert.Equal(t, 14, resp.GetCacheWritePromptTokens())
}

func TestOpenAIStreamingResponseCacheTokens(t *testing.T) {
	var resp OpenAIStreamingResponse

	require.NoError(t, json.Unmarshal(
		[]byte(`{"model":"openai.gpt-5.6-luna","usage":`+openAIUsageWithCache+`}`), &resp))

	assert.Equal(t, 6020, resp.GetPromptTokens())
	assert.Equal(t, 6004, resp.GetCacheReadPromptTokens())
	assert.Equal(t, 14, resp.GetCacheWritePromptTokens())
}

// A stream that never asks for usage (no include_usage) has no usage block at
// all; the cache accessors must report zero rather than dereferencing nil.
func TestOpenAIStreamingResponseCacheTokensWithoutUsage(t *testing.T) {
	var resp OpenAIStreamingResponse

	require.NoError(t, json.Unmarshal(
		[]byte(`{"model":"openai.gpt-5.6-luna","choices":[{"index":0,"delta":{}}]}`), &resp))

	assert.Nil(t, resp.Usage)
	assert.Equal(t, 0, resp.GetCacheReadPromptTokens())
	assert.Equal(t, 0, resp.GetCacheWritePromptTokens())
}

// Responses from backends that report no prompt_tokens_details must still
// report zero cache tokens rather than failing to parse.
func TestOpenAIResponseWithoutPromptTokensDetails(t *testing.T) {
	var resp OpenAIResponse

	require.NoError(t, json.Unmarshal(
		[]byte(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`), &resp))

	assert.Equal(t, 10, resp.GetPromptTokens())
	assert.Equal(t, 0, resp.GetCacheReadPromptTokens())
	assert.Equal(t, 0, resp.GetCacheWritePromptTokens())
}
