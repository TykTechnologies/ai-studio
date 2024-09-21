package responses

import (
	"encoding/json"
	"fmt"
)

// Tool used for the request message payload.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

// Content can be TextContent or ToolUseContent depending on the type.
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
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
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

	for _, raw := range aux.Content {
		var typeStruct struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &typeStruct); err != nil {
			return err
		}

		switch typeStruct.Type {
		case "text":
			tc := &TextContent{}
			if err := json.Unmarshal(raw, tc); err != nil {
				return err
			}
			m.Content = append(m.Content, tc)
		case "tool_use":
			tuc := &ToolUseContent{}
			if err := json.Unmarshal(raw, tuc); err != nil {
				return err
			}
			m.Content = append(m.Content, tuc)
		default:
			return fmt.Errorf("unknown content type: %s\n%v", typeStruct.Type, string(raw))
		}
	}

	return nil
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
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
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
