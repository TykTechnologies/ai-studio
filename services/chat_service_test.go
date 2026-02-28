package services

import (
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	_ "github.com/TykTechnologies/midsommar/v2/secrets/all"
	"github.com/stretchr/testify/assert"
)

func TestChatService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create test data
	llmSettings := &models.LLMSettings{}
	err := db.Create(llmSettings).Error
	assert.NoError(t, err)

	llm := &models.LLM{Name: "Test LLM"}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	group1 := &models.Group{Name: "Group 1"}
	group2 := &models.Group{Name: "Group 2"}
	err = db.Create(group1).Error
	assert.NoError(t, err)
	err = db.Create(group2).Error
	assert.NoError(t, err)

	// Test CreateChat
	chat, err := service.CreateChat("Test Chat", "Test Chat Description", llmSettings.ID, llm.ID, []uint{group1.ID, group2.ID}, nil, 5, true, "", 0, []uint{})
	assert.NoError(t, err)
	assert.NotNil(t, chat)
	assert.Equal(t, "Test Chat", chat.Name)
	assert.Equal(t, "Test Chat Description", chat.Description)
	assert.Equal(t, llmSettings.ID, chat.LLMSettingsID)
	assert.Equal(t, llm.ID, chat.LLMID)
	assert.Len(t, chat.Groups, 2)

	// Test GetChatByID
	retrievedChat, err := service.GetChatByID(chat.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedChat)
	assert.Equal(t, chat.ID, retrievedChat.ID)
	assert.Equal(t, chat.Name, retrievedChat.Name)
	assert.Equal(t, chat.Description, retrievedChat.Description)

	// Test UpdateChat
	updatedChat, err := service.UpdateChat(chat.ID, "Updated Chat", "Updated Chat Description", llmSettings.ID, llm.ID, []uint{group1.ID}, nil, 5, true, "", 0, []uint{})
	assert.NoError(t, err)
	assert.NotNil(t, updatedChat)
	assert.Equal(t, "Updated Chat", updatedChat.Name)
	assert.Equal(t, "Updated Chat Description", updatedChat.Description)
	assert.Len(t, updatedChat.Groups, 1)

	// Test ListChats
	chats, _, _, err := service.ListChats(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, chats, 1)

	// Test GetChatsByGroupID
	groupChats, err := service.GetChatsByGroupID(group1.ID)
	assert.NoError(t, err)
	assert.Len(t, groupChats, 1)

	// Test GetChatsByLLMID
	llmChats, err := service.GetChatsByLLMID(llm.ID)
	assert.NoError(t, err)
	assert.Len(t, llmChats, 1)

	// Test GetChatsByLLMSettingsID
	settingsChats, err := service.GetChatsByLLMSettingsID(llmSettings.ID)
	assert.NoError(t, err)
	assert.Len(t, settingsChats, 1)

	// Test DeleteChat
	err = service.DeleteChat(chat.ID)
	assert.NoError(t, err)

	// Verify deletion
	_, err = service.GetChatByID(chat.ID)
	assert.Error(t, err)
}

