package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMCPSessionTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// Auto migrate the schema
	db.AutoMigrate(&MCPSession{}, &MCPServer{})

	return db
}

func createTestMCPSession(t *testing.T, db *gorm.DB) *MCPSession {
	// Create a test server first
	server := &MCPServer{
		UserID:      1,
		Name:        "Test Server",
		Description: "Test Server for Session Tests",
		Status:      "running",
	}
	err := db.Create(server).Error
	assert.NoError(t, err)

	// Create a session
	session := &MCPSession{
		MCPServerID: server.ID,
		UserID:      1,
		SessionID:   "test-session-id",
		ClientID:    "test-client-id",
		LastSeen:    time.Now(),
		Active:      true,
	}
	err = session.Create(db)
	assert.NoError(t, err)

	return session
}

func TestMCPSessionCreate(t *testing.T) {
	db := setupMCPSessionTestDB(t)

	// Create a test server
	server := &MCPServer{
		UserID:      1,
		Name:        "Test Server",
		Description: "Test Server Description",
		Status:      "running",
	}
	err := db.Create(server).Error
	assert.NoError(t, err)

	// Create a session
	session := &MCPSession{
		MCPServerID: server.ID,
		UserID:      1,
		SessionID:   "test-session-id",
		ClientID:    "test-client-id",
		LastSeen:    time.Now(),
		Active:      true,
	}
	err = session.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, session.ID)

	// Verify the session was created in the database
	var retrievedSession MCPSession
	result := db.First(&retrievedSession, session.ID)
	assert.NoError(t, result.Error)
	assert.Equal(t, session.SessionID, retrievedSession.SessionID)
	assert.Equal(t, session.ClientID, retrievedSession.ClientID)
	assert.Equal(t, session.UserID, retrievedSession.UserID)
	assert.Equal(t, session.MCPServerID, retrievedSession.MCPServerID)
	assert.True(t, retrievedSession.Active)
}

func TestMCPSessionUpdate(t *testing.T) {
	db := setupMCPSessionTestDB(t)
	session := createTestMCPSession(t, db)

	// Update the session
	originalLastSeen := session.LastSeen
	session.Active = false
	session.LastSeen = time.Now().Add(time.Hour)
	err := session.Update(db)
	assert.NoError(t, err)

	// Verify the update
	var retrievedSession MCPSession
	err = retrievedSession.Get(db, session.ID)
	assert.NoError(t, err)
	assert.False(t, retrievedSession.Active)
	assert.True(t, retrievedSession.LastSeen.After(originalLastSeen))
}

