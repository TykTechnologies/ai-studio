package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupHistoryTest(t *testing.T) (*Service, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := NewService(db)
	return service, db
}

func TestCreateChatHistoryRecord(t *testing.T) {
	service, _ := setupHistoryTest(t)

	record, err := service.CreateChatHistoryRecord("session-123", 1, 42, "Test Chat")
	assert.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, "session-123", record.SessionID)
	assert.Equal(t, uint(1), record.ChatID)
	assert.Equal(t, uint(42), record.UserID)
	assert.Equal(t, "Test Chat", record.Name)
	assert.NotZero(t, record.ID)
}

func TestGetChatHistoryRecordByID(t *testing.T) {
	service, _ := setupHistoryTest(t)

	// Create record
	created, err := service.CreateChatHistoryRecord("session-456", 2, 43, "Another Chat")
	assert.NoError(t, err)

	t.Run("Get existing record", func(t *testing.T) {
		retrieved, err := service.GetChatHistoryRecordByID(created.ID)
		assert.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, "session-456", retrieved.SessionID)
	})

	t.Run("Get non-existent record", func(t *testing.T) {
		retrieved, err := service.GetChatHistoryRecordByID(99999)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestUpdateChatHistoryRecord(t *testing.T) {
	service, _ := setupHistoryTest(t)

	// Create initial record
	record, err := service.CreateChatHistoryRecord("original-session", 1, 100, "Original Name")
	assert.NoError(t, err)

	// Update it
	updated, err := service.UpdateChatHistoryRecord(record.ID, "updated-session", 2, 101, "Updated Name")
	assert.NoError(t, err)
	assert.Equal(t, "updated-session", updated.SessionID)
	assert.Equal(t, uint(2), updated.ChatID)
	assert.Equal(t, uint(101), updated.UserID)
	assert.Equal(t, "Updated Name", updated.Name)
}

func TestDeleteChatHistoryRecord(t *testing.T) {
	service, _ := setupHistoryTest(t)

	// Create record
	record, err := service.CreateChatHistoryRecord("delete-session", 1, 200, "To Delete")
	assert.NoError(t, err)

	t.Run("Delete existing record", func(t *testing.T) {
		err := service.DeleteChatHistoryRecord(record.ID)
		assert.NoError(t, err)

		// Verify it's deleted
		_, err = service.GetChatHistoryRecordByID(record.ID)
		assert.Error(t, err)
	})

	t.Run("Delete non-existent record", func(t *testing.T) {
		err := service.DeleteChatHistoryRecord(99999)
		assert.Error(t, err)
	})
}

func TestGetChatHistoryRecordBySessionID(t *testing.T) {
	service, _ := setupHistoryTest(t)

	// Create record
	created, err := service.CreateChatHistoryRecord("unique-session-id", 1, 300, "Session Test")
	assert.NoError(t, err)

	t.Run("Get by existing session ID", func(t *testing.T) {
		retrieved, err := service.GetChatHistoryRecordBySessionID("unique-session-id")
		assert.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
	})

	t.Run("Get by non-existent session ID", func(t *testing.T) {
		retrieved, err := service.GetChatHistoryRecordBySessionID("non-existent")
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestGetChatHistoryRecordByChatID(t *testing.T) {
	service, _ := setupHistoryTest(t)

	// Create record
	created, err := service.CreateChatHistoryRecord("chat-id-session", 999, 400, "Chat ID Test")
	assert.NoError(t, err)

	t.Run("Get by existing chat ID", func(t *testing.T) {
		retrieved, err := service.GetChatHistoryRecordByChatID(999)
		assert.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
	})

	t.Run("Get by non-existent chat ID", func(t *testing.T) {
		retrieved, err := service.GetChatHistoryRecordByChatID(88888)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestListChatHistoryRecordsByUserID(t *testing.T) {
	userID := uint(500)

	t.Run("List with pagination", func(t *testing.T) {
		service, db := setupHistoryTest(t)

		// Create records with messages (function requires >1 message per session)
		for i := 1; i <= 5; i++ {
			sessionID := "session-" + string(rune('0'+i))
			_, err := service.CreateChatHistoryRecord(sessionID, uint(i), userID, "Chat "+string(rune('0'+i)))
			assert.NoError(t, err)

			// Create 2 messages for each session (required by ListChatHistoryRecordsByUserID)
			for j := 1; j <= 2; j++ {
				msg := &models.CMessage{
					Session:   sessionID,
					Content:   []byte("Message"),
					ChatID:    uint(i),
					CreatedAt: time.Now(),
				}
				err = db.Create(msg).Error
				assert.NoError(t, err)
			}
		}

		records, totalCount, totalPages, err := service.ListChatHistoryRecordsByUserID(userID, 2, 1, false)
		assert.NoError(t, err)
		assert.Len(t, records, 2)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
	})

	t.Run("List all for user", func(t *testing.T) {
		service, db := setupHistoryTest(t)

		// Create records with messages
		for i := 1; i <= 5; i++ {
			sessionID := "session-all-" + string(rune('0'+i))
			_, err := service.CreateChatHistoryRecord(sessionID, uint(i), userID, "Chat "+string(rune('0'+i)))
			assert.NoError(t, err)

			// Create 2 messages for each session
			for j := 1; j <= 2; j++ {
				msg := &models.CMessage{
					Session:   sessionID,
					Content:   []byte("Message"),
					ChatID:    uint(i),
					CreatedAt: time.Now(),
				}
				err = db.Create(msg).Error
				assert.NoError(t, err)
			}
		}

		records, totalCount, _, err := service.ListChatHistoryRecordsByUserID(userID, 100, 1, true)
		assert.NoError(t, err)
		assert.Len(t, records, 5)
		assert.Equal(t, int64(5), totalCount)
	})

	t.Run("List for user with no records", func(t *testing.T) {
		service, _ := setupHistoryTest(t)

		records, totalCount, _, err := service.ListChatHistoryRecordsByUserID(99999, 10, 1, false)
		assert.NoError(t, err)
		assert.Len(t, records, 0)
		assert.Equal(t, int64(0), totalCount)
	})
}

func TestSearchChatHistoryRecords(t *testing.T) {
	service, db := setupHistoryTest(t)

	userID := uint(600)

	// Create records with different names
	_, err := service.CreateChatHistoryRecord("search-1", 1, userID, "Project Alpha")
	assert.NoError(t, err)
	_, err = service.CreateChatHistoryRecord("search-2", 2, userID, "Project Beta")
	assert.NoError(t, err)
	_, err = service.CreateChatHistoryRecord("search-3", 3, userID, "Meeting Notes")
	assert.NoError(t, err)

	// Create CMessage records for each session (need >1 per session for SearchChatHistoryRecords)
	for _, sessionID := range []string{"search-1", "search-2", "search-3"} {
		db.Create(&models.CMessage{Session: sessionID, Content: []byte("msg1")})
		db.Create(&models.CMessage{Session: sessionID, Content: []byte("msg2")})
	}

	t.Run("Search finds matching records", func(t *testing.T) {
		results, totalCount, _, err := service.SearchChatHistoryRecords(userID, "Project", 10, 1, false)
		assert.NoError(t, err)
		assert.Len(t, results, 2, "Should find 2 Project chats")
		assert.Equal(t, int64(2), totalCount)
	})

	t.Run("Search with pagination", func(t *testing.T) {
		results, totalCount, totalPages, err := service.SearchChatHistoryRecords(userID, "Project", 1, 1, false)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, int64(2), totalCount)
		assert.Equal(t, 2, totalPages)
	})

	t.Run("Search with no matches", func(t *testing.T) {
		results, totalCount, _, err := service.SearchChatHistoryRecords(userID, "Nonexistent", 10, 1, false)
		assert.NoError(t, err)
		assert.Len(t, results, 0)
		assert.Equal(t, int64(0), totalCount)
	})
}

func TestGetLatestChatHistoryRecord(t *testing.T) {
	userID := uint(700)

	t.Run("Get latest from multiple records", func(t *testing.T) {
		service, _ := setupHistoryTest(t)

		// Create records with slight time delay
		_, err := service.CreateChatHistoryRecord("old-session", 1, userID, "Old Chat")
		assert.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		latest, err := service.CreateChatHistoryRecord("new-session", 2, userID, "New Chat")
		assert.NoError(t, err)

		retrieved, err := service.GetLatestChatHistoryRecord(userID)
		assert.NoError(t, err)
		assert.Equal(t, latest.ID, retrieved.ID)
		assert.Equal(t, "New Chat", retrieved.Name)
	})

	t.Run("Get latest when no records exist", func(t *testing.T) {
		service, _ := setupHistoryTest(t)

		retrieved, err := service.GetLatestChatHistoryRecord(99999)
		assert.Error(t, err)
		// Function returns empty struct, not nil
		if retrieved != nil {
			assert.Zero(t, retrieved.ID)
		}
	})
}

func TestGetLastCMessagesForSession(t *testing.T) {
	service, db := setupHistoryTest(t)

	sessionID := "message-session"

	// Create multiple messages
	for i := 1; i <= 10; i++ {
		msg := &models.CMessage{
			Session:   sessionID,
			Content:   []byte("Message " + string(rune('0'+i))),
			ChatID:    1,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		err := db.Create(msg).Error
		assert.NoError(t, err)
	}

	t.Run("Get last 5 messages", func(t *testing.T) {
		messages, err := service.GetLastCMessagesForSession(sessionID, 5)
		assert.NoError(t, err)
		assert.Len(t, messages, 5)
	})

	t.Run("Get more messages than exist", func(t *testing.T) {
		messages, err := service.GetLastCMessagesForSession(sessionID, 100)
		assert.NoError(t, err)
		assert.Len(t, messages, 10)
	})

	t.Run("Get messages for non-existent session", func(t *testing.T) {
		messages, err := service.GetLastCMessagesForSession("non-existent", 10)
		assert.NoError(t, err)
		assert.Len(t, messages, 0)
	})
}

func TestGetCMessagesForSessionPaginated(t *testing.T) {
	service, db := setupHistoryTest(t)

	sessionID := "paginated-session"

	// Create messages
	for i := 1; i <= 7; i++ {
		msg := &models.CMessage{
			Session:   sessionID,
			Content:   []byte("Msg " + string(rune('0'+i))),
			ChatID:    1,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		err := db.Create(msg).Error
		assert.NoError(t, err)
	}

	t.Run("Get first page", func(t *testing.T) {
		messages, totalCount, totalPages, err := service.GetCMessagesForSessionPaginated(sessionID, 3, 1)
		assert.NoError(t, err)
		assert.Len(t, messages, 3)
		assert.Equal(t, int64(7), totalCount)
		assert.Equal(t, 3, totalPages)
	})

	t.Run("Get second page", func(t *testing.T) {
		messages, totalCount, totalPages, err := service.GetCMessagesForSessionPaginated(sessionID, 3, 2)
		assert.NoError(t, err)
		assert.Len(t, messages, 3)
		assert.Equal(t, int64(7), totalCount)
		assert.Equal(t, 3, totalPages)
	})

	t.Run("Get last partial page", func(t *testing.T) {
		messages, totalCount, totalPages, err := service.GetCMessagesForSessionPaginated(sessionID, 3, 3)
		assert.NoError(t, err)
		assert.Len(t, messages, 1) // Last page has only 1 message
		assert.Equal(t, int64(7), totalCount)
		assert.Equal(t, 3, totalPages)
	})
}

func TestEditUserMessage(t *testing.T) {
	service, db := setupHistoryTest(t)

	sessionID := "edit-session"

	// Create a sequence of messages
	var messageIDs []uint
	for i := 1; i <= 5; i++ {
		msg := &models.CMessage{
			Session:   sessionID,
			Content:   []byte("Message " + string(rune('0'+i))),
			ChatID:    1,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		err := db.Create(msg).Error
		assert.NoError(t, err)
		messageIDs = append(messageIDs, msg.ID)
	}

	t.Run("Edit message removes it and subsequent messages", func(t *testing.T) {
		// Edit the 3rd message (index 2)
		err := service.EditUserMessage(sessionID, messageIDs[2], "new content")
		assert.NoError(t, err)

		// Check that messages 3, 4, 5 are gone
		var remaining []models.CMessage
		err = db.Where("session = ?", sessionID).Find(&remaining).Error
		assert.NoError(t, err)
		assert.Len(t, remaining, 2, "Should have only first 2 messages left")
	})

	t.Run("Edit non-existent message fails", func(t *testing.T) {
		err := service.EditUserMessage(sessionID, 99999, "new content")
		assert.Error(t, err)
	})

	t.Run("Edit message with wrong session fails", func(t *testing.T) {
		// Create a message in different session
		msg := &models.CMessage{
			Session:   "other-session",
			Content:   []byte("Other"),
			ChatID:    1,
			CreatedAt: time.Now(),
		}
		err := db.Create(msg).Error
		assert.NoError(t, err)

		// Try to edit it with wrong session ID
		err = service.EditUserMessage("wrong-session", msg.ID, "new content")
		assert.Error(t, err)
	})
}

func TestEditUserMessageByIndex(t *testing.T) {
	service, db := setupHistoryTest(t)

	sessionID := "index-edit-session"

	// Create messages
	for i := 0; i < 5; i++ {
		msg := &models.CMessage{
			Session:   sessionID,
			Content:   []byte("Message " + string(rune('0'+i))),
			ChatID:    1,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		err := db.Create(msg).Error
		assert.NoError(t, err)
	}

	t.Run("Edit by index removes from that point onwards", func(t *testing.T) {
		// Edit from index 2 (3rd message)
		err := service.EditUserMessageByIndex(sessionID, 2)
		assert.NoError(t, err)

		// Should have only 2 messages left (indices 0 and 1)
		var remaining []models.CMessage
		err = db.Where("session = ?", sessionID).Find(&remaining).Error
		assert.NoError(t, err)
		assert.Len(t, remaining, 2)
	})

	t.Run("Edit with out-of-range index fails", func(t *testing.T) {
		err := service.EditUserMessageByIndex(sessionID, 100)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "out of range")
	})

	// Note: Negative index causes panic, not error - function doesn't validate negative indices
	// This is current behavior, not tested here to avoid panic in test suite
}
