package services

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/MegaGrindStone/go-mcp"
	"github.com/TykTechnologies/midsommar/v2/mcpserver"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

// MCPService handles MCP server operations
type MCPService struct {
	db                 *gorm.DB
	secretService      *secrets.SecretService
	notificationSvc    *NotificationService
	activeServers      map[uint]*serverInstance
	serversMutex       sync.RWMutex
	serverHandlerMutex sync.Mutex
}

// serverInstance represents a running MCP server instance
type serverInstance struct {
	server   *mcp.Server
	sseServ  *mcp.SSEServer
	handler  *mcpserver.HTTPHandler
	endpoint string
}

// NewMCPService creates a new MCP service
func NewMCPService(db *gorm.DB, secretService *secrets.SecretService, notificationSvc *NotificationService) *MCPService {
	return &MCPService{
		db:              db,
		secretService:   secretService,
		notificationSvc: notificationSvc,
		activeServers:   make(map[uint]*serverInstance),
	}
}

// CreateMCPServer creates a new MCP server configuration
func (s *MCPService) CreateMCPServer(userID uint, name, description string) (*models.MCPServer, error) {
	// Create a new server
	server := models.NewMCPServer()
	server.UserID = userID
	server.Name = name
	server.Description = description
	server.Status = "stopped"

	// Generate a unique endpoint
	endpoint := fmt.Sprintf("/mcp/server/%d", time.Now().UnixNano())
	server.Endpoint = endpoint

	// Save to database
	if err := server.Create(s.db); err != nil {
		return nil, fmt.Errorf("failed to create MCP server: %w", err)
	}

	return server, nil
}

// GetMCPServerByID retrieves an MCP server by ID
func (s *MCPService) GetMCPServerByID(id uint) (*models.MCPServer, error) {
	server := models.NewMCPServer()
	if err := server.Get(s.db, id); err != nil {
		return nil, fmt.Errorf("failed to get MCP server: %w", err)
	}

	// Load tools
	tools, err := server.GetTools(s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP server tools: %w", err)
	}
	server.Tools = tools

	return server, nil
}

// GetMCPServersByUserID retrieves all MCP servers for a user
func (s *MCPService) GetMCPServersByUserID(userID uint) ([]models.MCPServer, error) {
	var servers models.MCPServers
	if err := servers.GetByUserID(s.db, userID); err != nil {
		return nil, fmt.Errorf("failed to get MCP servers for user: %w", err)
	}

	// Load tools for each server
	for i := range servers {
		tools, err := servers[i].GetTools(s.db)
		if err != nil {
			return nil, fmt.Errorf("failed to get MCP server tools: %w", err)
		}
		servers[i].Tools = tools
	}

	return servers, nil
}

// UpdateMCPServer updates an MCP server configuration
func (s *MCPService) UpdateMCPServer(id uint, name, description string) (*models.MCPServer, error) {
	server, err := s.GetMCPServerByID(id)
	if err != nil {
		return nil, err
	}

	server.Name = name
	server.Description = description

	if err := server.Update(s.db); err != nil {
		return nil, fmt.Errorf("failed to update MCP server: %w", err)
	}

	return server, nil
}

// DeleteMCPServer deletes an MCP server configuration
func (s *MCPService) DeleteMCPServer(id uint) error {
	// Stop the server if it's running
	if err := s.StopMCPServer(id); err != nil {
		log.Printf("Error stopping MCP server %d: %v", id, err)
		// Continue with deletion even if stopping fails
	}

	// Delete the server
	server := models.NewMCPServer()
	if err := server.Get(s.db, id); err != nil {
		return fmt.Errorf("failed to get MCP server: %w", err)
	}

	if err := server.Delete(s.db); err != nil {
		return fmt.Errorf("failed to delete MCP server: %w", err)
	}

	return nil
}

// UserHasAccessToMCPServer checks if a user has access to an MCP server
func (s *MCPService) UserHasAccessToMCPServer(userID, mcpServerID uint) (bool, error) {
	server := models.NewMCPServer()
	if err := server.Get(s.db, mcpServerID); err != nil {
		return false, fmt.Errorf("failed to get MCP server: %w", err)
	}

	// Currently, only the owner has access
	return server.UserID == userID, nil
}