// TestGetChatByID_ResolvesSecrets verifies that GetChatByID properly resolves
// secret references in LLM credentials (APIKey and APIEndpoint).
// This is a regression test for a bug where the SSE handler bypassed secret resolution.
func TestGetChatByID_ResolvesSecrets(t *testing.T) {
	// Set up the secret key environment variable
	originalKey := os.Getenv("TYK_AI_SECRET_KEY")
	os.Setenv("TYK_AI_SECRET_KEY", "test-secret-key-for-encryption")
	defer os.Setenv("TYK_AI_SECRET_KEY", originalKey)

	db := setupTestDB(t)
	service := NewService(db)

	// Set the secrets DB reference (required for secret resolution)
	secrets.SetDBRef(db)

	// Migrate secrets table
	db.AutoMigrate(&secrets.Secret{})

	// Create test secrets
	apiKeySecret := &secrets.Secret{
		VarName: "TEST_API_KEY",
		Value:   "sk-actual-secret-api-key-12345",
	}
	err := secrets.CreateSecret(db, apiKeySecret)
	assert.NoError(t, err)

	apiEndpointSecret := &secrets.Secret{
		VarName: "TEST_API_ENDPOINT",
		Value:   "https://api.example.com/v1",
	}
	err = secrets.CreateSecret(db, apiEndpointSecret)
	assert.NoError(t, err)

	// Create LLM settings
	llmSettings := &models.LLMSettings{}
	err = db.Create(llmSettings).Error
	assert.NoError(t, err)

	// Create LLM with secret references (not raw values)
	llm := &models.LLM{
		Name:        "Test LLM with Secrets",
		APIKey:      "$SECRET/TEST_API_KEY",
		APIEndpoint: "$SECRET/TEST_API_ENDPOINT",
	}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	// Create a group for the chat
	group := &models.Group{Name: "Test Group"}
	err = db.Create(group).Error
	assert.NoError(t, err)

	// Create chat with the LLM
	chat, err := service.CreateChat(
		"Test Chat with Secrets",
		"Testing secret resolution",
		llmSettings.ID,
		llm.ID,
		[]uint{group.ID},
		nil, 5, true, "", 0, []uint{},
	)
	assert.NoError(t, err)
	assert.NotNil(t, chat)

	// Now retrieve the chat using GetChatByID - this should resolve secrets
	retrievedChat, err := service.GetChatByID(chat.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedChat)
	assert.NotNil(t, retrievedChat.LLM)

	// Verify the APIKey was resolved to the actual secret value
	assert.Equal(t, "sk-actual-secret-api-key-12345", retrievedChat.LLM.APIKey,
		"APIKey should be resolved from $SECRET/TEST_API_KEY to the actual value")

	// Verify the APIEndpoint was resolved to the actual secret value
	assert.Equal(t, "https://api.example.com/v1", retrievedChat.LLM.APIEndpoint,
		"APIEndpoint should be resolved from $SECRET/TEST_API_ENDPOINT to the actual value")

	// Also verify that raw values (non-secret references) pass through unchanged
	llmWithRawKey := &models.LLM{
		Name:        "Test LLM with Raw Key",
		APIKey:      "raw-api-key-no-secret",
		APIEndpoint: "https://direct-endpoint.com",
	}
	err = db.Create(llmWithRawKey).Error
	assert.NoError(t, err)

	chatWithRawKey, err := service.CreateChat(
		"Test Chat with Raw Key",
		"Testing raw key passthrough",
		llmSettings.ID,
		llmWithRawKey.ID,
		[]uint{group.ID},
		nil, 5, true, "", 0, []uint{},
	)
	assert.NoError(t, err)

	retrievedChatWithRawKey, err := service.GetChatByID(chatWithRawKey.ID)
	assert.NoError(t, err)
	assert.Equal(t, "raw-api-key-no-secret", retrievedChatWithRawKey.LLM.APIKey,
		"Raw APIKey should pass through unchanged")
	assert.Equal(t, "https://direct-endpoint.com", retrievedChatWithRawKey.LLM.APIEndpoint,
		"Raw APIEndpoint should pass through unchanged")
}

func TestChatServiceErrors(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test GetChatByID with non-existent ID
	_, err := service.GetChatByID(999)
	assert.Error(t, err)

	// Test UpdateChat with non-existent ID
	_, err = service.UpdateChat(999, "Updated Chat", "Updated Chat Description", 1, 1, []uint{1}, nil, 5, true, "", 0, []uint{})
	assert.Error(t, err)

	// Test DeleteChat with non-existent ID
	err = service.DeleteChat(999)
	assert.Error(t, err)

	// Test CreateChat with non-existent group IDs
	_, err = service.CreateChat("Test Chat", "Test Chat Description", 1, 1, []uint{9999999}, nil, 5, true, "", 0, []uint{})
	assert.Error(t, err)

	// Test GetChatsByGroupID with non-existent group ID
	chats, err := service.GetChatsByGroupID(999)
	assert.NoError(t, err)
	assert.Len(t, chats, 0)

	// Test GetChatsByLLMID with non-existent LLM ID
	chats, err = service.GetChatsByLLMID(999)
	assert.NoError(t, err)
	assert.Len(t, chats, 0)

	// Test GetChatsByLLMSettingsID with non-existent LLMSettings ID
	chats, err = service.GetChatsByLLMSettingsID(999)
	assert.NoError(t, err)
	assert.Len(t, chats, 0)
}
