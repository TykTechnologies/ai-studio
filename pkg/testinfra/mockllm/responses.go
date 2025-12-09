package mockllm

import (
	"encoding/json"
	"fmt"
	"time"
)

// ChatCompletionResponse represents an OpenAI-compatible chat completion response.
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// BuildChatCompletionResponse creates a complete OpenAI-compatible chat completion response.
func BuildChatCompletionResponse(model string) []byte {
	response := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "This is a mock response from the test LLM backend.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": 15,
			"total_tokens":      25,
		},
	}

	data, _ := json.Marshal(response)
	return data
}

// BuildChatCompletionResponseWithContent creates a response with custom content.
func BuildChatCompletionResponseWithContent(model, content string) []byte {
	response := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": len(content) / 4, // Rough estimate
			"total_tokens":      10 + len(content)/4,
		},
	}

	data, _ := json.Marshal(response)
	return data
}

// BuildErrorResponse creates an OpenAI-compatible error response.
func BuildErrorResponse(errorType, message, code string) []byte {
	response := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errorType,
			"code":    code,
		},
	}

	data, _ := json.Marshal(response)
	return data
}

// BuildStreamChunk creates a streaming chunk for SSE responses.
func BuildStreamChunk(model, content string, index int) []byte {
	chunk := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano()),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]string{
					"content": content,
				},
				"finish_reason": nil,
			},
		},
	}

	data, _ := json.Marshal(chunk)
	return []byte(fmt.Sprintf("data: %s\n\n", data))
}

// BuildStreamDone creates the final SSE done message.
func BuildStreamDone() []byte {
	return []byte("data: [DONE]\n\n")
}
