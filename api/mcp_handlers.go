package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// getUserIDFromContext extracts the user ID from the Gin context
func getUserIDFromContext(c *gin.Context) (uint, bool) {
	user, exists := c.Get("user")
	if !exists {
		return 0, false
	}
	currentUser := user.(*models.User)
	return currentUser.ID, true
}

// MCPServerCreateInput represents input for creating an MCP server
type MCPServerCreateInput struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// MCPServerUpdateInput represents input for updating an MCP server
type MCPServerUpdateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPServerToolInput represents input for adding/removing a tool
type MCPServerToolInput struct {
	ToolID uint `json:"tool_id" binding:"required"`
}

// MCPSessionCreateInput represents input for creating an MCP session
type MCPSessionCreateInput struct {
	MCPServerID uint   `json:"mcp_server_id" binding:"required"`
	ClientID    string `json:"client_id" binding:"required"`
}

// MCPHandlers contains handlers for MCP server operations
type MCPHandlers struct {
	db     *gorm.DB
	mcpSvc *services.MCPService
}

// NewMCPHandlers creates a new MCPHandlers
func NewMCPHandlers(db *gorm.DB, mcpSvc *services.MCPService) *MCPHandlers {
	return &MCPHandlers{
		db:     db,
		mcpSvc: mcpSvc,
	}
}

// RegisterRoutes registers MCP server routes
func (h *MCPHandlers) RegisterRoutes(router *gin.RouterGroup) {
	mcpGroup := router.Group("/mcp")
	{
		// Server management
		mcpGroup.POST("/servers", h.CreateMCPServer)
		mcpGroup.GET("/servers", h.GetMCPServers)
		mcpGroup.GET("/servers/:id", h.GetMCPServer)
		mcpGroup.PATCH("/servers/:id", h.UpdateMCPServer)
		mcpGroup.DELETE("/servers/:id", h.DeleteMCPServer)

		// Tool management
		mcpGroup.POST("/servers/:id/tools", h.AddToolToMCPServer)
		mcpGroup.DELETE("/servers/:id/tools/:tool_id", h.RemoveToolFromMCPServer)
		mcpGroup.GET("/servers/:id/tools", h.GetMCPServerTools)

		// Server control
		mcpGroup.POST("/servers/:id/start", h.StartMCPServer)
		mcpGroup.POST("/servers/:id/stop", h.StopMCPServer)
		mcpGroup.POST("/servers/:id/restart", h.RestartMCPServer)

		// Session management
		mcpGroup.POST("/sessions", h.CreateSession)
		mcpGroup.DELETE("/sessions/:session_id", h.EndSession)
		mcpGroup.PATCH("/sessions/:session_id/activity", h.UpdateSessionActivity)
	}
}

// CreateMCPServer creates a new MCP server
func (h *MCPHandlers) CreateMCPServer(c *gin.Context) {
	var input MCPServerCreateInput
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

	// Create the MCP server
	server, err := h.mcpSvc.CreateMCPServer(userID, input.Name, input.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, server)
}

// GetMCPServers gets all MCP servers for the current user
func (h *MCPHandlers) GetMCPServers(c *gin.Context) {
	// Get user ID from context
	userID, exists := getUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get servers for the user
	servers, err := h.mcpSvc.GetMCPServersByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, servers)
}

// GetMCPServer gets an MCP server by ID
func (h *MCPHandlers) GetMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, server)
}

// UpdateMCPServer updates an MCP server
func (h *MCPHandlers) UpdateMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Update the server
	server, err := h.mcpSvc.UpdateMCPServer(uint(id), input.Name, input.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, server)
}

// DeleteMCPServer deletes an MCP server
func (h *MCPHandlers) DeleteMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Delete the server
	if err := h.mcpSvc.DeleteMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// AddToolToMCPServer adds a tool to an MCP server
func (h *MCPHandlers) AddToolToMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Add the tool to the server
	if err := h.mcpSvc.AddToolToMCPServer(uint(id), input.ToolID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// RemoveToolFromMCPServer removes a tool from an MCP server
func (h *MCPHandlers) RemoveToolFromMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Remove the tool from the server
	if err := h.mcpSvc.RemoveToolFromMCPServer(uint(id), uint(toolID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetMCPServerTools gets all tools for an MCP server
func (h *MCPHandlers) GetMCPServerTools(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Get tools for the server
	tools, err := h.mcpSvc.GetToolsForMCPServer(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tools)
}

// StartMCPServer starts an MCP server
func (h *MCPHandlers) StartMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Start the server
	if err := h.mcpSvc.StartMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// StopMCPServer stops an MCP server
func (h *MCPHandlers) StopMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Stop the server
	if err := h.mcpSvc.StopMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// RestartMCPServer restarts an MCP server
func (h *MCPHandlers) RestartMCPServer(c *gin.Context) {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Restart the server
	if err := h.mcpSvc.RestartMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// CreateSession creates a new MCP session
func (h *MCPHandlers) CreateSession(c *gin.Context) {
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

	// Create the session
	session, err := h.mcpSvc.CreateSession(input.MCPServerID, userID, input.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, session)
}

// EndSession ends an MCP session
func (h *MCPHandlers) EndSession(c *gin.Context) {
	// Get session ID from path
	sessionID := c.Param("session_id")

	// End the session
	if err := h.mcpSvc.EndSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateSessionActivity updates the last seen timestamp for a session
func (h *MCPHandlers) UpdateSessionActivity(c *gin.Context) {
	// Get session ID from path
	sessionID := c.Param("session_id")

	// Update the session
	if err := h.mcpSvc.UpdateSessionActivity(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
