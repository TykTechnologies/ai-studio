package proxy

import (
	"log/slog"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"
)

// Models
type ChatCompletionRequest struct {
	Messages            []Message       `json:"messages"`
	Model               string          `json:"model"`
	Store               *bool           `json:"store,omitempty"`
	Metadata            map[string]any  `json:"metadata,omitempty"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]int  `json:"logit_bias,omitempty"`
	LogProbs            *bool           `json:"logprobs,omitempty"`
	TopLogProbs         *int            `json:"top_logprobs,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	N                   *int            `json:"n,omitempty"`
	Modalities          []string        `json:"modalities,omitempty"`
	Audio               *AudioConfig    `json:"audio,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	Seed                *int            `json:"seed,omitempty"`
	ServiceTier         *string         `json:"service_tier,omitempty"`
	Stop                any             `json:"stop,omitempty"`
	Stream              *bool           `json:"stream,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	User                *string         `json:"user,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type AudioConfig struct {
	Voice  string `json:"voice"`
	Format string `json:"format"`
}

type ResponseFormat struct {
	Type       string `json:"type"`
	JSONSchema *any   `json:"json_schema,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type Tool struct {
	Type     string         `json:"type"`
	Function FunctionConfig `json:"function"`
}

type FunctionConfig struct {
	Description string         `json:"description,omitempty"`
	Name        string         `json:"name"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      *bool          `json:"strict,omitempty"`
}

func (t Tool) ToLangchainTool() llms.Tool {
	return llms.Tool{
		Type: t.Type,
		Function: &llms.FunctionDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		},
	}
}

func (r *ChatCompletionRequest) ToLangchainOptions(conf *models.LLM) []llms.CallOption {
	options := make([]llms.CallOption, 0)

	// OpenAI interface can state model, but upstream might be different
	if conf.Vendor != models.OPENAI {
		// force model if not OpenAI upstream
		if conf.DefaultModel != "" {
			options = append(options, llms.WithModel(conf.DefaultModel))
		}
	} else {
		// it's OpenAI, so we can use the model from the request
		r.Model = conf.DefaultModel
		if r.Model != "" {
			options = append(options, llms.WithModel(r.Model))
		}
	}

	if r.MaxCompletionTokens != nil {
		options = append(options, llms.WithMaxTokens(*r.MaxCompletionTokens))
	}

	if r.N != nil {
		options = append(options, llms.WithN(*r.N))
	}

	if r.Temperature != nil {
		options = append(options, llms.WithTemperature(*r.Temperature))
	}

	if r.TopP != nil {
		options = append(options, llms.WithTopP(*r.TopP))
	}

	if r.FrequencyPenalty != nil {
		options = append(options, llms.WithFrequencyPenalty(*r.FrequencyPenalty))
	}

	if r.PresencePenalty != nil {
		options = append(options, llms.WithPresencePenalty(*r.PresencePenalty))
	}

	if r.Seed != nil {
		options = append(options, llms.WithSeed(*r.Seed))
	}

	// Handle Stop words
	if r.Stop != nil {
		if stopWords, ok := r.GetStop(); ok {
			options = append(options, llms.WithStopWords(stopWords))
		}
	}

	// Handle Tools and ToolChoice
	if len(r.Tools) > 0 {
		langchainTools := make([]llms.Tool, len(r.Tools))
		for i, tool := range r.Tools {
			langchainTools[i] = tool.ToLangchainTool()
		}
		options = append(options, llms.WithTools(langchainTools))
	}

	if r.ToolChoice != nil {
		options = append(options, llms.WithToolChoice(r.ToolChoice))
	}

	// Handle JSON mode
	if r.ResponseFormat != nil && r.ResponseFormat.Type == "json_object" {
		options = append(options, llms.WithJSONMode())
	}

	// Handle Metadata
	if r.Metadata != nil {
		options = append(options, llms.WithMetadata(r.Metadata))
	}

	return options
}

// Getter functions for ambiguous fields
func (r *ChatCompletionRequest) GetStop() ([]string, bool) {
	switch v := r.Stop.(type) {
	case string:
		return []string{v}, true
	case []string:
		return v, true
	default:
		return nil, false
	}
}

func (r *ChatCompletionRequest) GetToolChoice() (string, *FunctionConfig, bool) {
	switch v := r.ToolChoice.(type) {
	case string:
		return v, nil, true
	case map[string]any:
		if typeStr, ok := v["type"].(string); ok && typeStr == "function" {
			if funcMap, ok := v["function"].(map[string]any); ok {
				return "", &FunctionConfig{
					Name: funcMap["name"].(string),
				}, true
			}
		}
	}
	return "", nil, false
}

func (r *ChatCompletionRequest) GetMessageContent(msg Message) (string, []map[string]any, bool) {
	switch v := msg.Content.(type) {
	case string:
		return v, nil, true
	case []map[string]any:
		return "", v, true
	default:
		return "", nil, false
	}
}

