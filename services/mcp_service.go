package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ThinkInAIXYZ/go-mcp/protocol"
	"github.com/ThinkInAIXYZ/go-mcp/server"
	"github.com/ThinkInAIXYZ/go-mcp/transport"
	"github.com/TykTechnologies/midsommar/v2/mcpserver"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CustomLogger implements the pkg.Logger interface
type CustomLogger struct {
	logger *log.Logger
}

// NewCustomLogger creates a new CustomLogger
func NewCustomLogger(serverID uint) *CustomLogger {
	prefix := fmt.Sprintf("[MCP-SERVER-%d] ", serverID)
	logger := log.New(log.Writer(), prefix, log.LstdFlags)
	return &CustomLogger{
		logger: logger,
	}
}

// Info logs an info message
func (l *CustomLogger) Info(msg string) {
	l.logger.Printf("INFO: %s", msg)
}

// Infof logs an info message with formatting
func (l *CustomLogger) Infof(format string, args ...interface{}) {
	l.logger.Printf("INFO: "+format, args...)
}

// Debug logs a debug message
func (l *CustomLogger) Debug(msg string) {
	l.logger.Printf("DEBUG: %s", msg)
}

// Debugf logs a debug message with formatting
func (l *CustomLogger) Debugf(format string, args ...interface{}) {
	l.logger.Printf("DEBUG: "+format, args...)
}

// Error logs an error message
func (l *CustomLogger) Error(msg string) {
	l.logger.Printf("ERROR: %s", msg)
}

// Errorf logs an error message with formatting
func (l *CustomLogger) Errorf(format string, args ...interface{}) {
	l.logger.Printf("ERROR: "+format, args...)
}

// Warn logs a warning message
func (l *CustomLogger) Warn(msg string) {
	l.logger.Printf("WARN: %s", msg)
}