// AddToolToMCPServer adds a tool to an MCP server
func (s *MCPService) AddToolToMCPServer(mcpServerID, toolID uint) error {
	server := models.NewMCPServer()
	if err := server.Get(s.db, mcpServerID); err != nil {
		return fmt.Errorf("failed to get MCP server: %w", err)
	}

	tool := &models.Tool{}
	if err := tool.Get(s.db, toolID); err != nil {
		return fmt.Errorf("failed to get tool: %w", err)
	}

	if err := server.AddTool(s.db, tool); err != nil {
		return fmt.Errorf("failed to add tool to MCP server: %w", err)
	}

	// If the server is running, restart it to apply changes
	s.serversMutex.RLock()
	_, isRunning := s.activeServers[mcpServerID]
	s.serversMutex.RUnlock()

	if isRunning {
		if err := s.RestartMCPServer(mcpServerID); err != nil {
			return fmt.Errorf("failed to restart MCP server: %w", err)
		}
	}

	return nil
}

// RemoveToolFromMCPServer removes a tool from an MCP server
func (s *MCPService) RemoveToolFromMCPServer(mcpServerID, toolID uint) error {
	server := models.NewMCPServer()
	if err := server.Get(s.db, mcpServerID); err != nil {
		return fmt.Errorf("failed to get MCP server: %w", err)
	}

	tool := &models.Tool{}
	if err := tool.Get(s.db, toolID); err != nil {
		return fmt.Errorf("failed to get tool: %w", err)
	}

	if err := server.RemoveTool(s.db, tool); err != nil {
		return fmt.Errorf("failed to remove tool from MCP server: %w", err)
	}

	// If the server is running, restart it to apply changes
	s.serversMutex.RLock()
	_, isRunning := s.activeServers[mcpServerID]
	s.serversMutex.RUnlock()

	if isRunning {
		if err := s.RestartMCPServer(mcpServerID); err != nil {
			return fmt.Errorf("failed to restart MCP server: %w", err)
		}
	}

	return nil
}

// GetToolsForMCPServer retrieves all tools for an MCP server
func (s *MCPService) GetToolsForMCPServer(mcpServerID uint) ([]models.Tool, error) {
	server := models.NewMCPServer()
	if err := server.Get(s.db, mcpServerID); err != nil {
		return nil, fmt.Errorf("failed to get MCP server: %w", err)
	}

	tools, err := server.GetTools(s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP server tools: %w", err)
	}

	return tools, nil
}

// StartMCPServer starts an MCP server
func (s *MCPService) StartMCPServer(mcpServerID uint) error {
	s.serverHandlerMutex.Lock()
	defer s.serverHandlerMutex.Unlock()

	// Check if the server is already running
	s.serversMutex.RLock()
	if _, exists := s.activeServers[mcpServerID]; exists {
		s.serversMutex.RUnlock()
		return fmt.Errorf("MCP server is already running")
	}
	s.serversMutex.RUnlock()

	// Get the server configuration
	server, err := s.GetMCPServerByID(mcpServerID)
	if err != nil {
		return err
	}

	// For a complete implementation, we would load tools here
	// For now, we're using a mock implementation
	_, err = server.GetTools(s.db)
	if err != nil {
		return fmt.Errorf("failed to get MCP server tools: %w", err)
	}

	// We need to mock this implementation for now as we're having Go MCP library compatibility issues
	// In a real implementation, we would create the MCP server with proper tool handlers

	// For now, just update the server status in the database
	server.Status = "running"
	server.LastStarted = time.Now()
	if err := server.Update(s.db); err != nil {
		return fmt.Errorf("failed to update MCP server status: %w", err)
	}

	// Record this server as active
	s.serversMutex.Lock()
	s.activeServers[mcpServerID] = &serverInstance{
		endpoint: server.Endpoint,
	}
	s.serversMutex.Unlock()

	log.Printf("MCP server %d started with mock implementation", mcpServerID)

	return nil
}

