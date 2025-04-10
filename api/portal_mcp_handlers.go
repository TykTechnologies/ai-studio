package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PortalMCPHandlers contains handlers for MCP server operations for portal users
type PortalMCPHandlers struct {
	db     *gorm.DB
	mcpSvc *services.MCPService
}

// NewPortalMCPHandlers creates a new PortalMCPHandlers
func NewPortalMCPHandlers(db *gorm.DB, mcpSvc *services.MCPService) *PortalMCPHandlers {
	return &PortalMCPHandlers{
		db:     db,
		mcpSvc: mcpSvc,
	}
}

// CreateMCPServer creates a new MCP server for the current user
func (h *PortalMCPHandlers) CreateMCPServer(c *gin.Context) {
	var input MCPServerCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context (required for portal users)
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Additional validation could be added here
	// - Check server name length
	// - Validate description
	// - Check user quota for servers

	// Create the MCP server with user ownership
	server, err := h.mcpSvc.CreateMCPServer(userID, input.Name, input.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create MCP server"})
		return
	}

	// Format server to match the frontend expectation with attributes property
	formattedServer := gin.H{
		"id": server.ID,
		"attributes": gin.H{
			"name":        server.Name,
			"description": server.Description,
			"endpoint":    server.Endpoint,
			"status":      server.Status,
			"tools":       server.Tools,
			"created_at":  server.CreatedAt,
			"updated_at":  server.UpdatedAt,
		},
	}
	// Return sanitized response in the expected format that includes a data property
	c.JSON(http.StatusCreated, gin.H{"data": formattedServer})
}

// GetMCPServers gets all MCP servers for the current user
func (h *PortalMCPHandlers) GetMCPServers(c *gin.Context) {
	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get servers for the user - this already filters by user ID
	servers, err := h.mcpSvc.GetMCPServersByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch MCP servers"})
		return
	}

	// Format servers to match the frontend expectation with attributes property
	formattedServers := []gin.H{}
	for _, server := range servers {
		formattedServers = append(formattedServers, gin.H{
			"id": server.ID,
			"attributes": gin.H{
				"name":        server.Name,
				"description": server.Description,
				"endpoint":    server.Endpoint,
				"status":      server.Status,
				"tools":       server.Tools,
				"created_at":  server.CreatedAt,
				"updated_at":  server.UpdatedAt,
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": formattedServers})
}

// GetMCPServer gets an MCP server by ID, ensuring the user has access
func (h *PortalMCPHandlers) GetMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Get the server
	server, err := h.mcpSvc.GetMCPServerByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch server"})
		return
	}

	// Format server to match the frontend expectation with attributes property
	formattedServer := gin.H{
		"id": server.ID,
		"attributes": gin.H{
			"name":        server.Name,
			"description": server.Description,
			"endpoint":    server.Endpoint,
			"status":      server.Status,
			"tools":       server.Tools,
			"created_at":  server.CreatedAt,
			"updated_at":  server.UpdatedAt,
		},
	}
	c.JSON(http.StatusOK, gin.H{"data": formattedServer})
}

// UpdateMCPServer updates an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) UpdateMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	var input MCPServerUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Additional validation could be added here

	// Update the server
	server, err := h.mcpSvc.UpdateMCPServer(uint(id), input.Name, input.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update server"})
		return
	}

	// Format server to match the frontend expectation with attributes property
	formattedServer := gin.H{
		"id": server.ID,
		"attributes": gin.H{
			"name":        server.Name,
			"description": server.Description,
			"endpoint":    server.Endpoint,
			"status":      server.Status,
			"tools":       server.Tools,
			"created_at":  server.CreatedAt,
			"updated_at":  server.UpdatedAt,
		},
	}
	c.JSON(http.StatusOK, gin.H{"data": formattedServer})
}

// DeleteMCPServer deletes an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) DeleteMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Delete the server
	if err := h.mcpSvc.DeleteMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete server"})
		return
	}

	c.Status(http.StatusNoContent)
}

