package models

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestChatHistoryRecordCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Test Create
	record := &ChatHistoryRecord{
		SessionID: "test-session",
		ChatID:    1,
		UserID:    1,
		Name:      "Test Chat",
	}
	err := record.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, record.ID)

	// Test Get
	fetchedRecord := &ChatHistoryRecord{}
	err = fetchedRecord.Get(db, record.ID)
	assert.NoError(t, err)
	assert.Equal(t, record.SessionID, fetchedRecord.SessionID)

	// Test Update
	record.Name = "Updated Test Chat"
	err = record.Update(db)
	assert.NoError(t, err)

	err = fetchedRecord.Get(db, record.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Test Chat", fetchedRecord.Name)

	// Test Delete
	err = record.Delete(db)
	assert.NoError(t, err)

	err = fetchedRecord.Get(db, record.ID)
	assert.Error(t, err) // Should not find the deleted record
}

func TestGetBySessionID(t *testing.T) {
	db := setupTestDB(t)

	record := &ChatHistoryRecord{
		SessionID: "unique-session",
		ChatID:    1,
		UserID:    1,
		Name:      "Test Chat",
	}
	err := record.Create(db)
	assert.NoError(t, err)

	fetchedRecord := &ChatHistoryRecord{}
	err = fetchedRecord.GetBySessionID(db, "unique-session")
	assert.NoError(t, err)
	assert.Equal(t, record.ID, fetchedRecord.ID)
}

func TestGetByChatID(t *testing.T) {
	db := setupTestDB(t)

	record := &ChatHistoryRecord{
		SessionID: "test-session",
		ChatID:    42,
		UserID:    1,
		Name:      "Test Chat",
	}
	err := record.Create(db)
	assert.NoError(t, err)

	fetchedRecord := &ChatHistoryRecord{}
	err = fetchedRecord.GetByChatID(db, 42)
	assert.NoError(t, err)
	assert.Equal(t, record.ID, fetchedRecord.ID)
}

func TestListChatHistoryRecordsByUserID(t *testing.T) {
	db := setupTestDB(t)

	// Create some test records
	for i := 1; i <= 5; i++ {
		record := &ChatHistoryRecord{
			SessionID: "session-" + strconv.Itoa(i),
			ChatID:    uint(i),
			UserID:    1,
			Name:      "Test Chat " + strconv.Itoa(i),
		}
		err := record.Create(db)
		assert.NoError(t, err)
	}

	records, err := ListChatHistoryRecordsByUserID(db, 1)
	assert.NoError(t, err)
	assert.Len(t, records, 5)
}

func TestListChatHistoryRecordsByUserIDPaginated(t *testing.T) {
	db := setupTestDB(t)

	// Create some test records
	for i := 1; i <= 15; i++ {
		record := &ChatHistoryRecord{
			SessionID: "session-" + strconv.Itoa(i),
			ChatID:    uint(i),
			UserID:    1,
			Name:      "Test Chat " + strconv.Itoa(i),
		}
		err := record.Create(db)
		assert.NoError(t, err)
	}

	records, total, err := ListChatHistoryRecordsByUserIDPaginated(db, 1, 2, 5)
	assert.NoError(t, err)
	assert.Len(t, records, 5)
	assert.Equal(t, int64(15), total)
}

func TestSearchChatHistoryRecords(t *testing.T) {
	db := setupTestDB(t)

	// Create some test records
	records := []ChatHistoryRecord{
		{SessionID: "s1", ChatID: 1, UserID: 1, Name: "Alpha Chat"},
		{SessionID: "s2", ChatID: 2, UserID: 1, Name: "Beta Chat"},
		{SessionID: "s3", ChatID: 3, UserID: 1, Name: "Gamma Chat"},
	}

	for _, r := range records {
		err := r.Create(db)
		assert.NoError(t, err)
	}

	results, err := SearchChatHistoryRecords(db, 1, "Beta")
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Beta Chat", results[0].Name)
}

func TestGetLatestChatHistoryRecord(t *testing.T) {
	db := setupTestDB(t)

	// Create some test records with different creation times
	records := []ChatHistoryRecord{
		{SessionID: "s1", ChatID: 1, UserID: 1, Name: "Old Chat"},
		{SessionID: "s2", ChatID: 2, UserID: 1, Name: "Recent Chat"},
	}

	for i, r := range records {
		err := r.Create(db)
		assert.NoError(t, err)
		if i == 0 {
			time.Sleep(time.Millisecond * 10) // Ensure different creation times
		}
	}

	latest, err := GetLatestChatHistoryRecord(db, 1)
	assert.NoError(t, err)
	assert.NotNil(t, latest)
	assert.Equal(t, "Recent Chat", latest.Name)
}
