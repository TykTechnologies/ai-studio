package services

import (
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockDB is a mock implementation of the GORM DB
type MockDB struct {
	mock.Mock
	db *gorm.DB
}

// MockSecretService is a mock implementation of the secret service
type MockSecretService struct {
	mock.Mock
}

// MockNotificationService is a mock implementation of the notification service
type MockNotificationService struct {
	mock.Mock
}

// Test helpers
func setupMCPServiceTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// Auto migrate the schema
	db.AutoMigrate(&models.MCPServer{}, &models.MCPSession{}, &models.Tool{})

	return db
}

func createTestServer(t *testing.T, db *gorm.DB) *models.MCPServer {
	server := models.NewMCPServer()
	server.UserID = 1
	server.Name = "Test Server"
	server.Description = "Test Server Description"
	server.Endpoint = "/mcp/server/test"

	err := server.Create(db)
	assert.NoError(t, err)

	return server
}

func createTestSession(t *testing.T, db *gorm.DB, serverID uint) *models.MCPSession {
	session := &models.MCPSession{
		MCPServerID: serverID,
		UserID:      1,
		SessionID:   "test-session-id",
		ClientID:    "test-client-id",
		LastSeen:    time.Now(),
		Active:      true,
	}

	err := session.Create(db)
	assert.NoError(t, err)

	return session
}

func createTestTool(t *testing.T, db *gorm.DB) *models.Tool {
	tool := &models.Tool{
		Name:        "Test Tool",
		Description: "Test Tool Description",
	}

	err := db.Create(tool).Error
	assert.NoError(t, err)

	return tool
}

// Tests
func TestNewMCPService(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	assert.NotNil(t, mcpService)
	assert.Equal(t, db, mcpService.db)
	assert.Equal(t, secretService, mcpService.secretService)
	assert.Equal(t, notificationSvc, mcpService.notificationSvc)
	assert.NotNil(t, mcpService.activeServers)
	assert.Empty(t, mcpService.activeServers)
}

func TestCreateMCPServer(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a server
	server, err := mcpService.CreateMCPServer(1, "Test Server", "Test Description")
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, uint(1), server.UserID)
	assert.Equal(t, "Test Server", server.Name)
	assert.Equal(t, "Test Description", server.Description)
	assert.Equal(t, "stopped", server.Status)
	assert.NotEmpty(t, server.Endpoint)
}

func TestGetMCPServerByID(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	original := createTestServer(t, db)

	// Get the server by ID
	retrieved, err := mcpService.GetMCPServerByID(original.ID)
	assert.NoError(t, err)
	assert.Equal(t, original.ID, retrieved.ID)
	assert.Equal(t, original.Name, retrieved.Name)
	assert.Equal(t, original.Description, retrieved.Description)

	// Try to get a non-existent server
	nonExistent, err := mcpService.GetMCPServerByID(9999)
	assert.Error(t, err)
	assert.Nil(t, nonExistent)
}

func TestGetMCPServersByUserID(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create test servers for different users
	server1 := models.NewMCPServer()
	server1.UserID = 1
	server1.Name = "User 1 Server"
	server1.Description = "User 1 Server Description"
	server1.Endpoint = "/mcp/server/1"
	err := server1.Create(db)
	assert.NoError(t, err)

	server2 := models.NewMCPServer()
	server2.UserID = 1
	server2.Name = "User 1 Server 2"
	server2.Description = "User 1 Server 2 Description"
	server2.Endpoint = "/mcp/server/2"
	err = server2.Create(db)
	assert.NoError(t, err)

	server3 := models.NewMCPServer()
	server3.UserID = 2
	server3.Name = "User 2 Server"
	server3.Description = "User 2 Server Description"
	server3.Endpoint = "/mcp/server/3"
	err = server3.Create(db)
	assert.NoError(t, err)

	// Get servers for user 1
	user1Servers, err := mcpService.GetMCPServersByUserID(1)
	assert.NoError(t, err)
	assert.Len(t, user1Servers, 2)

	// Get servers for user 2
	user2Servers, err := mcpService.GetMCPServersByUserID(2)
	assert.NoError(t, err)
	assert.Len(t, user2Servers, 1)

	// Get servers for non-existent user
	emptyServers, err := mcpService.GetMCPServersByUserID(999)
	assert.NoError(t, err)
	assert.Len(t, emptyServers, 0)
}

