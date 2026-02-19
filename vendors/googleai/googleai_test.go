package googleaiVendor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
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
