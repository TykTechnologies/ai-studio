package proxy

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/llms"
)

func TestTool_ToLangchainTool(t *testing.T) {
	t.Run("Convert tool to langchain format", func(t *testing.T) {
		tool := Tool{
			Type: "function",
			Function: FunctionConfig{
				Name:        "get_weather",
				Description: "Get current weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		}

		lcTool := tool.ToLangchainTool()
		assert.Equal(t, "function", lcTool.Type)
		assert.NotNil(t, lcTool.Function)
		assert.Equal(t, "get_weather", lcTool.Function.Name)
		assert.Equal(t, "Get current weather", lcTool.Function.Description)
	})
}

func TestChatCompletionRequest_GetStop(t *testing.T) {
	t.Run("Get stop as string", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Stop: "STOP",
		}
		stopWords, ok := req.GetStop()
		assert.True(t, ok)
		assert.Equal(t, []string{"STOP"}, stopWords)
	})

	t.Run("Get stop as string array", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Stop: []string{"STOP", "END"},
		}
		stopWords, ok := req.GetStop()
		assert.True(t, ok)
		assert.Equal(t, []string{"STOP", "END"}, stopWords)
	})

	t.Run("Get stop with invalid type", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Stop: 123, // Invalid type
		}
		stopWords, ok := req.GetStop()
		assert.False(t, ok)
		assert.Nil(t, stopWords)
	})

	t.Run("Get stop with nil", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Stop: nil,
		}
		stopWords, ok := req.GetStop()
		assert.False(t, ok)
		assert.Nil(t, stopWords)
	})
}

func TestChatCompletionRequest_GetToolChoice(t *testing.T) {
	t.Run("Get tool choice as string", func(t *testing.T) {
		req := &ChatCompletionRequest{
			ToolChoice: "auto",
		}
		choice, funcConfig, ok := req.GetToolChoice()
		assert.True(t, ok)
		assert.Equal(t, "auto", choice)
		assert.Nil(t, funcConfig)
	})

	t.Run("Get tool choice as function object", func(t *testing.T) {
		req := &ChatCompletionRequest{
			ToolChoice: map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": "my_function",
				},
			},
		}
		choice, funcConfig, ok := req.GetToolChoice()
		assert.True(t, ok)
		assert.Equal(t, "", choice)
		assert.NotNil(t, funcConfig)
		assert.Equal(t, "my_function", funcConfig.Name)
	})

	t.Run("Get tool choice with invalid type", func(t *testing.T) {
		req := &ChatCompletionRequest{
			ToolChoice: 123,
		}
		choice, funcConfig, ok := req.GetToolChoice()
		assert.False(t, ok)
		assert.Equal(t, "", choice)
		assert.Nil(t, funcConfig)
	})
}

func TestChatCompletionRequest_GetMessageContent(t *testing.T) {
	req := &ChatCompletionRequest{}

	t.Run("Get message content as string", func(t *testing.T) {
		msg := Message{
			Role:    "user",
			Content: "Hello, world!",
		}
		text, parts, ok := req.GetMessageContent(msg)
		assert.True(t, ok)
		assert.Equal(t, "Hello, world!", text)
		assert.Nil(t, parts)
	})

	t.Run("Get message content as array", func(t *testing.T) {
		msg := Message{
			Role: "user",
			Content: []map[string]any{
				{"type": "text", "text": "Hello"},
				{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/image.jpg"}},
			},
		}
		text, parts, ok := req.GetMessageContent(msg)
		assert.True(t, ok)
		assert.Equal(t, "", text)
		assert.Len(t, parts, 2)
	})

	t.Run("Get message content with invalid type", func(t *testing.T) {
		msg := Message{
			Role:    "user",
			Content: 123,
		}
		text, parts, ok := req.GetMessageContent(msg)
		assert.False(t, ok)
		assert.Equal(t, "", text)
		assert.Nil(t, parts)
	})
}

func TestChatCompletionRequest_ToLangchainOptions(t *testing.T) {
	llmConfig := &models.LLM{
		Vendor:       models.OPENAI,
		DefaultModel: "gpt-4",
	}

	t.Run("Convert basic options", func(t *testing.T) {
		temp := 0.7
		maxTokens := 100
		req := &ChatCompletionRequest{
			Model:               "gpt-4",
			Temperature:         &temp,
			MaxCompletionTokens: &maxTokens,
		}

		opts := req.ToLangchainOptions(llmConfig)
		assert.NotNil(t, opts)
		assert.Greater(t, len(opts), 0)
	})

	t.Run("Convert with tools", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Tools: []Tool{
				{
					Type: "function",
					Function: FunctionConfig{
						Name:        "test_func",
						Description: "Test function",
					},
				},
			},
		}

		opts := req.ToLangchainOptions(llmConfig)
		assert.NotNil(t, opts)
		// Verify tools are included in options
		assert.Greater(t, len(opts), 0)
	})

	t.Run("Convert with stop words", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Stop: []string{"STOP", "END"},
		}

		opts := req.ToLangchainOptions(llmConfig)
		assert.NotNil(t, opts)
	})

	t.Run("Convert with JSON response format", func(t *testing.T) {
		req := &ChatCompletionRequest{
			ResponseFormat: &ResponseFormat{
				Type: "json_object",
			},
		}

		opts := req.ToLangchainOptions(llmConfig)
		assert.NotNil(t, opts)
	})

	t.Run("Non-OpenAI vendor uses default model", func(t *testing.T) {
		anthropicConfig := &models.LLM{
			Vendor:       models.ANTHROPIC,
			DefaultModel: "claude-3",
		}

		req := &ChatCompletionRequest{
			Model: "gpt-4", // This should be ignored for non-OpenAI
		}

		opts := req.ToLangchainOptions(anthropicConfig)
		assert.NotNil(t, opts)
	})
}

