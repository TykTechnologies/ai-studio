package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestLLMEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create LLM
	createLLMInput := LLMInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				APIKey           string   `json:"api_key"`
				APIEndpoint      string   `json:"api_endpoint"`
				PrivacyScore     int      `json:"privacy_score"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				LogoURL          string   `json:"logo_url"`
				Vendor           string   `json:"vendor"`
				Active           bool     `json:"active"`
				Filters          []int    `json:"filters"`
				DefaultModel     string   `json:"default_model"`
				AllowedModels    []string `json:"allowed_models"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name             string   `json:"name"`
				APIKey           string   `json:"api_key"`
				APIEndpoint      string   `json:"api_endpoint"`
				PrivacyScore     int      `json:"privacy_score"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				LogoURL          string   `json:"logo_url"`
				Vendor           string   `json:"vendor"`
				Active           bool     `json:"active"`
				Filters          []int    `json:"filters"`
				DefaultModel     string   `json:"default_model"`
				AllowedModels    []string `json:"allowed_models"`
			}{
				Name:             "Test LLM",
				APIKey:           "test-api-key",
				APIEndpoint:      "https://api.test.com",
				PrivacyScore:     75,
				ShortDescription: "A test LLM",
				LongDescription:  "This is a test LLM for API testing",
				LogoURL:          "https://testllm.com/logo.png",
				Vendor:           "Test Vendor",
				Active:           true,
				DefaultModel:     "gpt-4",
				AllowedModels:    []string{"gpt-4.*", "gpt-3.5-turbo"},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/llms", createLLMInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]LLMResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test LLM", response["data"].Attributes.Name)
	assert.Equal(t, []string{"gpt-4.*", "gpt-3.5-turbo"}, response["data"].Attributes.AllowedModels)

	llmID := response["data"].ID

	// Test Get LLM
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/llms/%s", llmID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var getLLMResponse map[string]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &getLLMResponse)
	assert.NoError(t, err)
	assert.Equal(t, []string{"gpt-4.*", "gpt-3.5-turbo"}, getLLMResponse["data"].Attributes.AllowedModels)

	// Test Update LLM
	updateLLMInput := LLMInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				APIKey           string   `json:"api_key"`
				APIEndpoint      string   `json:"api_endpoint"`
				PrivacyScore     int      `json:"privacy_score"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				LogoURL          string   `json:"logo_url"`
				Vendor           string   `json:"vendor"`
				Active           bool     `json:"active"`
				Filters          []int    `json:"filters"`
				DefaultModel     string   `json:"default_model"`
				AllowedModels    []string `json:"allowed_models"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name             string   `json:"name"`
				APIKey           string   `json:"api_key"`
				APIEndpoint      string   `json:"api_endpoint"`
				PrivacyScore     int      `json:"privacy_score"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				LogoURL          string   `json:"logo_url"`
				Vendor           string   `json:"vendor"`
				Active           bool     `json:"active"`
				Filters          []int    `json:"filters"`
				DefaultModel     string   `json:"default_model"`
				AllowedModels    []string `json:"allowed_models"`
			}{
				Name:             "Updated Test LLM",
				APIKey:           "updated-api-key",
				APIEndpoint:      "https://updated-api.test.com",
				PrivacyScore:     80,
				ShortDescription: "An updated test LLM",
				LongDescription:  "This is an updated test LLM for API testing",
				LogoURL:          "https://updatedtestllm.com/logo.png",
				Vendor:           "Updated Test Vendor",
				Active:           true,
				DefaultModel:     "gpt-4",
				AllowedModels:    []string{"gpt-4.*", "gpt-3.5-turbo", "claude-.*"},
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/llms/%s", llmID), updateLLMInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, []string{"gpt-4.*", "gpt-3.5-turbo", "claude-.*"}, updateResponse["data"].Attributes.AllowedModels)

	// Test List LLMs
	w = performRequest(api.router, "GET", "/api/v1/llms", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Greater(t, len(listResponse["data"]), 0)
	assert.Equal(t, []string{"gpt-4.*", "gpt-3.5-turbo", "claude-.*"}, listResponse["data"][0].Attributes.AllowedModels)

	// Test Search LLMs
	w = performRequest(api.router, "GET", "/api/v1/llms/search?name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Delete LLM
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/llms/%s", llmID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestLLMPrivacyScoreEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)

	// Create some test LLMs with different privacy scores
	llms := []models.LLM{
		{
			Name:          "LLM1",
			APIKey:        "key1",
			APIEndpoint:   "https://api1.com",
			PrivacyScore:  30,
			AllowedModels: []string{"gpt-4"},
		},
		{
			Name:          "LLM2",
			APIKey:        "key2",
			APIEndpoint:   "https://api2.com",
			PrivacyScore:  50,
			AllowedModels: []string{"gpt-4.*", "gpt-3.5-turbo"},
		},
		{
			Name:          "LLM3",
			APIKey:        "key3",
			APIEndpoint:   "https://api3.com",
			PrivacyScore:  70,
			AllowedModels: []string{"claude-.*"},
		},
		{
			Name:          "LLM4",
			APIKey:        "key4",
			APIEndpoint:   "https://api4.com",
			PrivacyScore:  90,
			AllowedModels: nil, // Test empty allowlist
		},
	}

	for _, llm := range llms {
		err := db.Create(&llm).Error
		assert.NoError(t, err)
	}

	// Test GetLLMsByMaxPrivacyScore
	w := performRequest(api.router, "GET", "/api/v1/llms/max-privacy-score?max_score=60", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var maxScoreResponse map[string][]LLMResponse
	err := json.Unmarshal(w.Body.Bytes(), &maxScoreResponse)
	assert.NoError(t, err)
	assert.Len(t, maxScoreResponse["data"], 2)
	assert.ElementsMatch(t, []string{"LLM1", "LLM2"}, []string{maxScoreResponse["data"][0].Attributes.Name, maxScoreResponse["data"][1].Attributes.Name})

	// Test GetLLMsByMinPrivacyScore
	w = performRequest(api.router, "GET", "/api/v1/llms/min-privacy-score?min_score=70", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var minScoreResponse map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &minScoreResponse)
	assert.NoError(t, err)
	assert.Len(t, minScoreResponse["data"], 2)
	assert.ElementsMatch(t, []string{"LLM3", "LLM4"}, []string{minScoreResponse["data"][0].Attributes.Name, minScoreResponse["data"][1].Attributes.Name})

	// Test GetLLMsByPrivacyScoreRange
	w = performRequest(api.router, "GET", "/api/v1/llms/privacy-score-range?min_score=40&max_score=80", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var rangeScoreResponse map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &rangeScoreResponse)
	assert.NoError(t, err)
	assert.Len(t, rangeScoreResponse["data"], 2)
	assert.ElementsMatch(t, []string{"LLM2", "LLM3"}, []string{rangeScoreResponse["data"][0].Attributes.Name, rangeScoreResponse["data"][1].Attributes.Name})

	// Test invalid input for GetLLMsByMaxPrivacyScore
	w = performRequest(api.router, "GET", "/api/v1/llms/max-privacy-score?max_score=invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test invalid input for GetLLMsByMinPrivacyScore
	w = performRequest(api.router, "GET", "/api/v1/llms/min-privacy-score?min_score=invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test invalid input for GetLLMsByPrivacyScoreRange
	w = performRequest(api.router, "GET", "/api/v1/llms/privacy-score-range?min_score=80&max_score=70", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