type ChatCompletionMessage struct {
	Role      string                   `json:"role"`
	Content   string                   `json:"content"`
	Name      string                   `json:"name,omitempty"`
	ToolCalls []map[string]interface{} `json:"tool_calls,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   CompletionUsage        `json:"usage"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type CreateCompletionRequest struct {
	Model            string      `json:"model"`
	Prompt           string      `json:"prompt,omitempty"`
	MaxTokens        *int        `json:"max_tokens,omitempty"`
	Temperature      *float64    `json:"temperature,omitempty"`
	TopP             *float64    `json:"top_p,omitempty"`
	N                *int        `json:"n,omitempty"`
	Stream           *bool       `json:"stream,omitempty"`
	LogProbs         *int        `json:"logprobs,omitempty"`
	Echo             *bool       `json:"echo,omitempty"`
	Stop             interface{} `json:"stop,omitempty"`
	PresencePenalty  *float64    `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64    `json:"frequency_penalty,omitempty"`
	User             string      `json:"user,omitempty"`
}

type CompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []CompletionChoice `json:"choices"`
	Usage   CompletionUsage    `json:"usage"`
}

type CompletionChoice struct {
	Text         string    `json:"text"`
	Index        int       `json:"index"`
	LogProbs     *LogProbs `json:"logprobs"`
	FinishReason string    `json:"finish_reason"`
}

type CompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type LogProbs struct {
	Tokens        []string             `json:"tokens"`
	TokenLogProbs []float32            `json:"token_logprobs"`
	TopLogProbs   []map[string]float32 `json:"top_logprobs"`
	TextOffset    []int                `json:"text_offset"`
}

// Streaming response types for OpenAI-compatible SSE streaming

// ChatCompletionChunk represents a streaming chunk in OpenAI format
type ChatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"` // "chat.completion.chunk"
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []ChatCompletionChunkChoice `json:"choices"`
	Usage   *CompletionUsage            `json:"usage,omitempty"` // Only in final chunk
}

// ChatCompletionChunkChoice represents a choice in a streaming chunk
type ChatCompletionChunkChoice struct {
	Index        int                 `json:"index"`
	Delta        ChatCompletionDelta `json:"delta"`
	FinishReason *string             `json:"finish_reason"` // null until final chunk
}

// ChatCompletionDelta represents the delta content in a streaming chunk
type ChatCompletionDelta struct {
	Role    string `json:"role,omitempty"`    // Only in first chunk
	Content string `json:"content,omitempty"` // Streaming text content
}

// ChatCompletionStreamError represents an error in SSE format
type ChatCompletionStreamError struct {
	Error ChatCompletionErrorDetail `json:"error"`
}

// ChatCompletionErrorDetail contains error details
type ChatCompletionErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (r *ChatCompletionRequest) GetMessages() []llms.MessageContent {
	messages := make([]llms.MessageContent, len(r.Messages))

	for i, msg := range r.Messages {
		msgContent := llms.MessageContent{
			Role:  convertRole(msg.Role),
			Parts: []llms.ContentPart{},
		}

		// Use the existing GetMessageContent helper
		if textContent, contentArray, ok := r.GetMessageContent(msg); ok {
			if textContent != "" {
				// Simple text content
				msgContent.Parts = append(msgContent.Parts, llms.TextPart(textContent))
			} else if contentArray != nil {
				// Handle multi-part content
				for _, part := range contentArray {
					if contentType, ok := part["type"].(string); ok {
						switch contentType {
						case "text":
							if text, ok := part["text"].(string); ok {
								msgContent.Parts = append(msgContent.Parts, llms.TextPart(text))
							}
						case "image_url":
							if imageUrl, ok := part["image_url"].(map[string]interface{}); ok {
								if url, ok := imageUrl["url"].(string); ok {
									if detail, ok := imageUrl["detail"].(string); ok {
										msgContent.Parts = append(msgContent.Parts,
											llms.ImageURLWithDetailPart(url, detail))
									} else {
										msgContent.Parts = append(msgContent.Parts,
											llms.ImageURLPart(url))
									}
								}
							}
						}
					}
				}
			}
		}

		messages[i] = msgContent
	}

	return messages
}

// convertRole converts OpenAI role strings to langchaingo ChatMessageType
func convertRole(role string) llms.ChatMessageType {
	switch role {
	case "system":
		return llms.ChatMessageTypeSystem
	case "user":
		return llms.ChatMessageTypeHuman
	case "assistant":
		return llms.ChatMessageTypeAI
	case "function":
		return llms.ChatMessageTypeFunction
	case "tool":
		return llms.ChatMessageTypeTool
	default:
		slog.Warn("role is not supported, defaulting to human", "role", role)
		return llms.ChatMessageTypeHuman
	}
}

func NewChatCompletionResponse(llmResponse *llms.ContentResponse, model string) *ChatCompletionResponse {
	response := &ChatCompletionResponse{
		ID:      uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: make([]ChatCompletionChoice, len(llmResponse.Choices)),
	}

	for i, choice := range llmResponse.Choices {
		// Convert the choice
		response.Choices[i] = ChatCompletionChoice{
			Index: i,
			Message: ChatCompletionMessage{
				Role:    "assistant",
				Content: choice.Content,
			},
			FinishReason: convertFinishReason(choice.StopReason),
		}

		// If there are tool calls, add them to the message
		if len(choice.ToolCalls) > 0 {
			toolCalls := make([]map[string]interface{}, len(choice.ToolCalls))
			for j, toolCall := range choice.ToolCalls {
				toolCalls[j] = map[string]interface{}{
					"id":   toolCall.ID,
					"type": toolCall.Type,
					"function": map[string]interface{}{
						"name":      toolCall.FunctionCall.Name,
						"arguments": toolCall.FunctionCall.Arguments,
					},
				}
			}
			response.Choices[i].Message.Content = "" // Clear content when there are tool calls
			response.Choices[i].Message.ToolCalls = toolCalls
		}
	}

	// Note: Usage stats would need to be set separately as they're not part of ContentResponse
	return response
}

// Helper to convert finish reasons
func convertFinishReason(reason string) string {
	// Map langchaingo stop reasons to OpenAI format
	switch reason {
	case "stop":
		return "stop"
	case "length":
		return "length"
	case "tool_calls":
		return "tool_calls"
	case "content_filter":
		return "content_filter"
	default:
		return "stop"
	}
}
