package models

import (
	"time"

	"gorm.io/gorm"
)

// MCPSession represents a session for an MCP server
type MCPSession struct {
	gorm.Model
	MCPServerID uint      `json:"mcp_server_id"`
	UserID      uint      `json:"user_id"`
	SessionID   string    `json:"session_id"`
	ClientID    string    `json:"client_id"`
	LastSeen    time.Time `json:"last_seen"`
	Active      bool      `json:"active"`
}

// Create creates a new MCP session in the database
func (s *MCPSession) Create(db *gorm.DB) error {
	return db.Create(s).Error
}

// Update updates an existing MCP session in the database
func (s *MCPSession) Update(db *gorm.DB) error {
	return db.Save(s).Error
}

// Delete deletes an MCP session from the database
func (s *MCPSession) Delete(db *gorm.DB) error {
	return db.Delete(s).Error
}

// Get retrieves an MCP session by ID
func (s *MCPSession) Get(db *gorm.DB, id uint) error {
	return db.First(s, id).Error
}

// GetBySessionID retrieves an MCP session by session ID
func (s *MCPSession) GetBySessionID(db *gorm.DB, sessionID string) error {
	return db.Where("session_id = ?", sessionID).First(s).Error
}

// GetByUserAndServer retrieves MCP sessions for a user and server
func GetSessionsByUserAndServer(db *gorm.DB, userID, serverID uint) ([]MCPSession, error) {
	var sessions []MCPSession
	err := db.Where("user_id = ? AND mcp_server_id = ?", userID, serverID).Find(&sessions).Error
	return sessions, err
}

// GetActiveSessionsByServer retrieves active MCP sessions for a server
func GetActiveSessionsByServer(db *gorm.DB, serverID uint) ([]MCPSession, error) {
	var sessions []MCPSession
	err := db.Where("mcp_server_id = ? AND active = ?", serverID, true).Find(&sessions).Error
	return sessions, err
}

// MarkInactive marks a session as inactive
func (s *MCPSession) MarkInactive(db *gorm.DB) error {
	s.Active = false
	return s.Update(db)
}

// UpdateLastSeen updates the last seen timestamp
func (s *MCPSession) UpdateLastSeen(db *gorm.DB) error {
	s.LastSeen = time.Now()
	return s.Update(db)
}
