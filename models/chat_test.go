package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChat_Create(t *testing.T) {
	db := setupTestDB(t)

	chat := &Chat{
		Name:          "Test Chat",
		LLMSettingsID: 1,
		LLMID:         1,
	}

	err := chat.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, chat.ID)
}

func TestChat_Get(t *testing.T) {
	db := setupTestDB(t)

	// Create a chat
	chat := &Chat{
		Name:          "Test Chat",
		LLMSettingsID: 1,
		LLMID:         1,
	}
	err := chat.Create(db)
	assert.NoError(t, err)

	// Retrieve the chat
	retrievedChat := &Chat{}
	err = retrievedChat.Get(db, chat.ID)
	assert.NoError(t, err)
	assert.Equal(t, chat.ID, retrievedChat.ID)
	assert.Equal(t, chat.Name, retrievedChat.Name)
}

func TestChat_Update(t *testing.T) {
	db := setupTestDB(t)

	// Create a chat
	chat := &Chat{
		Name:          "Test Chat",
		LLMSettingsID: 1,
		LLMID:         1,
	}
	err := chat.Create(db)
	assert.NoError(t, err)

	// Update the chat
	chat.Name = "Updated Chat"
	err = chat.Update(db)
	assert.NoError(t, err)

	// Retrieve the chat to verify the update
	retrievedChat := &Chat{}
	err = retrievedChat.Get(db, chat.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Chat", retrievedChat.Name)
}

func TestChat_Delete(t *testing.T) {
	db := setupTestDB(t)

	// Create a chat
	chat := &Chat{
		Name:          "Test Chat",
		LLMSettingsID: 1,
		LLMID:         1,
	}
	err := chat.Create(db)
	assert.NoError(t, err)

	// Delete the chat
	err = chat.Delete(db)
	assert.NoError(t, err)

	// Try to retrieve the deleted chat
	retrievedChat := &Chat{}
	err = retrievedChat.Get(db, chat.ID)
	assert.Error(t, err) // Should return an error as the chat is deleted
}

func TestChats_List(t *testing.T) {
	db := setupTestDB(t)

	// Create multiple chats
	chats := []Chat{
		{Name: "Chat 1", LLMSettingsID: 1, LLMID: 1},
		{Name: "Chat 2", LLMSettingsID: 2, LLMID: 2},
		{Name: "Chat 3", LLMSettingsID: 3, LLMID: 3},
	}
	for _, c := range chats {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// List all chats
	var retrievedChats Chats
	_, _, err := retrievedChats.List(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, retrievedChats, 3)
}

func TestChats_GetByGroupID(t *testing.T) {
	db := setupTestDB(t)

	// Create a group
	group := &Group{Name: "Test Group"}
	err := db.Create(group).Error
	assert.NoError(t, err)

	// Create chats and associate them with the group
	chats := []Chat{
		{Name: "Chat 1", LLMSettingsID: 1, LLMID: 1, Groups: []Group{*group}},
		{Name: "Chat 2", LLMSettingsID: 2, LLMID: 2, Groups: []Group{*group}},
		{Name: "Chat 3", LLMSettingsID: 3, LLMID: 3},
	}
	for _, c := range chats {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Get chats by group ID
	var retrievedChats Chats
	err = retrievedChats.GetByGroupID(db, group.ID)
	assert.NoError(t, err)
	assert.Len(t, retrievedChats, 2)
}

func TestChats_GetByLLMID(t *testing.T) {
	db := setupTestDB(t)

	// Create an LLM
	llm := &LLM{Name: "Test LLM"}
	err := db.Create(llm).Error
	assert.NoError(t, err)

	// Create chats and associate them with the LLM
	chats := []Chat{
		{Name: "Chat 1", LLMSettingsID: 1, LLMID: llm.ID},
		{Name: "Chat 2", LLMSettingsID: 2, LLMID: llm.ID},
		{Name: "Chat 3", LLMSettingsID: 3, LLMID: 999}, // Different LLM
	}
	for _, c := range chats {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Get chats by LLM ID
	var retrievedChats Chats
	err = retrievedChats.GetByLLMID(db, llm.ID)
	assert.NoError(t, err)
	assert.Len(t, retrievedChats, 2)
}

func TestChats_GetChatCount(t *testing.T) {
	db := setupTestDB(t)

	// Ensure we start with an empty database
	var initialChats Chats
	initialCount, err := initialChats.GetChatCount(db)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), initialCount, "Database should be empty at start")

	// Create a specific number of chats
	expectedCount := 3
	chats := []Chat{
		{Name: "Chat 1", LLMSettingsID: 1, LLMID: 1},
		{Name: "Chat 2", LLMSettingsID: 2, LLMID: 2},
		{Name: "Chat 3", LLMSettingsID: 3, LLMID: 3},
	}
	for _, c := range chats {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Get the count and verify it matches our expectation
	var testChats Chats
	count, err := testChats.GetChatCount(db)
	assert.NoError(t, err)
	assert.Equal(t, int64(expectedCount), count)
}

func TestChats_GetByLLMSettingsID(t *testing.T) {
	db := setupTestDB(t)

	// Create an LLMSettings
	llmSettings := &LLMSettings{}
	err := db.Create(llmSettings).Error
	assert.NoError(t, err)

	// Create chats and associate them with the LLMSettings
	chats := []Chat{
		{Name: "Chat 1", LLMSettingsID: llmSettings.ID, LLMID: 1},
		{Name: "Chat 2", LLMSettingsID: llmSettings.ID, LLMID: 2},
		{Name: "Chat 3", LLMSettingsID: 999, LLMID: 3}, // Different LLMSettings
	}
	for _, c := range chats {
		err := db.Create(&c).Error
		assert.NoError(t, err)
	}

	// Get chats by LLMSettings ID
	var retrievedChats Chats
	err = retrievedChats.GetByLLMSettingsID(db, llmSettings.ID)
	assert.NoError(t, err)
	assert.Len(t, retrievedChats, 2)
}