// Warnf logs a warning message with formatting
func (l *CustomLogger) Warnf(format string, args ...interface{}) {
	l.logger.Printf("WARN: "+format, args...)
}

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
	server       *server.Server
	transport    transport.ServerTransport
	SSEHandler   *transport.SSEHandler // Exported for use by handlers
	toolHandler  *mcpserver.ToolHandler
	endpoint     string
	shutdownFunc func()
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
	mcpServerModel, err := s.GetMCPServerByID(mcpServerID)
	if err != nil {
		return err
	}

	// Get tools for the server
	tools, err := mcpServerModel.GetTools(s.db)
	if err != nil {
		return fmt.Errorf("failed to get MCP server tools: %w", err)
	}

	// Create a dedicated tool handler that manages tool registration and calls
	toolHandler := mcpserver.NewToolHandler(s.db, mcpServerID, tools)

	// Define the full URL for the message endpoint (used by SSE transport)
	baseURL := fmt.Sprintf("http://localhost:8080%s", mcpServerModel.Endpoint)
	messageEndpoint := fmt.Sprintf("%s/message", baseURL)

	// Create a logger for the server
	mcpLogger := NewCustomLogger(mcpServerID)

	// Create the server transport and handler
	serverTransport, sseHandler, err := transport.NewSSEServerTransportAndHandler(
		messageEndpoint,
		transport.WithSSEServerTransportAndHandlerOptionLogger(mcpLogger),
	)
	if err != nil {
		return fmt.Errorf("failed to create MCP server transport: %w", err)
	}

	// Create the server with the transport
	mcpServer, err := server.NewServer(
		serverTransport,
		server.WithServerInfo(protocol.Implementation{
			Name:    fmt.Sprintf("midsommar-mcp-server-%d", mcpServerID),
			Version: "0.1.0",
		}),
		server.WithLogger(mcpLogger),
		server.WithCapabilities(protocol.ServerCapabilities{
			Tools: &protocol.ToolsCapability{
				ListChanged: true,
			},
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	// Register tools with the MCP server
	// This is required to make the tools available via the MCP protocol
	mcpLogger.Infof("Registering %d tools with MCP server", len(tools))

	for _, tool := range tools {
		operations := tool.GetOperations()

		// Skip tools with no operations
		if len(operations) == 0 {
			mcpLogger.Debugf("Skipping tool %s with no operations", tool.Name)
			continue
		}

		// Try to decode the OAS spec (it's stored as base64)
		decodedSpec, err := base64.StdEncoding.DecodeString(tool.OASSpec)
		if err != nil {
			mcpLogger.Warnf("Failed to decode base64 OAS spec for tool %s: %v", tool.Name, err)
			continue
		}

		// Create a universal client to parse the OAS spec
		uc, err := universalclient.NewClient(decodedSpec, "")
		if err != nil {
			mcpLogger.Warnf("Failed to create universal client for tool %s: %v", tool.Name, err)
			continue
		}

		for _, op := range operations {
			toolName := fmt.Sprintf("%s_%s", tool.Name, op)
			mcpLogger.Infof("Registering tool: %s", toolName)

			var schemaJSON []byte

			// Use AsTool to generate the complete schema with all type information
			llmTools, err := uc.AsTool(op)
			if err != nil {
				mcpLogger.Warnf("Failed to get tool definition for %s: %v", op, err)
				// Fallback to basic schema
				schemaJSON = []byte(`{
					"type": "object",
					"properties": {
						"parameters": { "type": "object" },
						"body": { "type": "object" }
					}
				}`)
			} else if len(llmTools) > 0 {
				// Extract the parameter schema from the first tool
				// This contains all the rich schema information from the OAS spec
				// including types, enums, formats, required flags, etc.
				paramSchema := llmTools[0].Function.Parameters

				// Convert to JSON - this preserves all the rich type information
				schemaJSON, err = json.Marshal(paramSchema)
				if err != nil {
					mcpLogger.Warnf("Failed to marshal schema for %s: %v", op, err)
					// Fallback to basic schema
					schemaJSON = []byte(`{
						"type": "object",
						"properties": {
							"parameters": { "type": "object" },
							"body": { "type": "object" }
						}
					}`)
				}

				mcpLogger.Debugf("Rich schema for %s: %s", op, string(schemaJSON))
			} else {
				mcpLogger.Warnf("AsTool returned no tools for %s", op)
				// Fallback to basic schema
				schemaJSON = []byte(`{
					"type": "object",
					"properties": {
						"parameters": { "type": "object" },
						"body": { "type": "object" }
					}
				}`)
			}

			// Register the tool with the MCP server
			mcpTool := &protocol.Tool{
				Name:           toolName,
				Description:    fmt.Sprintf("%s - %s operation", tool.Description, op),
				RawInputSchema: schemaJSON,
			}

			// Use toolHandler.CallTool as the handler for this tool
			mcpServer.RegisterTool(mcpTool, func(req *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
				mcpLogger.Debugf("Tool call: %s with args: %s", req.Name, string(req.RawArguments))

				// Directly delegate to the toolHandler
				return toolHandler.HandleToolRequest(req)
			})
		}
	}

	// Create a cancellable context for the server
	_, shutdownFunc := context.WithCancel(context.Background())

	// Start the server in a goroutine
	go func() {
		if err := mcpServer.Run(); err != nil {
			log.Printf("Error running MCP server %d: %v", mcpServerID, err)
		}
	}()

	// Update the server status in the database
	mcpServerModel.Status = "running"
	mcpServerModel.LastStarted = time.Now()
	if err := mcpServerModel.Update(s.db); err != nil {
		shutdownFunc() // Shutdown the server if we can't update the status
		return fmt.Errorf("failed to update MCP server status: %w", err)
	}

	// Store the active server instance with all components
	s.serversMutex.Lock()
	s.activeServers[mcpServerID] = &serverInstance{
		server:       mcpServer,
		transport:    serverTransport,
		SSEHandler:   sseHandler,
		toolHandler:  toolHandler,
		endpoint:     mcpServerModel.Endpoint,
		shutdownFunc: shutdownFunc,
	}
	s.serversMutex.Unlock()

	log.Printf("MCP server %d started with %d tools", mcpServerID, len(tools))

	return nil
}

// StopMCPServer stops an MCP server
func (s *MCPService) StopMCPServer(mcpServerID uint) error {
	s.serverHandlerMutex.Lock()
	defer s.serverHandlerMutex.Unlock()

	// Check if the server is running
	s.serversMutex.RLock()
	instance, exists := s.activeServers[mcpServerID]
	s.serversMutex.RUnlock()

	if !exists {
		return fmt.Errorf("MCP server is not running")
	}

	// Trigger server shutdown
	if instance.shutdownFunc != nil {
		instance.shutdownFunc()
	}

	// Give the server a chance to shut down
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Properly shut down the server
	if err := instance.server.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down MCP server %d: %v", mcpServerID, err)
	}

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

// RegisterMCPServerEndpoints registers MCP server routes with the HTTP router
func (s *MCPService) RegisterMCPServerEndpoints(router *gin.Engine) {
	// Lock to prevent changes while we're iterating
	s.serversMutex.RLock()
	defer s.serversMutex.RUnlock()

	// Register routes for each active server
	for _, instance := range s.activeServers {
		// Need to create an instance-specific handler to avoid closure issues
		sseHandler := instance.SSEHandler
		endpoint := instance.endpoint

		// Register SSE endpoint
		sseEndpoint := fmt.Sprintf("%s/sse", endpoint)
		router.GET(sseEndpoint, func(c *gin.Context) {
			sseHandler.HandleSSE().ServeHTTP(c.Writer, c.Request)
		})

		// Register message endpoint
		messageEndpoint := fmt.Sprintf("%s/message", endpoint)
		router.POST(messageEndpoint, func(c *gin.Context) {
			sseHandler.HandleMessage().ServeHTTP(c.Writer, c.Request)
		})

		log.Printf("Registered MCP server routes: %s/sse and %s/message", endpoint, endpoint)
	}
}

// GetMCPServerByEndpoint retrieves an MCP server by endpoint
func (s *MCPService) GetMCPServerByEndpoint(endpoint string) (*models.MCPServer, *serverInstance, error) {
	// Find server by endpoint in the database
	var mcpServer models.MCPServer
	if err := s.db.Where("endpoint = ?", endpoint).First(&mcpServer).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to get MCP server by endpoint: %w", err)
	}

	// Load tools
	tools, err := mcpServer.GetTools(s.db)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get MCP server tools: %w", err)
	}
	mcpServer.Tools = tools

	// Find the corresponding server instance
	s.serversMutex.RLock()
	instance, exists := s.activeServers[mcpServer.ID]
	s.serversMutex.RUnlock()

	if !exists {
		return &mcpServer, nil, nil
	}

	return &mcpServer, instance, nil
}

// GetMCPServerByPathPrefix retrieves an MCP server by path prefix
func (s *MCPService) GetMCPServerByPathPrefix(path string) (*models.MCPServer, *serverInstance, error) {
	// Loop through all active servers to find one with a matching path prefix
	s.serversMutex.RLock()
	defer s.serversMutex.RUnlock()

	for id, instance := range s.activeServers {
		if len(path) >= len(instance.endpoint) && path[:len(instance.endpoint)] == instance.endpoint {
			// Found a matching server
			server, err := s.GetMCPServerByID(id)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get MCP server: %w", err)
			}
			return server, instance, nil
		}
	}

	return nil, nil, fmt.Errorf("no MCP server found for path: %s", path)
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
