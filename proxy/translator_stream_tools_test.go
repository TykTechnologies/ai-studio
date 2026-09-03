package proxy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCallDelta(t *testing.T) {
	t.Run("carries id, name and arguments", func(t *testing.T) {
		d := toolCallDelta(0, map[string]interface{}{
			"id":   "call_1",
			"type": "function",
			"function": map[string]interface{}{
				"name":      "get_weather",
				"arguments": `{"city":"London"}`,
			},
		})

		assert.Equal(t, 0, d.Index)
		assert.Equal(t, "call_1", d.ID)
		assert.Equal(t, "function", d.Type)
		require.NotNil(t, d.Function)
		assert.Equal(t, "get_weather", d.Function.Name)
		assert.True(t, json.Valid([]byte(d.Function.Arguments)))
	})

	t.Run("defaults an absent type to function", func(t *testing.T) {
		// Anthropic's driver leaves Type empty; OpenAI's schema pins it.
		d := toolCallDelta(1, map[string]interface{}{
			"id":       "call_2",
			"function": map[string]interface{}{"name": "f", "arguments": `{}`},
		})
		assert.Equal(t, "function", d.Type)
		assert.Equal(t, 1, d.Index, "index is how a client reassembles fragments")
	})

	t.Run("empty arguments become an empty object", func(t *testing.T) {
		// Clients concatenate argument fragments and unmarshal the result; ""
		// fails with "unexpected end of JSON input".
		d := toolCallDelta(0, map[string]interface{}{
			"id":       "call_3",
			"function": map[string]interface{}{"name": "f"},
		})
		require.NotNil(t, d.Function)
		assert.True(t, json.Valid([]byte(d.Function.Arguments)))
	})
}

// A streamed tool call has to survive the round trip through the chunk struct,
// index included: it is omitempty-free for a reason - index 0 is the common case.
func TestChatCompletionChunkCarriesToolCalls(t *testing.T) {
	chunk := ChatCompletionChunk{
		ID:      "chatcmpl-1",
		Object:  "chat.completion.chunk",
		Created: 1,
		Model:   "m",
		Choices: []ChatCompletionChunkChoice{{
			Index: 0,
			Delta: ChatCompletionDelta{
				Role: "assistant",
				ToolCalls: []ChatCompletionToolCallDelta{{
					Index:    0,
					ID:       "call_1",
					Type:     "function",
					Function: &ChatCompletionFunctionDelta{Name: "get_weather", Arguments: `{"city":"London"}`},
				}},
			},
		}},
	}

	raw, err := json.Marshal(chunk)
	require.NoError(t, err)

	var decoded struct {
		Choices []struct {
			Delta struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					Index    *int   `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded.Choices, 1)
	require.Len(t, decoded.Choices[0].Delta.ToolCalls, 1)

	tc := decoded.Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tc.Index, "index must be on the wire even when it is 0")
	assert.Equal(t, 0, *tc.Index)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "function", tc.Type)
	assert.Equal(t, "get_weather", tc.Function.Name)
	assert.Equal(t, `{"city":"London"}`, tc.Function.Arguments)
}

// A text-only delta must not sprout an empty tool_calls array: clients that test
// for the key's presence would see a tool call that never happened.
func TestTextDeltaOmitsToolCalls(t *testing.T) {
	raw, err := json.Marshal(ChatCompletionDelta{Content: "hi"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"content":"hi"}`, string(raw))
}