func TestUpdateMCPServer(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	original := createTestServer(t, db)

	// Update the server
	updated, err := mcpService.UpdateMCPServer(original.ID, "Updated Name", "Updated Description")
	assert.NoError(t, err)
	assert.Equal(t, original.ID, updated.ID)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "Updated Description", updated.Description)

	// Verify the update in the database
	retrieved, err := mcpService.GetMCPServerByID(original.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Name)
	assert.Equal(t, "Updated Description", retrieved.Description)

	// Try to update a non-existent server
	nonExistent, err := mcpService.UpdateMCPServer(9999, "Non-existent", "Non-existent")
	assert.Error(t, err)
	assert.Nil(t, nonExistent)
}

func TestDeleteMCPServer(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)

	// Delete the server
	err := mcpService.DeleteMCPServer(server.ID)
	assert.NoError(t, err)

	// Verify the server was deleted
	retrieved, err := mcpService.GetMCPServerByID(server.ID)
	assert.Error(t, err)
	assert.Nil(t, retrieved)

	// Try to delete a non-existent server
	err = mcpService.DeleteMCPServer(9999)
	assert.Error(t, err)
}

func TestUserHasAccessToMCPServer(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)

	// Check if the owner has access
	hasAccess, err := mcpService.UserHasAccessToMCPServer(1, server.ID)
	assert.NoError(t, err)
	assert.True(t, hasAccess)

	// Check if a different user has access
	hasAccess, err = mcpService.UserHasAccessToMCPServer(2, server.ID)
	assert.NoError(t, err)
	assert.False(t, hasAccess)

	// Check access to a non-existent server
	hasAccess, err = mcpService.UserHasAccessToMCPServer(1, 9999)
	assert.Error(t, err)
	assert.False(t, hasAccess)
}

func TestAddAndRemoveToolFromMCPServer(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)

	// Create a test tool
	tool := createTestTool(t, db)

	// Add the tool to the server
	err := mcpService.AddToolToMCPServer(server.ID, tool.ID)
	assert.NoError(t, err)

	// Verify the tool was added
	tools, err := mcpService.GetToolsForMCPServer(server.ID)
	assert.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Equal(t, tool.ID, tools[0].ID)

	// Remove the tool from the server
	err = mcpService.RemoveToolFromMCPServer(server.ID, tool.ID)
	assert.NoError(t, err)

	// Verify the tool was removed
	tools, err = mcpService.GetToolsForMCPServer(server.ID)
	assert.NoError(t, err)
	assert.Len(t, tools, 0)
}

func TestStartStopRestartMCPServer(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)
	assert.Equal(t, "stopped", server.Status)

	// Start the server
	err := mcpService.StartMCPServer(server.ID)
	assert.NoError(t, err)

	// Verify the server status
	server, err = mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "running", server.Status)

	// Try to start an already running server
	err = mcpService.StartMCPServer(server.ID)
	assert.Error(t, err)

	// Stop the server
	err = mcpService.StopMCPServer(server.ID)
	assert.NoError(t, err)

	// Verify the server status
	server, err = mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "stopped", server.Status)

	// Try to stop an already stopped server
	err = mcpService.StopMCPServer(server.ID)
	assert.Error(t, err)

	// Restart a stopped server
	err = mcpService.RestartMCPServer(server.ID)
	assert.NoError(t, err)

	// Verify the server is running after restart
	server, err = mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "running", server.Status)
}

func TestConcurrentStartStop(t *testing.T) {
	// This test verifies that the mutex works correctly for concurrent server operations
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)

	// Concurrent start operations
	var startWg sync.WaitGroup
	startWg.Add(2)
	var startResults []error
	var startMux sync.Mutex

	go func() {
		defer startWg.Done()
		err := mcpService.StartMCPServer(server.ID)
		startMux.Lock()
		startResults = append(startResults, err)
		startMux.Unlock()
	}()

	go func() {
		defer startWg.Done()
		err := mcpService.StartMCPServer(server.ID)
		startMux.Lock()
		startResults = append(startResults, err)
		startMux.Unlock()
	}()

	startWg.Wait()

	// Only one start operation should succeed
	successCount := 0
	for _, err := range startResults {
		if err == nil {
			successCount++
		}
	}

	assert.Equal(t, 1, successCount)

	// Verify the server is running
	server, err := mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "running", server.Status)
}

