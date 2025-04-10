package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMCPTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// Auto migrate the schema
	db.AutoMigrate(&MCPServer{}, &Tool{}, &MCPSession{})

	return db
}

func TestNewMCPServer(t *testing.T) {
	server := NewMCPServer()
	assert.NotNil(t, server)
	assert.Equal(t, "stopped", server.Status)
}

func TestMCPServerCreate(t *testing.T) {
	db := setupMCPTestDB(t)

	server := NewMCPServer()
	server.UserID = 1
	server.Name = "Test Server"
	server.Description = "Test Description"

	err := server.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, server.ID)
	assert.Equal(t, "stopped", server.Status)

	// Verify the server was created in the database
	var retrievedServer MCPServer
	result := db.First(&retrievedServer, server.ID)
	assert.NoError(t, result.Error)
	assert.Equal(t, server.Name, retrievedServer.Name)
	assert.Equal(t, server.Description, retrievedServer.Description)
}

func TestMCPServerUpdate(t *testing.T) {
	db := setupMCPTestDB(t)

	// Create a test server
	server := NewMCPServer()
	server.UserID = 1
	server.Name = "Original Name"
	server.Description = "Original Description"
	err := server.Create(db)
	assert.NoError(t, err)

	// Update the server
	server.Name = "Updated Name"
	server.Description = "Updated Description"
	err = server.Update(db)
	assert.NoError(t, err)

	// Verify the update
	var retrievedServer MCPServer
	err = retrievedServer.Get(db, server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Name", retrievedServer.Name)
	assert.Equal(t, "Updated Description", retrievedServer.Description)
}

func TestMCPServerDelete(t *testing.T) {
	db := setupMCPTestDB(t)

	// Create a test server
	server := NewMCPServer()
	server.UserID = 1
	server.Name = "Test Server"
	err := server.Create(db)
	assert.NoError(t, err)

	// Delete the server
	err = server.Delete(db)
	assert.NoError(t, err)

	// Verify the deletion
	var retrievedServer MCPServer
	err = retrievedServer.Get(db, server.ID)
	assert.Error(t, err)
	assert.True(t, gorm.ErrRecordNotFound == err)
}

func TestMCPServerGet(t *testing.T) {
	db := setupMCPTestDB(t)

	// Create a test server
	original := NewMCPServer()
	original.UserID = 1
	original.Name = "Test Server"
	original.Description = "Test Description"
	err := original.Create(db)
	assert.NoError(t, err)

	// Get the server by ID
	var retrieved MCPServer
	err = retrieved.Get(db, original.ID)
	assert.NoError(t, err)
	assert.Equal(t, original.ID, retrieved.ID)
	assert.Equal(t, original.Name, retrieved.Name)
	assert.Equal(t, original.Description, retrieved.Description)

	// Try to get a non-existent server
	var notFound MCPServer
	err = notFound.Get(db, 9999)
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestMCPServersGetByUserID(t *testing.T) {
	db := setupMCPTestDB(t)

	// Create test servers for different users
	server1 := NewMCPServer()
	server1.UserID = 1
	server1.Name = "User 1 Server"
	err := server1.Create(db)
	assert.NoError(t, err)

	server2 := NewMCPServer()
	server2.UserID = 1
	server2.Name = "User 1 Server 2"
	err = server2.Create(db)
	assert.NoError(t, err)

	server3 := NewMCPServer()
	server3.UserID = 2
	server3.Name = "User 2 Server"
	err = server3.Create(db)
	assert.NoError(t, err)

	// Get servers for user 1
	var user1Servers MCPServers
	err = user1Servers.GetByUserID(db, 1)
	assert.NoError(t, err)
	assert.Len(t, user1Servers, 2)

	// Get servers for user 2
	var user2Servers MCPServers
	err = user2Servers.GetByUserID(db, 2)
	assert.NoError(t, err)
	assert.Len(t, user2Servers, 1)

	// Get servers for non-existent user
	var emptyServers MCPServers
	err = emptyServers.GetByUserID(db, 999)
	assert.NoError(t, err)
	assert.Len(t, emptyServers, 0)
}

func TestMCPServerAddAndRemoveTool(t *testing.T) {
	db := setupMCPTestDB(t)

	// Create a test server
	server := NewMCPServer()
	server.UserID = 1
	server.Name = "Test Server"
	err := server.Create(db)
	assert.NoError(t, err)

	// Create test tools
	tool1 := &Tool{Name: "Tool 1", Description: "Test Tool 1"}
	err = db.Create(tool1).Error
	assert.NoError(t, err)

	tool2 := &Tool{Name: "Tool 2", Description: "Test Tool 2"}
	err = db.Create(tool2).Error
	assert.NoError(t, err)

	// Add tools to the server
	err = server.AddTool(db, tool1)
	assert.NoError(t, err)

	err = server.AddTool(db, tool2)
	assert.NoError(t, err)

	// Verify tools were added
	tools, err := server.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, tools, 2)
	assert.Equal(t, tool1.ID, tools[0].ID)
	assert.Equal(t, tool2.ID, tools[1].ID)

	// Remove a tool
	err = server.RemoveTool(db, tool1)
	assert.NoError(t, err)

	// Verify tool was removed
	tools, err = server.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Equal(t, tool2.ID, tools[0].ID)
}

func TestMCPServerMarshalJSON(t *testing.T) {
	server := &MCPServer{
		Name:        "Test Server",
		Description: "Test Description",
		Status:      "stopped",
	}

	// Set created_at and updated_at for testing
	server.CreatedAt = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	server.UpdatedAt = time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

	// Marshal to JSON
	jsonData, err := server.MarshalJSON()
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "Test Server")
	assert.Contains(t, string(jsonData), "Test Description")
	assert.Contains(t, string(jsonData), "stopped")
	assert.Contains(t, string(jsonData), "2023-01-01T00:00:00Z")
	assert.Contains(t, string(jsonData), "2023-01-02T00:00:00Z")
}