func TestChatCompletionRequest_GetMessages(t *testing.T) {
	t.Run("Convert simple text messages", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Messages: []Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
		}

		messages := req.GetMessages()
		assert.Len(t, messages, 2)
		assert.Equal(t, llms.ChatMessageTypeHuman, messages[0].Role)
		assert.Equal(t, llms.ChatMessageTypeAI, messages[1].Role)
		assert.Len(t, messages[0].Parts, 1)
	})

	t.Run("Convert multi-part content", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Messages: []Message{
				{
					Role: "user",
					Content: []map[string]any{
						{"type": "text", "text": "What's in this image?"},
						{
							"type": "image_url",
							"image_url": map[string]any{
								"url":    "https://example.com/image.jpg",
								"detail": "high",
							},
						},
					},
				},
			},
		}

		messages := req.GetMessages()
		assert.Len(t, messages, 1)
		assert.Len(t, messages[0].Parts, 2)
	})
}

func TestConvertRole(t *testing.T) {
	tests := []struct {
		input    string
		expected llms.ChatMessageType
	}{
		{"system", llms.ChatMessageTypeSystem},
		{"user", llms.ChatMessageTypeHuman},
		{"assistant", llms.ChatMessageTypeAI},
		{"function", llms.ChatMessageTypeFunction},
		{"tool", llms.ChatMessageTypeTool},
		{"unknown", llms.ChatMessageTypeHuman}, // Default
	}

	for _, tt := range tests {
		t.Run("Convert role: "+tt.input, func(t *testing.T) {
			result := convertRole(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "stop"},
		{"length", "length"},
		{"tool_calls", "tool_calls"},
		{"content_filter", "content_filter"},
		{"unknown", "stop"}, // Default
	}

	for _, tt := range tests {
		t.Run("Convert finish reason: "+tt.input, func(t *testing.T) {
			result := convertFinishReason(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewChatCompletionResponse(t *testing.T) {
	t.Run("Create response from langchain content", func(t *testing.T) {
		llmResp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{
					Content:    "This is a response",
					StopReason: "stop",
				},
			},
		}

		resp := NewChatCompletionResponse(llmResp, "gpt-4")
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, "chat.completion", resp.Object)
		assert.Equal(t, "gpt-4", resp.Model)
		assert.Len(t, resp.Choices, 1)
		assert.Equal(t, "This is a response", resp.Choices[0].Message.Content)
		assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
		assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	})

	t.Run("Create response with tool calls", func(t *testing.T) {
		llmResp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{
					Content:    "Tool call response",
					StopReason: "tool_calls",
					ToolCalls: []llms.ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							FunctionCall: &llms.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "London"}`,
							},
						},
					},
				},
			},
		}

		resp := NewChatCompletionResponse(llmResp, "gpt-4")
		assert.Len(t, resp.Choices, 1)
		assert.Empty(t, resp.Choices[0].Message.Content) // Content cleared when tool calls present
		assert.Len(t, resp.Choices[0].Message.ToolCalls, 1)
		assert.Equal(t, "call_123", resp.Choices[0].Message.ToolCalls[0]["id"])
	})

	t.Run("Create response with multiple choices", func(t *testing.T) {
		llmResp := &llms.ContentResponse{
			Choices: []*llms.ContentChoice{
				{Content: "Choice 1", StopReason: "stop"},
				{Content: "Choice 2", StopReason: "stop"},
			},
		}

		resp := NewChatCompletionResponse(llmResp, "gpt-4")
		assert.Len(t, resp.Choices, 2)
		assert.Equal(t, 0, resp.Choices[0].Index)
		assert.Equal(t, 1, resp.Choices[1].Index)
	})
}