func TestCreateSession(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)

	// Create a session
	session, err := mcpService.CreateSession(server.ID, server.UserID, "test-client")
	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, server.ID, session.MCPServerID)
	assert.Equal(t, server.UserID, session.UserID)
	assert.Equal(t, "test-client", session.ClientID)
	assert.True(t, session.Active)

	// Verify the server is running
	server, err = mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "running", server.Status)

	// Try to create a session for a non-existent server
	session, err = mcpService.CreateSession(9999, server.UserID, "test-client")
	assert.Error(t, err)
	assert.Nil(t, session)

	// Try to create a session for an unauthorized user
	session, err = mcpService.CreateSession(server.ID, 999, "test-client")
	assert.Error(t, err)
	assert.Nil(t, session)
}

func TestEndSession(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)

	// Start the server
	err := mcpService.StartMCPServer(server.ID)
	assert.NoError(t, err)

	// Create sessions
	session1, err := mcpService.CreateSession(server.ID, server.UserID, "client-1")
	assert.NoError(t, err)

	session2, err := mcpService.CreateSession(server.ID, server.UserID, "client-2")
	assert.NoError(t, err)

	// End the first session
	err = mcpService.EndSession(session1.SessionID)
	assert.NoError(t, err)

	// Verify the session is inactive
	var retrievedSession models.MCPSession
	err = retrievedSession.GetBySessionID(db, session1.SessionID)
	assert.NoError(t, err)
	assert.False(t, retrievedSession.Active)

	// Verify the server is still running (because session2 is still active)
	server, err = mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "running", server.Status)

	// End the second session
	err = mcpService.EndSession(session2.SessionID)
	assert.NoError(t, err)

	// Verify the session is inactive
	err = retrievedSession.GetBySessionID(db, session2.SessionID)
	assert.NoError(t, err)
	assert.False(t, retrievedSession.Active)

	// Verify the server is now stopped (because no active sessions)
	server, err = mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "stopped", server.Status)

	// Try to end a non-existent session
	err = mcpService.EndSession("non-existent-session-id")
	assert.Error(t, err)
}

func TestUpdateSessionActivity(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// Create a test server
	server := createTestServer(t, db)

	// Create a session
	session, err := mcpService.CreateSession(server.ID, server.UserID, "test-client")
	assert.NoError(t, err)
	originalLastSeen := session.LastSeen

	// Wait a moment to ensure timestamps differ
	time.Sleep(10 * time.Millisecond)

	// Update session activity
	err = mcpService.UpdateSessionActivity(session.SessionID)
	assert.NoError(t, err)

	// Verify the last seen timestamp was updated
	var retrievedSession models.MCPSession
	err = retrievedSession.GetBySessionID(db, session.SessionID)
	assert.NoError(t, err)
	assert.True(t, retrievedSession.LastSeen.After(originalLastSeen))

	// Try to update a non-existent session
	err = mcpService.UpdateSessionActivity("non-existent-session-id")
	assert.Error(t, err)
}

// Additional integration test that simulates a complete server lifecycle
func TestMCPServerLifecycle(t *testing.T) {
	db := setupMCPServiceTestDB(t)
	secretService := &secrets.SecretService{}
	notificationSvc := &NotificationService{}

	mcpService := NewMCPService(db, secretService, notificationSvc)

	// 1. Create a server
	server, err := mcpService.CreateMCPServer(1, "Lifecycle Test Server", "Server for lifecycle testing")
	assert.NoError(t, err)
	assert.Equal(t, "stopped", server.Status)

	// 2. Create a tool
	tool := createTestTool(t, db)

	// 3. Add the tool to the server
	err = mcpService.AddToolToMCPServer(server.ID, tool.ID)
	assert.NoError(t, err)

	// 4. Start the server
	err = mcpService.StartMCPServer(server.ID)
	assert.NoError(t, err)

	// 5. Create a session
	session, err := mcpService.CreateSession(server.ID, 1, "lifecycle-client")
	assert.NoError(t, err)

	// 6. Update session activity
	err = mcpService.UpdateSessionActivity(session.SessionID)
	assert.NoError(t, err)

	// 7. Remove the tool from the server
	err = mcpService.RemoveToolFromMCPServer(server.ID, tool.ID)
	assert.NoError(t, err)

	// 8. End the session
	err = mcpService.EndSession(session.SessionID)
	assert.NoError(t, err)

	// 9. Verify the server was stopped
	server, err = mcpService.GetMCPServerByID(server.ID)
	assert.NoError(t, err)
	assert.Equal(t, "stopped", server.Status)

	// 10. Delete the server
	err = mcpService.DeleteMCPServer(server.ID)
	assert.NoError(t, err)

	// 11. Verify the server was deleted
	deletedServer, err := mcpService.GetMCPServerByID(server.ID)
	assert.Error(t, err)
	assert.Nil(t, deletedServer)
}
