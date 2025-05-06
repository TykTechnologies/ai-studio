package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	gotest "testing"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestChatEndpoints(t *gotest.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	licenser := apitest.SetupTestLicenser()
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, licenser)

	// Create test user
	user := &models.User{
		Email:         "test@test.com",
		Name:          "Test User",
		IsAdmin:       true,
		EmailVerified: true,
		ShowPortal:    true,
		ShowChat:      true,
	}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create default group
	defaultGroup := &models.Group{
		Name: "Default",
	}
	err = defaultGroup.Create(db)
	assert.NoError(t, err)

	// Add user to default group
	err = service.AddUserToGroup(user.ID, defaultGroup.ID)
	assert.NoError(t, err)

	// Create test group
	group, err := service.CreateGroup("Test Group")
	assert.NoError(t, err)

	// Create LLM settings
	llmSettings, err := service.CreateLLMSettings(&models.LLMSettings{
		ModelName:   "claude-3-sonnet-20240229",
		MaxTokens:   4000,
		Temperature: 0.7,
	})
	assert.NoError(t, err)

	// Create LLM
	llm, err := service.CreateLLM(
		"Default Anthropic", "api-key", "https://api.anthropic.com", 75,
		"Short desc", "Long desc", "http://logo.test",
		"anthropic", true, nil, "", []string{}, nil, nil)
	assert.NoError(t, err)

	// Create default chat
	defaultChat := &models.Chat{
		Name:          "Default Chat",
		Groups:        []models.Group{*defaultGroup, *group},
		SupportsTools: true,
		SystemPrompt:  "You are a helpful assistant.",
		LLMSettingsID: llmSettings.ID,
		LLMID:         llm.ID,
	}
	err = defaultChat.Create(db)
	assert.NoError(t, err)

	// Test Create Chat
	createChatInput := api.ChatInput{
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

	w := apitest.PerformRequest(a.Router(), "POST", "/api/v1/chats", createChatInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]api.ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Chat", response["data"].Attributes.Name)
	assert.Equal(t, "Test Chat Description", response["data"].Attributes.Description)

	chatID := response["data"].ID

	// Test Get Chat
	w = apitest.PerformRequest(a.Router(), "GET", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Chat
	updateChatInput := api.ChatInput{
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

	w = apitest.PerformRequest(a.Router(), "PATCH", fmt.Sprintf("/api/v1/chats/%s", chatID), updateChatInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Chats
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/chats", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]api.ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	// Verify the updated chat is in the response
	found := false
	for _, chat := range listResponse["data"] {
		if chat.Attributes.Name == "Updated Chat" {
			found = true
			assert.Equal(t, "Updated Chat Description", chat.Attributes.Description)
			assert.Equal(t, strconv.FormatUint(uint64(llmSettings.ID), 10), chat.Attributes.LLMSettingsID)
			assert.Equal(t, strconv.FormatUint(uint64(llm.ID), 10), chat.Attributes.LLMID)
			assert.Len(t, chat.Attributes.Groups, 1)
			groupID, err := strconv.ParseUint(chat.Attributes.Groups[0].ID, 10, 32)
			assert.NoError(t, err)
			assert.Equal(t, group.ID, uint(groupID))
			break
		}
	}
	assert.True(t, found, "Updated test chat should be in the response")
	assert.Len(t, listResponse["data"], 2) // 1 test chat + 1 default chat

	// Test Get Chats by Group ID
	w = apitest.PerformRequest(a.Router(), "GET", fmt.Sprintf("/api/v1/chats/by-group?group_id=%d", group.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var groupChatsResponse map[string][]api.ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &groupChatsResponse)
	assert.NoError(t, err)
	// Verify the updated chat is in the response
	found = false
	for _, chat := range groupChatsResponse["data"] {
		if chat.Attributes.Name == "Updated Chat" {
			found = true
			assert.Equal(t, "Updated Chat Description", chat.Attributes.Description)
			assert.Equal(t, strconv.FormatUint(uint64(llmSettings.ID), 10), chat.Attributes.LLMSettingsID)
			assert.Equal(t, strconv.FormatUint(uint64(llm.ID), 10), chat.Attributes.LLMID)
			assert.Len(t, chat.Attributes.Groups, 1)
			groupID, err := strconv.ParseUint(chat.Attributes.Groups[0].ID, 10, 32)
			assert.NoError(t, err)
			assert.Equal(t, group.ID, uint(groupID))
			break
		}
	}
	assert.True(t, found, "Updated test chat should be in the response")
	assert.Len(t, groupChatsResponse["data"], 2) // 1 test chat + 1 default chat

	// Test Delete Chat
	w = apitest.PerformRequest(a.Router(), "DELETE", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify chat is deleted
	w = apitest.PerformRequest(a.Router(), "GET", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChatEndpointsErrors(t *gotest.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	licenser := apitest.SetupTestLicenser()
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, licenser)

	// Test Get non-existent chat
	w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/chats/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent chat
	updateChatInput := api.ChatInput{
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
	w = apitest.PerformRequest(a.Router(), "PATCH", "/api/v1/chats/999", updateChatInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent chat
	w = apitest.PerformRequest(a.Router(), "DELETE", "/api/v1/chats/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Create chat with invalid input
	invalidCreateChatInput := api.ChatInput{
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
	w = apitest.PerformRequest(a.Router(), "POST", "/api/v1/chats", invalidCreateChatInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get chats by non-existent group
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/chats/by-group?group_id=999", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]api.ChatResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)
}
