package responses

import "github.com/tmc/langchaingo/llms"

type OpenAIRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Stream        bool `json:"stream"`
	StreamOptions struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

type AnthropicRequest struct {
	Model     string `json:"model"`
	Messages  []any  `json:"messages"`
	MaxTokens int    `json:"max_tokens"`

	System        string         `json:"-"`
	MultiSystem   []any          `json:"-"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	StopSequences []string       `json:"stop_sequences,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	Temperature   *float32       `json:"temperature,omitempty"`
	TopP          *float32       `json:"top_p,omitempty"`
	TopK          *int           `json:"top_k,omitempty"`
	Tools         []any          `json:"tools,omitempty"`
	ToolChoice    *ToolChoice    `json:"tool_choice,omitempty"`
}

type OpenAIResponse struct {
	ID                string                  `json:"id,omitempty"`
	Created           int64                   `json:"created,omitempty"`
	Choices           []*ChatCompletionChoice `json:"choices,omitempty"`
	Model             string                  `json:"model,omitempty"`
	Object            string                  `json:"object,omitempty"`
	Usage             ChatUsage               `json:"usage,omitempty"`
	SystemFingerprint string                  `json:"system_fingerprint"`
}

// ChatCompletionChoice is a choice in a chat response.
type ChatCompletionChoice struct {
	Index        int          `json:"index"`
	Message      ChatMessage  `json:"message"`
	FinishReason FinishReason `json:"finish_reason"`
}

// ToolType is the type of a tool.
type ToolType string

const (
	ToolTypeFunction ToolType = "function"
)

type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonFunctionCall  FinishReason = "function_call"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonNull          FinishReason = "null"
)

func (r FinishReason) MarshalJSON() ([]byte, error) {
	if r == FinishReasonNull || r == "" {
		return []byte("null"), nil
	}
	return []byte(`"` + string(r) + `"`), nil // best effort to not break future API changes
}

// FunctionDefinition is a definition of a function that can be called by the model.
type FunctionDefinition struct {
	// Name is the name of the function.
	Name string `json:"name"`
	// Description is a description of the function.
	Description string `json:"description,omitempty"`
	// Parameters is a list of parameters for the function.
	Parameters any `json:"parameters"`
	// Strict is a flag to enable structured output mode.
	Strict bool `json:"strict,omitempty"`
}

// ToolChoice is a choice of a tool to use.
type ToolChoice struct {
	Type     ToolType     `json:"type"`
	Function ToolFunction `json:"function,omitempty"`
}

// ToolFunction is a function to be called in a tool choice.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall is a call to a tool.
type ToolCall struct {
	ID       string       `json:"id,omitempty"`
	Type     ToolType     `json:"type"`
	Function ToolFunction `json:"function,omitempty"`
}

type ResponseFormatJSONSchemaProperty struct {
	Type                 string                                       `json:"type"`
	Description          string                                       `json:"description,omitempty"`
	Enum                 []interface{}                                `json:"enum,omitempty"`
	Items                *ResponseFormatJSONSchemaProperty            `json:"items,omitempty"`
	Properties           map[string]*ResponseFormatJSONSchemaProperty `json:"properties,omitempty"`
	AdditionalProperties bool                                         `json:"additionalProperties"`
	Required             []string                                     `json:"required,omitempty"`
	Ref                  string                                       `json:"$ref,omitempty"`
}

type ResponseFormatJSONSchema struct {
	Name   string                            `json:"name"`
	Strict bool                              `json:"strict"`
	Schema *ResponseFormatJSONSchemaProperty `json:"schema"`
}

// ResponseFormat is the format of the response.
type ResponseFormat struct {
	Type       string                    `json:"type"`
	JSONSchema *ResponseFormatJSONSchema `json:"json_schema,omitempty"`
}

type ChatMessage struct { //nolint:musttag
	// The role of the author of this message. One of system, user, assistant, function, or tool.
	Role string

	// The content of the message.
	// This field is mutually exclusive with MultiContent.
	Content string

	// MultiContent is a list of content parts to use in the message.
	MultiContent []llms.ContentPart

	// The name of the author of this message. May contain a-z, A-Z, 0-9, and underscores,
	// with a maximum length of 64 characters.
	Name string

	// ToolCalls is a list of tools that were called in the message.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID is the ID of the tool call this message is for.
	// Only present in tool messages.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ChatUsage is the usage of a chat completion request.
type ChatUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

type Usage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (o *OpenAIResponse) GetPromptTokens() int {
	return o.Usage.PromptTokens
}

func (o *OpenAIResponse) GetResponseTokens() int {
	return o.Usage.CompletionTokens
}

func (o *OpenAIResponse) GetChoiceCount() int {
	return len(o.Choices)
}

func (o *OpenAIResponse) GetToolCount() int {
	cnt := 0
	for _, choice := range o.Choices {
		cnt += len(choice.Message.ToolCalls)
	}

	return cnt
}

func (o *OpenAIResponse) GetModel() string {
	return o.Model
}

func (o *OpenAIResponse) GetCacheWritePromptTokens() int {
	return 0
}

func (o *OpenAIResponse) GetCacheReadPromptTokens() int {
	return 0
}

type OpenAIStreamingResponse struct {
	ID                string `json:"id"`
	Object            string `json:"object"`
	Created           int    `json:"created"`
	Model             string `json:"model"`
	SystemFingerprint string `json:"system_fingerprint"`
	Choices           []struct {
		Index        int                    `json:"index"`
		Delta        map[string]interface{} `json:"delta"`
		Logprobs     any                    `json:"logprobs"`
		FinishReason string                 `json:"finish_reason"`
	} `json:"choices"`
	Usage *OAIUsage `json:"usage"`
}

type OAIUsage struct {
	CompletionTokens int `json:"completion_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (o *OpenAIStreamingResponse) GetPromptTokens() int {
	return o.Usage.PromptTokens
}

func (o *OpenAIStreamingResponse) GetResponseTokens() int {
	return o.Usage.CompletionTokens
}

func (o *OpenAIStreamingResponse) GetChoiceCount() int {
	return len(o.Choices)
}

func (o *OpenAIStreamingResponse) GetToolCount() int {
	cnt := 0
	if len(o.Choices) > 0 {
		for _, v := range o.Choices {
			if v.FinishReason == "tool_calls" {
				if _, ok := v.Delta["tool_calls"]; ok {
					cnt += len(v.Delta["tool_calls"].([]interface{}))
				}
			}
		}
	}
	return cnt
}

func (o *OpenAIStreamingResponse) GetModel() string {
	return o.Model
}

func (o *OpenAIStreamingResponse) GetCacheWritePromptTokens() int {
	return 0
}

func (o *OpenAIStreamingResponse) GetCacheReadPromptTokens() int {
	return 0
}
