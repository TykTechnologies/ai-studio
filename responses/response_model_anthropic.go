package responses

import (
	"encoding/json"
)

// Tool used for the request message payload.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

// Content is one block of an Anthropic message: TextContent, ToolUseContent,
// ThinkingContent, RedactedThinkingContent, or OtherContent for every block
// type this package does not model.
type Content interface {
	GetType() string
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (tc TextContent) GetType() string {
	return tc.Type
}

type ToolUseContent struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

func (tuc ToolUseContent) GetType() string {
	return tuc.Type
}

type ToolResultContent struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

func (trc ToolResultContent) GetType() string {
	return trc.Type
}

// ThinkingContent is an extended-thinking block. Thinking may be empty: current
// Claude models add an empty thinking block to tool-use answers.
type ThinkingContent struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature,omitempty"`
}

func (tc ThinkingContent) GetType() string {
	return tc.Type
}

// RedactedThinkingContent is a thinking block the vendor returned encrypted.
type RedactedThinkingContent struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

func (rtc RedactedThinkingContent) GetType() string {
	return rtc.Type
}

// OtherContent stands for a content block this package does not model
// (server_tool_use, web_search_tool_result, a block type added after this code
// was written, or a known type whose body did not parse). Only the type is
// kept: the block can carry model output and must not reach a log.
type OtherContent struct {
	Type string `json:"type"`
}

func (oc OtherContent) GetType() string {
	return oc.Type
}

// This is from langchaingo's implementation
type AnthropicResponse struct {
	Content      []Content `json:"content"`
	ID           string    `json:"id"`
	Model        string    `json:"model"`
	Role         string    `json:"role"`
	StopReason   string    `json:"stop_reason"`
	StopSequence string    `json:"stop_sequence"`
	Type         string    `json:"type"`
	Usage        struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (m *AnthropicResponse) UnmarshalJSON(data []byte) error {
	type Alias AnthropicResponse
	aux := &struct {
		Content []json.RawMessage `json:"content"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// The response is read for accounting: usage, model and the tool count. A
	// content block this package cannot read never fails the whole response,
	// because the caller would then drop the analytics record, the budget
	// charge and the trace span for a call the vendor billed.
	for _, raw := range aux.Content {
		var typeStruct struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &typeStruct); err != nil {
			m.Content = append(m.Content, &OtherContent{})
			continue
		}

		var block Content
		switch typeStruct.Type {
		case "text":
			block = &TextContent{}
		case "tool_use":
			block = &ToolUseContent{}
		case "thinking":
			block = &ThinkingContent{}
		case "redacted_thinking":
			block = &RedactedThinkingContent{}
		default:
			m.Content = append(m.Content, &OtherContent{Type: typeStruct.Type})
			continue
		}
		if err := json.Unmarshal(raw, block); err != nil {
			// A tool_use block stays a tool_use block for the tool count.
			if typeStruct.Type == "tool_use" {
				block = &ToolUseContent{Type: typeStruct.Type}
			} else {
				block = &OtherContent{Type: typeStruct.Type}
			}
		}
		m.Content = append(m.Content, block)
	}

	return nil
}

// UnmodelledContentTypes returns the types of the content blocks that were
// kept as OtherContent, in order and without duplicates. It is for logging:
// the type only, never the block.
func (a *AnthropicResponse) UnmodelledContentTypes() []string {
	var types []string
	seen := map[string]bool{}
	for _, c := range a.Content {
		oc, ok := c.(*OtherContent)
		if !ok || seen[oc.Type] {
			continue
		}
		seen[oc.Type] = true
		types = append(types, oc.Type)
	}
	return types
}

func (a *AnthropicResponse) GetPromptTokens() int {
	return a.Usage.InputTokens
}

func (a *AnthropicResponse) GetResponseTokens() int {
	return a.Usage.OutputTokens
}

func (a *AnthropicResponse) GetChoiceCount() int {
	return 0
}

func (a *AnthropicResponse) GetToolCount() int {
	cnt := 0
	for _, c := range a.Content {
		if c.GetType() == "tool_use" {
			cnt += 1
		}
	}

	return cnt
}

func (a *AnthropicResponse) GetModel() string {
	return a.Model
}

func (a *AnthropicResponse) GetCacheWritePromptTokens() int {
	return a.Usage.CacheCreationInputTokens
}

func (a *AnthropicResponse) GetCacheReadPromptTokens() int {
	return a.Usage.CacheReadInputTokens
}

type DummyResponse struct {
	Usage struct {
		PromptTokens   int `json:"prompt_tokens"`
		ResponseTokens int `json:"response_tokens"`
	}
	Model string `json:"model"`
}

func (o *DummyResponse) GetPromptTokens() int {
	return o.Usage.PromptTokens
}

func (o *DummyResponse) GetResponseTokens() int {
	return o.Usage.ResponseTokens
}

func (o *DummyResponse) GetChoiceCount() int {
	return 0
}

func (o *DummyResponse) GetToolCount() int {
	return 0
}

func (o *DummyResponse) GetModel() string {
	if o.Model == "" {
		return "dummy"
	}

	return o.Model
}

func (o *DummyResponse) GetCacheWritePromptTokens() int {
	return 0
}

func (o *DummyResponse) GetCacheReadPromptTokens() int {
	return 0
}

type ProxyDummyResponse struct {
	Model string `json:"model"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (o *ProxyDummyResponse) GetPromptTokens() int {
	return o.Usage.PromptTokens
}

func (o *ProxyDummyResponse) GetResponseTokens() int {
	return o.Usage.CompletionTokens
}

func (o *ProxyDummyResponse) GetChoiceCount() int {
	return 0
}

func (o *ProxyDummyResponse) GetToolCount() int {
	return 0
}

func (o *ProxyDummyResponse) GetModel() string {
	if o.Model == "" {
		return "dummy"
	}

	return o.Model
}

func (o *ProxyDummyResponse) GetCacheWritePromptTokens() int {
	return 0
}

func (o *ProxyDummyResponse) GetCacheReadPromptTokens() int {
	return 0
}

type AnthropicStreamingChunkType struct {
	Type string `json:"type"`
}

type AnthropicStreamingChunkDelta struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason   string `json:"stop_reason"`
		StopSequence any    `json:"stop_sequence"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type AnthropicStreamingChunkStart struct {
	Type    string `json:"type"`
	Message struct {
		ID           string `json:"id"`
		Type         string `json:"type"`
		Role         string `json:"role"`
		Content      []any  `json:"content"`
		Model        string `json:"model"`
		StopReason   any    `json:"stop_reason"`
		StopSequence any    `json:"stop_sequence"`
		Usage        struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type AnthropicStreamingChunkCBStart struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input struct {
		} `json:"input"`
	} `json:"content_block"`
}
