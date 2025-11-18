package openaiVendor

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestAnalyzeResponse_OpenAICompletions(t *testing.T) {
	v := &OpenAI{}

	// Simulate an OpenAI chat completions response
	restResponse := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello! How can I help you today?"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 9,
			"completion_tokens": 12,
			"total_tokens": 21
		}
	}`

	// Create a mock request with OpenAI completions endpoint
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)

	// Test the response analysis
	llm := &models.LLM{Vendor: "openai"}
	app := &models.App{}
	llmResult, appResult, tokenResp, err := v.AnalyzeResponse(llm, app, 200, []byte(restResponse), req)

	assert.NoError(t, err)
	assert.Equal(t, llm, llmResult)
	assert.Equal(t, app, appResult)
	assert.NotNil(t, tokenResp)

	// Verify token counts
	assert.Equal(t, 9, tokenResp.GetPromptTokens())
	assert.Equal(t, 12, tokenResp.GetResponseTokens())
	assert.Equal(t, "gpt-4", tokenResp.GetModel())
}

func TestAnalyzeResponse_OpenAIEmbeddings(t *testing.T) {
	v := &OpenAI{}

	// Simulate an OpenAI embeddings response
	restResponse := `{
		"object": "list",
		"data": [{
			"object": "embedding",
			"embedding": [0.0023064255, -0.009327292, -0.0028842222],
			"index": 0
		}],
		"model": "text-embedding-ada-002",
		"usage": {
			"prompt_tokens": 8,
			"total_tokens": 8
		}
	}`

	// Create a mock request with OpenAI embeddings endpoint
	req, _ := http.NewRequest("POST", "/v1/embeddings", nil)

	// Test the response analysis
	llm := &models.LLM{Vendor: "openai"}
	app := &models.App{}
	llmResult, appResult, tokenResp, err := v.AnalyzeResponse(llm, app, 200, []byte(restResponse), req)

	assert.NoError(t, err)
	assert.Equal(t, llm, llmResult)
	assert.Equal(t, app, appResult)
	assert.NotNil(t, tokenResp)

	// Verify token counts
	assert.Equal(t, 8, tokenResp.GetPromptTokens())
	assert.Equal(t, "text-embedding-ada-002", tokenResp.GetModel())
}

func TestAnalyzeResponse_AzureOpenAICompletions(t *testing.T) {
	v := &OpenAI{}

	// Simulate an Azure OpenAI chat completions response
	restResponse := `{
		"id": "chatcmpl-azure-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-35-turbo",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Azure OpenAI response"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 15,
			"completion_tokens": 8,
			"total_tokens": 23
		}
	}`

	// Create a mock request with Azure OpenAI completions endpoint
	req, _ := http.NewRequest("POST", "/openai/deployments/gpt-35-turbo/chat/completions?api-version=2024-02-01", nil)

	// Test the response analysis
	llm := &models.LLM{Vendor: "openai"}
	app := &models.App{}
	llmResult, appResult, tokenResp, err := v.AnalyzeResponse(llm, app, 200, []byte(restResponse), req)

	assert.NoError(t, err)
	assert.Equal(t, llm, llmResult)
	assert.Equal(t, app, appResult)
	assert.NotNil(t, tokenResp)

	// Verify token counts
	assert.Equal(t, 15, tokenResp.GetPromptTokens())
	assert.Equal(t, 8, tokenResp.GetResponseTokens())
	assert.Equal(t, "gpt-35-turbo", tokenResp.GetModel())
}

func TestAnalyzeResponse_UnknownEndpoint(t *testing.T) {
	v := &OpenAI{}

	restResponse := `{}`

	// Create a mock request with an unknown endpoint
	req, _ := http.NewRequest("POST", "/v1/unknown/endpoint", nil)

	// Test the response analysis
	llm := &models.LLM{Vendor: "openai"}
	app := &models.App{}
	_, _, _, err := v.AnalyzeResponse(llm, app, 200, []byte(restResponse), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown completions endpoint")
}

func TestAnalyzeResponse_InvalidJSON(t *testing.T) {
	v := &OpenAI{}

	// Invalid JSON response
	restResponse := `{invalid json}`

	// Create a mock request with OpenAI completions endpoint
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)

	// Test the response analysis
	llm := &models.LLM{Vendor: "openai"}
	app := &models.App{}
	_, _, _, err := v.AnalyzeResponse(llm, app, 200, []byte(restResponse), req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal llm rest response")
}
