package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestChatEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create test data
	group, err := api.service.CreateGroup("Test Group")
	assert.NoError(t, err)

	llmSettings, err := api.service.CreateLLMSettings(&models.LLMSettings{ModelName: "TestModel"})
	assert.NoError(t, err)

	llm, err := api.service.CreateLLM(
		"TestLLM", "api-key", "http://api.test", 75,
		"Short desc", "Long desc", "http://logo.test",
		models.OPENAI, true, nil, "", []string{})
	assert.NoError(t, err)

	// Test Create Chat
	createChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			}{
				Name:          "Test Chat",
				Description:   "Test Chat Description",
				LLMSettingsID: llmSettings.ID,
				LLMID:         llm.ID,
				GroupIDs:      []uint{group.ID},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/chats", createChatInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Chat", response["data"].Attributes.Name)
	assert.Equal(t, "Test Chat Description", response["data"].Attributes.Description)

	chatID := response["data"].ID

	// Test Get Chat
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Chat
	updateChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			}{
				Name:          "Updated Chat",
				Description:   "Updated Chat Description",
				LLMSettingsID: llmSettings.ID,
				LLMID:         llm.ID,
				GroupIDs:      []uint{group.ID},
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/chats/%s", chatID), updateChatInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Chats
	w = performRequest(api.router, "GET", "/api/v1/chats", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "Updated Chat", listResponse["data"][0].Attributes.Name)
	assert.Equal(t, "Updated Chat Description", listResponse["data"][0].Attributes.Description)

	// Test Get Chats by Group ID
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chats/by-group?group_id=%d", group.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var groupChatsResponse map[string][]ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &groupChatsResponse)
	assert.NoError(t, err)
	assert.Len(t, groupChatsResponse["data"], 1)
	assert.Equal(t, "Updated Chat", groupChatsResponse["data"][0].Attributes.Name)
	assert.Equal(t, "Updated Chat Description", groupChatsResponse["data"][0].Attributes.Description)

	// Test Delete Chat
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify chat is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChatEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent chat
	w := performRequest(api.router, "GET", "/api/v1/chats/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent chat
	updateChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			}{
				Name:          "Updated Chat",
				Description:   "Updated Chat Description",
				LLMSettingsID: 1,
				LLMID:         1,
				GroupIDs:      []uint{1},
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/chats/999", updateChatInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent chat
	w = performRequest(api.router, "DELETE", "/api/v1/chats/999", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test Create chat with invalid input
	invalidCreateChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name                string `json:"name"`
				Description         string `json:"description"`
				LLMSettingsID       uint   `json:"llm_settings_id"`
				LLMID               uint   `json:"llm_id"`
				GroupIDs            []uint `json:"group_ids"`
				FilterIDs           []uint `json:"filter_ids"`
				RagN                int    `json:"rag_n"`
				ToolSupport         bool   `json:"tool_support"`
				SystemPrompt        string `json:"system_prompt"`
				DefaultDataSourceID int    `json:"default_data_source_id"`
				DefaultToolIDs      []uint `json:"default_tool_ids"`
			}{
				Name:          "",
				Description:   "",
				LLMSettingsID: 0,
				LLMID:         0,
				GroupIDs:      []uint{},
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/chats", invalidCreateChatInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get chats by non-existent group
	w = performRequest(api.router, "GET", "/api/v1/chats/by-group?group_id=999", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]ChatResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)
}