// StopMCPServer stops an MCP server
func (s *MCPService) StopMCPServer(mcpServerID uint) error {
	s.serverHandlerMutex.Lock()
	defer s.serverHandlerMutex.Unlock()

	// Check if the server is running
	s.serversMutex.RLock()
	_, exists := s.activeServers[mcpServerID]
	s.serversMutex.RUnlock()

	if !exists {
		return fmt.Errorf("MCP server is not running")
	}

	// For a complete implementation, we would shut down the MCP server here
	// Since we're using a mock implementation, we just need to log the shutdown
	log.Printf("MCP server %d stopped", mcpServerID)

	// Remove the server from active servers
	s.serversMutex.Lock()
	delete(s.activeServers, mcpServerID)
	s.serversMutex.Unlock()

	// Update the server status in the database
	server, err := s.GetMCPServerByID(mcpServerID)
	if err != nil {
		return err
	}

	server.Status = "stopped"
	if err := server.Update(s.db); err != nil {
		return fmt.Errorf("failed to update MCP server status: %w", err)
	}

	return nil
}

// RestartMCPServer restarts an MCP server
func (s *MCPService) RestartMCPServer(mcpServerID uint) error {
	if err := s.StopMCPServer(mcpServerID); err != nil {
		// If the server was not running, that's fine
		if err.Error() != "MCP server is not running" {
			return err
		}
	}

	// Allow some time for shutdown to complete
	time.Sleep(500 * time.Millisecond)

	return s.StartMCPServer(mcpServerID)
}

// RegisterMCPServerRoutes registers MCP server routes with the HTTP router
func (s *MCPService) RegisterMCPServerRoutes(router http.Handler) {
	// This method would be used to register the MCP server routes with the HTTP router
	// For now, we'll leave it unimplemented
}

// CreateSession creates a new MCP session
func (s *MCPService) CreateSession(mcpServerID, userID uint, clientID string) (*models.MCPSession, error) {
	// Check if the server exists
	server, err := s.GetMCPServerByID(mcpServerID)
	if err != nil {
		return nil, err
	}

	// Check if the user has access to the server
	hasAccess, err := s.UserHasAccessToMCPServer(userID, mcpServerID)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, fmt.Errorf("user does not have access to this MCP server")
	}

	// Create a new session
	session := &models.MCPSession{
		MCPServerID: mcpServerID,
		UserID:      userID,
		SessionID:   fmt.Sprintf("session-%d", time.Now().UnixNano()),
		ClientID:    clientID,
		LastSeen:    time.Now(),
		Active:      true,
	}

	// Save to database
	if err := session.Create(s.db); err != nil {
		return nil, fmt.Errorf("failed to create MCP session: %w", err)
	}

	// Start the server if it's not already running
	s.serversMutex.RLock()
	_, isRunning := s.activeServers[mcpServerID]
	s.serversMutex.RUnlock()

	if !isRunning && server.Status != "running" {
		if err := s.StartMCPServer(mcpServerID); err != nil {
			return nil, fmt.Errorf("failed to start MCP server: %w", err)
		}
	}

	return session, nil
}

// EndSession ends an MCP session
func (s *MCPService) EndSession(sessionID string) error {
	// Find the session
	session := &models.MCPSession{}
	if err := session.GetBySessionID(s.db, sessionID); err != nil {
		return fmt.Errorf("failed to get MCP session: %w", err)
	}

	// Mark the session as inactive
	if err := session.MarkInactive(s.db); err != nil {
		return fmt.Errorf("failed to mark MCP session as inactive: %w", err)
	}

	// Check if there are any other active sessions for this server
	activeSessions, err := models.GetActiveSessionsByServer(s.db, session.MCPServerID)
	if err != nil {
		return fmt.Errorf("failed to get active MCP sessions: %w", err)
	}

	// If there are no other active sessions, stop the server
	if len(activeSessions) == 0 {
		if err := s.StopMCPServer(session.MCPServerID); err != nil {
			return fmt.Errorf("failed to stop MCP server: %w", err)
		}
	}

	return nil
}

// UpdateSessionActivity updates the last seen timestamp for a session
func (s *MCPService) UpdateSessionActivity(sessionID string) error {
	// Find the session
	session := &models.MCPSession{}
	if err := session.GetBySessionID(s.db, sessionID); err != nil {
		return fmt.Errorf("failed to get MCP session: %w", err)
	}

	// Update the last seen timestamp
	if err := session.UpdateLastSeen(s.db); err != nil {
		return fmt.Errorf("failed to update MCP session last seen: %w", err)
	}

	return nil
}

// CleanupInactiveSessions cleans up inactive sessions
func (s *MCPService) CleanupInactiveSessions(inactiveThreshold time.Duration) error {
	// This would be run periodically to clean up inactive sessions
	// For now, we'll leave it unimplemented
	return nil
}
