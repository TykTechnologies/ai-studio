package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
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
	chat, err := service.CreateChat("Test Chat", llmSettings.ID, llm.ID, []uint{group1.ID, group2.ID}, nil, 5, true)
	assert.NoError(t, err)
	assert.NotNil(t, chat)
	assert.Equal(t, "Test Chat", chat.Name)
	assert.Equal(t, llmSettings.ID, chat.LLMSettingsID)
	assert.Equal(t, llm.ID, chat.LLMID)
	assert.Len(t, chat.Groups, 2)

	// Test GetChatByID
	retrievedChat, err := service.GetChatByID(chat.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedChat)
	assert.Equal(t, chat.ID, retrievedChat.ID)
	assert.Equal(t, chat.Name, retrievedChat.Name)

	// Test UpdateChat
	updatedChat, err := service.UpdateChat(chat.ID, "Updated Chat", llmSettings.ID, llm.ID, []uint{group1.ID}, nil, 5, true)
	assert.NoError(t, err)
	assert.NotNil(t, updatedChat)
	assert.Equal(t, "Updated Chat", updatedChat.Name)
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

func TestChatServiceErrors(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test GetChatByID with non-existent ID
	_, err := service.GetChatByID(999)
	assert.Error(t, err)

	// Test UpdateChat with non-existent ID
	_, err = service.UpdateChat(999, "Updated Chat", 1, 1, []uint{1}, nil, 5, true)
	assert.Error(t, err)

	// Test DeleteChat with non-existent ID
	err = service.DeleteChat(999)
	assert.Error(t, err)

	// Test CreateChat with non-existent group IDs
	_, err = service.CreateChat("Test Chat", 1, 1, []uint{9999999}, nil, 5, true)
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
