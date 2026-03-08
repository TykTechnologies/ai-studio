package googleaiVendor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogleAI_AnalyzeStreamingResponse_ErrorCases(t *testing.T) {
	v := &GoogleAI{}
	mockLLM := &models.LLM{ID: 1}
	mockApp := &models.App{ID: 1}
	mockRequest := httptest.NewRequest("POST", "/v1/models/gemini-pro:generateContent", nil)

	tests := []struct {
		name          string
		responseBody  []byte
		expectedError string
	}{
		{
			name:          "invalid JSON - not array or single object",
			responseBody:  []byte(`invalid json`),
			expectedError: "failed to unmarshal googleai streaming response",
		},
		{
			name:          "malformed JSON - incomplete",
			responseBody:  []byte(`{"candidates": [`),
			expectedError: "failed to unmarshal googleai streaming response",
		},
		{
			name:          "empty response body",
			responseBody:  []byte(``),
			expectedError: "failed to unmarshal googleai streaming response",
		},
		{
			name:          "null response",
			responseBody:  []byte(`null`),
			expectedError: "googleai streaming response contained no chunks",
		},
		{
			name:          "empty array response",
			responseBody:  []byte(`[]`),
			expectedError: "googleai streaming response contained no chunks",
		},
		{
			name:          "mixed valid and invalid JSON in array",
			responseBody:  []byte(`[{"usageMetadata": {"promptTokenCount": 10}}, invalid]`),
			expectedError: "failed to unmarshal googleai streaming response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm, app, response, err := v.AnalyzeStreamingResponse(
				mockLLM,
				mockApp,
				http.StatusOK,
				tt.responseBody,
				mockRequest,
				[][]byte{},
			)

			assert.Nil(t, llm)
			assert.Nil(t, app)
			assert.Nil(t, response)
			assert.Error(t, err)

			if tt.expectedError != "" {
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestGoogleAI_AnalyzeStreamingResponse_SSEFormat(t *testing.T) {
	v := &GoogleAI{}
	mockLLM := &models.LLM{ID: 1}
	mockApp := &models.App{ID: 1}
	mockRequest := httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", nil)

	// SSE format as returned by Google's streamGenerateContent with alt=sse
	sseResponse := []byte(`data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"index":0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15,"thoughtsTokenCount":0},"modelVersion":"gemini-2.5-pro"}

data: {"candidates":[{"content":{"parts":[{"text":" world!"}],"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":12,"totalTokenCount":22,"thoughtsTokenCount":0},"modelVersion":"gemini-2.5-pro"}

`)

	llm, app, response, err := v.AnalyzeStreamingResponse(
		mockLLM, mockApp, http.StatusOK, sseResponse, mockRequest, [][]byte{},
	)

	require.NoError(t, err)
	assert.NotNil(t, llm)
	assert.NotNil(t, app)
	assert.NotNil(t, response)
	assert.Equal(t, 10, response.GetPromptTokens())
	assert.Equal(t, 12, response.GetResponseTokens())
	assert.Equal(t, "gemini-2.5-pro", response.GetModel())
}

func TestGoogleAI_AnalyzeStreamingResponse_SSEWithThinkingTokens(t *testing.T) {
	v := &GoogleAI{}
	mockLLM := &models.LLM{ID: 1}
	mockApp := &models.App{ID: 1}
	mockRequest := httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", nil)

	sseResponse := []byte(`data: {"candidates":[{"content":{"parts":[{"text":"thinking..."}],"role":"model"},"index":0}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":36,"totalTokenCount":220,"thoughtsTokenCount":176},"modelVersion":"gemini-2.5-pro"}

data: {"candidates":[{"content":{"parts":[{"text":" done."}],"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":43,"totalTokenCount":227,"thoughtsTokenCount":176},"modelVersion":"gemini-2.5-pro"}

`)

	_, _, response, err := v.AnalyzeStreamingResponse(
		mockLLM, mockApp, http.StatusOK, sseResponse, mockRequest, [][]byte{},
	)

	require.NoError(t, err)
	assert.Equal(t, 8, response.GetPromptTokens())
	// Response tokens should include thinking tokens
	assert.Equal(t, 43+176, response.GetResponseTokens())
	assert.Equal(t, "gemini-2.5-pro", response.GetModel())
}

func TestGoogleAI_AnalyzeStreamingResponse_JSONArrayFormat(t *testing.T) {
	v := &GoogleAI{}
	mockLLM := &models.LLM{ID: 1}
	mockApp := &models.App{ID: 1}
	mockRequest := httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-flash:streamGenerateContent", nil)

	// JSON array format (without alt=sse)
	jsonResponse := []byte(`[
		{"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"index":0}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8},"modelVersion":"gemini-2.5-flash"},
		{"candidates":[{"content":{"parts":[{"text":" world"}],"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":7,"totalTokenCount":12},"modelVersion":"gemini-2.5-flash"}
	]`)

	_, _, response, err := v.AnalyzeStreamingResponse(
		mockLLM, mockApp, http.StatusOK, jsonResponse, mockRequest, [][]byte{},
	)

	require.NoError(t, err)
	assert.Equal(t, 5, response.GetPromptTokens())
	assert.Equal(t, 7, response.GetResponseTokens())
	assert.Equal(t, "gemini-2.5-flash", response.GetModel())
}

func TestGoogleAI_AnalyzeStreamingResponse_ModelVersionPrecedence(t *testing.T) {
	v := &GoogleAI{}
	mockLLM := &models.LLM{ID: 1}
	mockApp := &models.App{ID: 1}
	// URL says gemini-flash but response says gemini-2.5-pro
	mockRequest := httptest.NewRequest("POST", "/v1beta/models/gemini-flash:streamGenerateContent", nil)

	jsonResponse := []byte(`[{"candidates":[{"content":{"parts":[{"text":"test"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2},"modelVersion":"gemini-2.5-pro"}]`)

	_, _, response, err := v.AnalyzeStreamingResponse(
		mockLLM, mockApp, http.StatusOK, jsonResponse, mockRequest, [][]byte{},
	)

	require.NoError(t, err)
	// modelVersion from response should take precedence over URL
	assert.Equal(t, "gemini-2.5-pro", response.GetModel())
}

func TestGoogleAI_AnalyzeStreamingResponse_FallbackToURL(t *testing.T) {
	v := &GoogleAI{}
	mockLLM := &models.LLM{ID: 1}
	mockApp := &models.App{ID: 1}
	mockRequest := httptest.NewRequest("POST", "/v1beta/models/gemini-2.5-flash:streamGenerateContent", nil)

	// No modelVersion in response
	jsonResponse := []byte(`[{"candidates":[{"content":{"parts":[{"text":"test"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}]`)

	_, _, response, err := v.AnalyzeStreamingResponse(
		mockLLM, mockApp, http.StatusOK, jsonResponse, mockRequest, [][]byte{},
	)

	require.NoError(t, err)
	// Should fall back to URL extraction
	assert.Equal(t, "gemini-2.5-flash", response.GetModel())
}

func TestGoogleAI_AnalyzeResponse_ModelVersionPrecedence(t *testing.T) {
	v := &GoogleAI{}
	mockLLM := &models.LLM{ID: 1}
	mockApp := &models.App{ID: 1}
	// URL says gemini-flash but response says gemini-2.5-pro
	mockRequest := httptest.NewRequest("POST", "/v1beta/models/gemini-flash:generateContent", nil)

	responseBody := []byte(`{
		"candidates":[{"content":{"parts":[{"text":"test"}],"role":"model"},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":10,"totalTokenCount":15},
		"modelVersion":"gemini-2.5-pro"
	}`)

	_, _, response, err := v.AnalyzeResponse(
		mockLLM, mockApp, http.StatusOK, responseBody, mockRequest,
	)

	require.NoError(t, err)
	assert.Equal(t, "gemini-2.5-pro", response.GetModel())
	assert.Equal(t, 5, response.GetPromptTokens())
	assert.Equal(t, 10, response.GetResponseTokens())
}

func TestIsSSEFormat(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"SSE with data prefix", []byte("data: {\"test\":true}\n"), true},
		{"SSE with leading newline", []byte("\ndata: {\"test\":true}\n"), true},
		{"JSON array", []byte(`[{"test":true}]`), false},
		{"JSON object", []byte(`{"test":true}`), false},
		{"empty", []byte(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isSSEFormat(tt.data))
		})
	}
}