func TestMCPSessionDelete(t *testing.T) {
	db := setupMCPSessionTestDB(t)
	session := createTestMCPSession(t, db)

	// Delete the session
	err := session.Delete(db)
	assert.NoError(t, err)

	// Verify the deletion
	var retrievedSession MCPSession
	err = retrievedSession.Get(db, session.ID)
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestMCPSessionGet(t *testing.T) {
	db := setupMCPSessionTestDB(t)
	originalSession := createTestMCPSession(t, db)

	// Get the session by ID
	var retrievedSession MCPSession
	err := retrievedSession.Get(db, originalSession.ID)
	assert.NoError(t, err)
	assert.Equal(t, originalSession.ID, retrievedSession.ID)
	assert.Equal(t, originalSession.SessionID, retrievedSession.SessionID)
	assert.Equal(t, originalSession.ClientID, retrievedSession.ClientID)

	// Try to get a non-existent session
	var notFoundSession MCPSession
	err = notFoundSession.Get(db, 9999)
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestMCPSessionGetBySessionID(t *testing.T) {
	db := setupMCPSessionTestDB(t)
	originalSession := createTestMCPSession(t, db)

	// Get the session by session ID
	var retrievedSession MCPSession
	err := retrievedSession.GetBySessionID(db, originalSession.SessionID)
	assert.NoError(t, err)
	assert.Equal(t, originalSession.ID, retrievedSession.ID)
	assert.Equal(t, originalSession.SessionID, retrievedSession.SessionID)
	assert.Equal(t, originalSession.ClientID, retrievedSession.ClientID)

	// Try to get a non-existent session ID
	var notFoundSession MCPSession
	err = notFoundSession.GetBySessionID(db, "non-existent-session-id")
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestGetSessionsByUserAndServer(t *testing.T) {
	db := setupMCPSessionTestDB(t)

	// Create a test server
	server := &MCPServer{
		UserID:      1,
		Name:        "Test Server",
		Description: "Test Server Description",
		Status:      "running",
	}
	err := db.Create(server).Error
	assert.NoError(t, err)

	// Create multiple sessions for the same user and server
	session1 := &MCPSession{
		MCPServerID: server.ID,
		UserID:      1,
		SessionID:   "session-1",
		ClientID:    "client-1",
		LastSeen:    time.Now(),
		Active:      true,
	}
	err = session1.Create(db)
	assert.NoError(t, err)

	session2 := &MCPSession{
		MCPServerID: server.ID,
		UserID:      1,
		SessionID:   "session-2",
		ClientID:    "client-2",
		LastSeen:    time.Now(),
		Active:      true,
	}
	err = session2.Create(db)
	assert.NoError(t, err)

	// Create a session for a different user with the same server
	session3 := &MCPSession{
		MCPServerID: server.ID,
		UserID:      2,
		SessionID:   "session-3",
		ClientID:    "client-3",
		LastSeen:    time.Now(),
		Active:      true,
	}
	err = session3.Create(db)
	assert.NoError(t, err)

	// Get sessions for user 1 and the server
	sessions, err := GetSessionsByUserAndServer(db, 1, server.ID)
	assert.NoError(t, err)
	assert.Len(t, sessions, 2)

	// Get sessions for user 2 and the server
	sessions, err = GetSessionsByUserAndServer(db, 2, server.ID)
	assert.NoError(t, err)
	assert.Len(t, sessions, 1)

	// Get sessions for non-existent user
	sessions, err = GetSessionsByUserAndServer(db, 999, server.ID)
	assert.NoError(t, err)
	assert.Len(t, sessions, 0)
}

func TestGetActiveSessionsByServer(t *testing.T) {
	db := setupMCPSessionTestDB(t)

	// Create a test server
	server := &MCPServer{
		UserID:      1,
		Name:        "Test Server",
		Description: "Test Server Description",
		Status:      "running",
	}
	err := db.Create(server).Error
	assert.NoError(t, err)

	// Create active sessions
	session1 := &MCPSession{
		MCPServerID: server.ID,
		UserID:      1,
		SessionID:   "active-session-1",
		ClientID:    "client-1",
		LastSeen:    time.Now(),
		Active:      true,
	}
	err = session1.Create(db)
	assert.NoError(t, err)

	session2 := &MCPSession{
		MCPServerID: server.ID,
		UserID:      2,
		SessionID:   "active-session-2",
		ClientID:    "client-2",
		LastSeen:    time.Now(),
		Active:      true,
	}
	err = session2.Create(db)
	assert.NoError(t, err)

	// Create inactive session
	session3 := &MCPSession{
		MCPServerID: server.ID,
		UserID:      3,
		SessionID:   "inactive-session",
		ClientID:    "client-3",
		LastSeen:    time.Now(),
		Active:      false,
	}
	err = session3.Create(db)
	assert.NoError(t, err)

	// Get active sessions for the server
	sessions, err := GetActiveSessionsByServer(db, server.ID)
	assert.NoError(t, err)
	assert.Len(t, sessions, 2)

	// Verify all returned sessions are active
	for _, s := range sessions {
		assert.True(t, s.Active)
	}
}

func TestMCPSessionMarkInactive(t *testing.T) {
	db := setupMCPSessionTestDB(t)
	session := createTestMCPSession(t, db)
	assert.True(t, session.Active)

	// Mark the session as inactive
	err := session.MarkInactive(db)
	assert.NoError(t, err)

	// Verify the session is now inactive
	var retrievedSession MCPSession
	err = retrievedSession.Get(db, session.ID)
	assert.NoError(t, err)
	assert.False(t, retrievedSession.Active)
}

func TestMCPSessionUpdateLastSeen(t *testing.T) {
	db := setupMCPSessionTestDB(t)
	session := createTestMCPSession(t, db)
	originalLastSeen := session.LastSeen

	// Wait a moment to ensure timestamps differ
	time.Sleep(10 * time.Millisecond)

	// Update last seen
	err := session.UpdateLastSeen(db)
	assert.NoError(t, err)

	// Verify last seen was updated
	var retrievedSession MCPSession
	err = retrievedSession.Get(db, session.ID)
	assert.NoError(t, err)
	assert.True(t, retrievedSession.LastSeen.After(originalLastSeen))
}
