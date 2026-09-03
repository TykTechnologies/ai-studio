package vendorconformance

import (
	"strings"
	"testing"

	bedrockvendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests carry no build tag and need no credentials: they protect the
// harness itself. A conformance suite whose own assertions are broken is worse
// than no suite, because it reports green.

const validCompletion = `{
  "id": "chatcmpl-1",
  "object": "chat.completion",
  "created": 1700000000,
  "model": "test-model",
  "choices": [{
    "index": 0,
    "finish_reason": "stop",
    "message": {"role": "assistant", "content": "pong"}
  }],
  "usage": {"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}
}`

func TestValidateAcceptsWellFormedCompletion(t *testing.T) {
	res, err := Validate(FormatOpenAICompletion, []byte(validCompletion))
	require.NoError(t, err)
	assert.True(t, res.OK(), res.Error())
	assert.Empty(t, res.Drift)
}

// The regression this whole suite exists for: tool call arguments must be a
// JSON-encoded string. Emitting a decoded object is valid JSON, passes a naive
// "did it parse" check, and breaks every OpenAI SDK.
func TestValidateRejectsToolArgumentsAsObject(t *testing.T) {
	payload := `{
      "id": "chatcmpl-1", "object": "chat.completion", "created": 1700000000, "model": "m",
      "choices": [{"index": 0, "finish_reason": "tool_calls", "message": {
        "role": "assistant",
        "tool_calls": [{"id": "call_1", "type": "function",
          "function": {"name": "get_weather", "arguments": {"city": "Wellington"}}}]
      }}]
    }`

	res, err := Validate(FormatOpenAICompletion, []byte(payload))
	require.NoError(t, err)
	require.False(t, res.OK(), "an object-valued arguments field must not validate")
	assert.Contains(t, res.Error(), "arguments")
}

func TestValidateRejectsMissingRequiredFields(t *testing.T) {
	cases := map[string]string{
		"no id":            `{"object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"x"}}]}`,
		"wrong object":     `{"id":"1","object":"text_completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"x"}}]}`,
		"empty choices":    `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[]}`,
		"bad finish":       `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"finished","message":{"role":"assistant","content":"x"}}]}`,
		"null finish":      `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":null,"message":{"role":"assistant","content":"x"}}]}`,
		"negative tokens":  `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"x"}}],"usage":{"prompt_tokens":-1,"completion_tokens":1,"total_tokens":0}}`,
		"partial usage":    `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"x"}}],"usage":{"prompt_tokens":1}}`,
		"user role reply":  `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"user","content":"x"}}]}`,
		"string created":   `{"id":"1","object":"chat.completion","created":"1","model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"x"}}]}`,
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := Validate(FormatOpenAICompletion, []byte(payload))
			require.NoError(t, err)
			assert.False(t, res.OK(), "expected a violation, got a clean result")
		})
	}
}

func TestValidateDetectsDrift(t *testing.T) {
	payload := `{
      "id": "1", "object": "chat.completion", "created": 1, "model": "m",
      "brand_new_field": "surprise",
      "choices": [{"index": 0, "finish_reason": "stop",
        "message": {"role": "assistant", "content": "x", "vendor_extra": 42}}]
    }`

	res, err := Validate(FormatOpenAICompletion, []byte(payload))
	require.NoError(t, err)
	assert.True(t, res.OK(), "drift alone is not a schema violation: %s", res.Error())

	pointers := map[string]string{}
	for _, d := range res.Drift {
		pointers[d.Pointer] = d.Kind
	}
	assert.Equal(t, "string", pointers["brand_new_field"])
	assert.Equal(t, "number", pointers["choices.0.message.vendor_extra"])
}

// Tool arguments are user data, not contract. The drift walker must not descend
// into a schema-less object and report every key a user happened to pass.
func TestDriftIgnoresOpaqueObjects(t *testing.T) {
	payload := `{
      "id": "1", "type": "message", "role": "assistant", "model": "m",
      "content": [{"type": "tool_use", "id": "t1", "name": "get_weather",
                   "input": {"city": "Wellington", "units": "metric", "anything": {"nested": true}}}]
    }`

	res, err := Validate(FormatAnthropicMessage, []byte(payload))
	require.NoError(t, err)
	assert.True(t, res.OK(), res.Error())
	for _, d := range res.Drift {
		assert.NotContains(t, d.Pointer, "input.", "must not descend into a free-form tool input object")
	}
}

func TestValidateAnthropicRejectsStringContent(t *testing.T) {
	// Flattening the typed content block array to a bare string is a real and
	// tempting translation shortcut. It must not validate.
	payload := `{"id":"1","type":"message","role":"assistant","model":"m","content":"hello"}`
	res, err := Validate(FormatAnthropicMessage, []byte(payload))
	require.NoError(t, err)
	assert.False(t, res.OK())
}

func TestValidateChunkRequiresToolCallIndex(t *testing.T) {
	// Without index, a client cannot reassemble interleaved tool-call fragments.
	payload := `{"id":"1","object":"chat.completion.chunk","created":1,"model":"m",
      "choices":[{"index":0,"delta":{"tool_calls":[{"id":"c1","function":{"name":"f","arguments":"{"}}]}}]}`
	res, err := Validate(FormatOpenAIChunk, []byte(payload))
	require.NoError(t, err)
	assert.False(t, res.OK())
	assert.Contains(t, res.Error(), "index")
}

func TestUsageConsistency(t *testing.T) {
	assert.True(t, Usage{10, 2, 12}.Consistent())
	assert.False(t, Usage{10, 2, 99}.Consistent())
}

// regionFromBedrockEndpoint is duplicated from the vendor package to keep this
// package dependency-light. This pins the two together so the copy cannot drift.
func TestRegionFromBedrockEndpointMatchesVendor(t *testing.T) {
	endpoints := []string{
		"https://bedrock-runtime.us-east-1.amazonaws.com",
		"https://bedrock-mantle.ap-southeast-2.api.aws",
		"https://bedrock-mantle.ap-southeast-2.api.aws/v1",
		"us-east-1",
		"ap-southeast-2",
	}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			want, wantErr := bedrockvendor.ParseRegionFromEndpoint(ep)
			got, gotErr := regionFromBedrockEndpoint(ep)
			assert.Equal(t, wantErr == nil, gotErr == nil, "error agreement")
			if wantErr == nil {
				assert.Equal(t, want, got)
			}
		})
	}
}

func TestRegionFromBedrockEndpointRejectsUnknownHost(t *testing.T) {
	_, err := regionFromBedrockEndpoint("https://example.com/bedrock")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "cannot extract region")
}
