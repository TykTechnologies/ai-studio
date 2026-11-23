package responses

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func anthropicBody(content string) []byte {
	return []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-luna",
		"content":` + content + `,"stop_reason":"tool_use",
		"usage":{"input_tokens":11,"output_tokens":7,"cache_creation_input_tokens":3,"cache_read_input_tokens":5}}`)
}

// A content block this package does not model must never fail the whole
// response: the caller drops the analytics record, the budget charge and the
// trace span when it does (midsommar-clean-6bg5.5). Usage, model and the tool
// count are read whatever the blocks are.
func TestAnthropicResponse_ContentBlocksNeverFailTheResponse(t *testing.T) {
	const toolUse = `{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Oslo"}}`

	for _, tc := range []struct {
		name      string
		content   string
		types     []string
		tools     int
		unmodeled []string
	}{
		{
			name:    "empty thinking block before a tool call",
			content: `[{"type":"thinking","thinking":"","signature":"c2ln"},` + toolUse + `]`,
			types:   []string{"thinking", "tool_use"},
			tools:   1,
		},
		{
			name:    "non-empty thinking block before a tool call",
			content: `[{"type":"thinking","thinking":"The user wants the weather.","signature":"c2ln"},{"type":"text","text":"hi"},` + toolUse + `]`,
			types:   []string{"thinking", "text", "tool_use"},
			tools:   1,
		},
		{
			name:    "redacted thinking",
			content: `[{"type":"redacted_thinking","data":"ZW5jcnlwdGVk"},{"type":"text","text":"hi"}]`,
			types:   []string{"redacted_thinking", "text"},
		},
		{
			name: "server tool use and its result",
			content: `[{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"oslo"}},
				{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","url":"https://example.com","title":"t"}]},
				{"type":"text","text":"hi"}]`,
			types:     []string{"server_tool_use", "web_search_tool_result", "text"},
			unmodeled: []string{"server_tool_use", "web_search_tool_result"},
		},
		{
			name:      "a block type that does not exist yet",
			content:   `[{"type":"future_block","payload":{"a":1}},{"type":"future_block"},` + toolUse + `]`,
			types:     []string{"future_block", "future_block", "tool_use"},
			tools:     1,
			unmodeled: []string{"future_block"},
		},
		{
			name:      "blocks that are not objects, and a known type with a wrong shape",
			content:   `["loose string",{"type":"text","text":{"nested":true}},{"type":"tool_use","id":"toolu_2","name":"f","input":"not an object"}]`,
			types:     []string{"", "text", "tool_use"},
			tools:     1,
			unmodeled: []string{"", "text"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp AnthropicResponse
			require.NoError(t, json.Unmarshal(anthropicBody(tc.content), &resp))

			var types []string
			for _, c := range resp.Content {
				types = append(types, c.GetType())
			}
			assert.Equal(t, tc.types, types)
			assert.Equal(t, tc.tools, resp.GetToolCount())
			assert.Equal(t, tc.unmodeled, resp.UnmodelledContentTypes())

			assert.Equal(t, "claude-luna", resp.GetModel())
			assert.Equal(t, 11, resp.GetPromptTokens())
			assert.Equal(t, 7, resp.GetResponseTokens())
			assert.Equal(t, 3, resp.GetCacheWritePromptTokens())
			assert.Equal(t, 5, resp.GetCacheReadPromptTokens())
		})
	}
}

func TestAnthropicResponse_ThinkingBlocksAreParsed(t *testing.T) {
	var resp AnthropicResponse
	require.NoError(t, json.Unmarshal(anthropicBody(
		`[{"type":"thinking","thinking":"step one","signature":"c2ln"},{"type":"redacted_thinking","data":"ZW5j"},{"type":"text","text":"hi"}]`), &resp))

	require.Len(t, resp.Content, 3)
	thinking, ok := resp.Content[0].(*ThinkingContent)
	require.True(t, ok)
	assert.Equal(t, "step one", thinking.Thinking)
	assert.Equal(t, "c2ln", thinking.Signature)
	redacted, ok := resp.Content[1].(*RedactedThinkingContent)
	require.True(t, ok)
	assert.Equal(t, "ZW5j", redacted.Data)
	text, ok := resp.Content[2].(*TextContent)
	require.True(t, ok)
	assert.Equal(t, "hi", text.Text)
}

// A body that is not a message at all is still an error.
func TestAnthropicResponse_InvalidJSONIsAnError(t *testing.T) {
	var resp AnthropicResponse
	assert.Error(t, json.Unmarshal([]byte(`{"content":"not a list"}`), &resp))
	assert.Error(t, json.Unmarshal([]byte(`not json`), &resp))
}
