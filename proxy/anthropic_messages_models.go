package proxy

import "encoding/json"

// This file defines the Anthropic Messages API wire format
// (https://docs.anthropic.com/en/api/messages) as consumed and produced by the
// Anthropic -> Bedrock bridge. Only the fields the bridge needs are modelled;
// unknown fields (e.g. "metadata", beta extras) are ignored by encoding/json.

// --- Request ---

// AnthropicMessagesRequest is the body of POST /anthropic/{routeId}/v1/messages.
type AnthropicMessagesRequest struct {
	Model         string               `json:"model"`
	Messages      []AnthropicMessage   `json:"messages"`
	System        json.RawMessage      `json:"system,omitempty"` // string | []AnthropicContentBlock
	MaxTokens     int                  `json:"max_tokens"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	Tools         []AnthropicTool      `json:"tools,omitempty"`
	ToolChoice    *AnthropicToolChoice `json:"tool_choice,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
}

// AnthropicMessage is a single conversation turn. Content is either a plain
// string or an array of typed content blocks.
type AnthropicMessage struct {
	Role    string          `json:"role"` // "user" | "assistant"
	Content json.RawMessage `json:"content"`
}

// AnthropicContentBlock is one block within a message's content array (or a
// system block). Different block types populate different fields.
type AnthropicContentBlock struct {
	Type string `json:"type"` // "text" | "tool_use" | "tool_result"

	// text
	Text string `json:"text,omitempty"`

	// tool_use (assistant asking to call a tool)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result (user returning a tool's output)
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string | []AnthropicContentBlock
	IsError   bool            `json:"is_error,omitempty"`

	// prompt caching marker
	CacheControl *AnthropicCacheControl `json:"cache_control,omitempty"`
}

// AnthropicCacheControl marks a block as a prompt-cache breakpoint.
type AnthropicCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// AnthropicTool is a tool definition the model may call.
type AnthropicTool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]any         `json:"input_schema,omitempty"`
	CacheControl *AnthropicCacheControl `json:"cache_control,omitempty"`
}

// AnthropicToolChoice controls whether/which tool the model must use.
type AnthropicToolChoice struct {
	Type string `json:"type"` // "auto" | "any" | "tool"
	Name string `json:"name,omitempty"`
}

// --- Response (non-streaming) ---

// AnthropicMessagesResponse is the non-streaming response, and also the message
// skeleton embedded in the streaming "message_start" event.
type AnthropicMessagesResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"` // "message"
	Role         string                   `json:"role"` // "assistant"
	Model        string                   `json:"model"`
	Content      []AnthropicResponseBlock `json:"content"`
	StopReason   string                   `json:"stop_reason,omitempty"`
	StopSequence *string                  `json:"stop_sequence"`
	Usage        AnthropicUsage           `json:"usage"`
}

// AnthropicResponseBlock is an output content block (text or tool_use).
type AnthropicResponseBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// AnthropicUsage carries token counts. input_tokens is reported in message_start,
// output_tokens in message_delta (streaming) or both together (non-streaming).
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- Helpers ---

// parseAnthropicBlocks normalises a field that may be a JSON string or an array
// of content blocks into a block slice. A bare string becomes a single text block.
func parseAnthropicBlocks(raw json.RawMessage) ([]AnthropicContentBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil, nil
		}
		return []AnthropicContentBlock{{Type: "text", Text: s}}, nil
	}
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// anthropicContentToText flattens a tool_result content field (string or block
// array) into plain text for the Bedrock tool result content block.
func anthropicContentToText(raw json.RawMessage) string {
	blocks, err := parseAnthropicBlocks(raw)
	if err != nil {
		// Fall back to the raw JSON so nothing is silently dropped.
		return string(raw)
	}
	var out string
	for _, b := range blocks {
		if b.Type == "text" || b.Text != "" {
			out += b.Text
		}
	}
	return out
}
