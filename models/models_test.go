package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFixLLMChatRecordIDs(t *testing.T) {
	t.Skip("Skipping as per commented fixLLMChatRecordIDs")
	// Create an in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	assert.NoError(t, err)

	// Create the table
	err = db.AutoMigrate(&LLMChatRecord{})
	assert.NoError(t, err)

	// Insert test records
	testRecords := []LLMChatRecord{
		{LLMID: 5, InteractionType: InteractionType("old_type"), Name: "old_name", Currency: "EUR"},
		{LLMID: 5, InteractionType: InteractionType("another_type"), Name: "another_name", Currency: "GBP"},
		{LLMID: 6, InteractionType: InteractionType("different_type"), Name: "different_name", Currency: "JPY"}, // Should not be updated
	}
	err = db.Create(&testRecords).Error
	assert.NoError(t, err)

	// Run the fix function
	//err = fixLLMChatRecordIDs(db)
	//assert.NoError(t, err)

	// Verify records with LLMID = 5 were updated correctly
	var updatedRecords []LLMChatRecord
	err = db.Where("llm_id = ?", 5).Find(&updatedRecords).Error
	assert.NoError(t, err)
	for _, record := range updatedRecords {
		assert.Equal(t, ProxyInteraction, record.InteractionType)
		assert.Equal(t, "claude-3-5-sonnet-20241022", record.Name)
		assert.Equal(t, "USD", record.Currency)
	}

	// Verify record with LLMID = 6 was not updated
	var unchangedRecord LLMChatRecord
	err = db.Where("llm_id = ?", 6).First(&unchangedRecord).Error
	assert.NoError(t, err)
	assert.Equal(t, InteractionType("different_type"), unchangedRecord.InteractionType)
	assert.Equal(t, "different_name", unchangedRecord.Name)
	assert.Equal(t, "JPY", unchangedRecord.Currency)
}