// AddToolToMCPServer adds a tool to an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) AddToolToMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	var input MCPServerToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Check if user has access to the tool
	// This would need to be implemented in the MCP service
	// hasAccessToTool, err := h.mcpSvc.UserHasAccessToTool(userID, input.ToolID)
	// if err != nil || !hasAccessToTool {
	//     c.JSON(http.StatusForbidden, gin.H{"error": "access denied to tool"})
	//     return
	// }

	// Add the tool to the server
	if err := h.mcpSvc.AddToolToMCPServer(uint(id), input.ToolID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add tool to server"})
		return
	}

	c.Status(http.StatusNoContent)
}

// RemoveToolFromMCPServer removes a tool from an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) RemoveToolFromMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	// Get tool ID from path
	toolID, err := strconv.Atoi(c.Param("tool_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool ID"})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Remove the tool from the server
	if err := h.mcpSvc.RemoveToolFromMCPServer(uint(id), uint(toolID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove tool from server"})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetMCPServerTools gets all tools for an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) GetMCPServerTools(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Get tools for the server
	tools, err := h.mcpSvc.GetToolsForMCPServer(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get tools"})
		return
	}

	// Format tools to match the frontend expectation with attributes property
	formattedTools := []gin.H{}
	for _, tool := range tools {
		formattedTools = append(formattedTools, gin.H{
			"id":   tool.ID,
			"type": "tool",
			"attributes": gin.H{
				"name":          tool.Name,
				"description":   tool.Description,
				"tool_type":     tool.ToolType, // Fixed field name from Type to ToolType
				"oas_spec":      tool.OASSpec,
				"privacy_score": tool.PrivacyScore,
				// Using empty slice for operations since it's not directly available
				"operations":       []string{},
				"auth_key":         tool.AuthKey,
				"auth_schema_name": tool.AuthSchemaName,
				"file_stores":      tool.FileStores,
				"filters":          tool.Filters,
				"dependencies":     tool.Dependencies,
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": formattedTools})
}

// StartMCPServer starts an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) StartMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Start the server
	if err := h.mcpSvc.StartMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start server"})
		return
	}

	c.Status(http.StatusNoContent)
}

// StopMCPServer stops an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) StopMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Stop the server
	if err := h.mcpSvc.StopMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stop server"})
		return
	}

	c.Status(http.StatusNoContent)
}

// RestartMCPServer restarts an MCP server, ensuring the user has access
func (h *PortalMCPHandlers) RestartMCPServer(c *gin.Context) {
	// Get server ID from path
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Restart the server
	if err := h.mcpSvc.RestartMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restart server"})
		return
	}

	c.Status(http.StatusNoContent)
}

// CreateSession creates a new MCP session
func (h *PortalMCPHandlers) CreateSession(c *gin.Context) {
	var input MCPSessionCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if the user has access to the server
	hasAccess, err := h.mcpSvc.UserHasAccessToMCPServer(userID, input.MCPServerID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Create the session
	session, err := h.mcpSvc.CreateSession(input.MCPServerID, userID, input.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": session})
}

// EndSession ends an MCP session, ensuring the user has access
func (h *PortalMCPHandlers) EndSession(c *gin.Context) {
	// Get session ID from path
	sessionID := c.Param("session_id")

	// Get user ID from context
	_, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// TODO: Check if user owns this session
	// This would need a service method to verify session ownership
	// For now, we'll just check authentication

	// End the session
	if err := h.mcpSvc.EndSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to end session"})
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateSessionActivity updates the last seen timestamp for a session
func (h *PortalMCPHandlers) UpdateSessionActivity(c *gin.Context) {
	// Get session ID from path
	sessionID := c.Param("session_id")

	// Get user ID from context
	_, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// TODO: Check if user owns this session
	// This would need a service method to verify session ownership
	// For now, we'll just check authentication

	// Update the session
	if err := h.mcpSvc.UpdateSessionActivity(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session activity"})
		return
	}

	c.Status(http.StatusNoContent)
}
