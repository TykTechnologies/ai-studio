package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLLMSettingsEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create LLMSettings
	createLLMSettingsInput := LLMSettingsInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			} `json:"attributes"`
		}{
			Type: "llm-settings",
			Attributes: struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			}{
				ModelName:         "TestModel",
				MaxLength:         100,
				MaxTokens:         50,
				Metadata:          map[string]interface{}{"key": "value"},
				MinLength:         10,
				RepetitionPenalty: 1.2,
				Seed:              42,
				StopWords:         []string{"stop1", "stop2"},
				Temperature:       0.7,
				TopK:              40,
				TopP:              0.9,
				SystemPrompt:      "Test prompt",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/llm-settings", createLLMSettingsInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]LLMSettingsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "TestModel", response["data"].Attributes.ModelName)

	settingsID := response["data"].ID

	// Test Get LLMSettings
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update LLMSettings
	updateLLMSettingsInput := LLMSettingsInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			} `json:"attributes"`
		}{
			Type: "llm-settings",
			Attributes: struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			}{
				ModelName:         "UpdatedTestModel",
				MaxLength:         120,
				MaxTokens:         60,
				Metadata:          map[string]interface{}{"key": "updated_value"},
				MinLength:         15,
				RepetitionPenalty: 1.3,
				Seed:              43,
				StopWords:         []string{"stop1", "stop2", "stop3"},
				Temperature:       0.8,
				TopK:              50,
				TopP:              0.95,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), updateLLMSettingsInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List LLMSettings
	w = performRequest(api.router, "GET", "/api/v1/llm-settings", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]LLMSettingsResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "UpdatedTestModel", listResponse["data"][0].Attributes.ModelName)

	// Test Search LLMSettings
	w = performRequest(api.router, "GET", "/api/v1/llm-settings/search?model_name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]LLMSettingsResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "UpdatedTestModel", searchResponse["data"][0].Attributes.ModelName)

	// Test Delete LLMSettings
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify LLMSettings is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
