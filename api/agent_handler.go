package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/agent_session"
	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug"
)

// AgentMessageRequest represents the request body for sending a message to an agent
type AgentMessageRequest struct {
	Message   string                   `json:"message" binding:"required"`
	History   []map[string]interface{} `json:"history"`
	SessionID string                   `json:"session_id"`
}

// AgentConfigRequest represents the request body for creating/updating an agent config
type AgentConfigRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	PluginID    uint                   `json:"plugin_id" binding:"required"`
	AppID       uint                   `json:"app_id" binding:"required"`
	Config      map[string]interface{} `json:"config"`
	GroupIDs    []uint                 `json:"group_ids"`
	IsActive    bool                   `json:"is_active"`
	Namespace   string                 `json:"namespace"`
}

// HandleAgentMessage handles POST /api/agents/:id/message - sends message to agent and streams responses
func (a *API) HandleAgentMessage(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Parse agent config ID
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid agent ID", Detail: "Agent ID must be a valid number"}},
		})
		return
	}

	// Parse request body
	var req AgentMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid request", Detail: err.Error()}},
		})
		return
	}

	// Load agent config with preloaded relationships
	var agentConfig models.AgentConfig
	if err := a.service.DB.
		Preload("App.LLMs").
		Preload("App.Tools").
		Preload("App.Datasources").
		Preload("App.Credential").
		Preload("Plugin").
		Preload("Groups").
		First(&agentConfig, uint(agentID)).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
		})
		return
	}

	// Check if agent is active
	if !agentConfig.IsActive {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent inactive", Detail: "This agent is not currently active"}},
		})
		return
	}

	// Check if plugin is active and is agent type
	if !agentConfig.Plugin.IsActive {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Plugin inactive", Detail: "The agent's plugin is not active"}},
		})
		return
	}
	if !agentConfig.Plugin.IsAgentPlugin() {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid plugin type", Detail: "Plugin is not an agent plugin"}},
		})
		return
	}

	// Check user has access to agent (via groups)
	hasAccess := false
	if len(agentConfig.Groups) == 0 {
		// No groups means public access
		hasAccess = true
	} else {
		// Check if user is in any of the agent's groups
		for _, agentGroup := range agentConfig.Groups {
			for _, userGroup := range thisUser.Groups {
				if agentGroup.ID == userGroup.ID {
					hasAccess = true
					break
				}
			}
			if hasAccess {
				break
			}
		}
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Access denied", Detail: "You don't have access to this agent"}},
		})
		return
	}

	// Note: Budget checking happens in the proxy when agent calls LLMs
	// No need to check here since agents call through the proxy which enforces budgets

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-transform")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Content-Encoding", "none")

	// Create context with cancellation
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// Get plugin client
	pluginClient, err := a.service.GetPluginClient(agentConfig.PluginID)
	if err != nil {
		slog.Error("Failed to get plugin client", "error", err, "plugin_id", agentConfig.PluginID)
		sendSSEMessage(c.Writer, "error", fmt.Sprintf("Failed to connect to agent plugin: %v", err))
		return
	}

	// Create queue for this session
	factory := chat_session.CreateDefaultQueueFactoryWithSharedDB(a.service.DB)
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("agent-%d-%d", agentID, thisUser.ID)
	}
	queue, err := factory.CreateQueue(sessionID, nil)
	if err != nil {
		slog.Error("Failed to create message queue", "error", err, "session_id", sessionID)
		sendSSEMessage(c.Writer, "error", "Failed to create message queue")
		return
	}
	defer queue.Close()

	// Create agent session
	session, err := agent_session.NewAgentSession(&agentConfig, pluginClient, queue, a.service.DB)
	if err != nil {
		slog.Error("Failed to create agent session", "error", err, "agent_id", agentID)
		sendSSEMessage(c.Writer, "error", "Failed to create agent session")
		return
	}
	defer session.Close()

	// Send session ID to client
	sessionMsg := map[string]interface{}{
		"session_id":     session.GetID(),
		"agent_id":       agentConfig.ID,
		"agent_name":     agentConfig.Name,
		"available_llms": len(agentConfig.App.LLMs),
		"available_tools": len(agentConfig.App.Tools),
		"available_datasources": len(agentConfig.App.Datasources),
	}
	sessionJSON, _ := json.Marshal(sessionMsg)
	sendSSEMessage(c.Writer, "session", string(sessionJSON))

	// Send message to agent (async - responses will stream through queue)
	if err := session.SendMessage(req.Message, req.History); err != nil {
		slog.Error("Failed to send message to agent", "error", err, "agent_id", agentID)
		sendSSEMessage(c.Writer, "error", fmt.Sprintf("Failed to send message: %v", err))
		return
	}

	// Stream responses from queue
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	streamChan := queue.ConsumeStream(streamCtx)
	errorChan := queue.ConsumeErrors(streamCtx)

	clientGone := c.Writer.CloseNotify()

	for {
		select {
		case <-clientGone:
			slog.Debug("Client disconnected", "agent_id", agentID, "session_id", session.GetID())
			return

		case <-ctx.Done():
			slog.Debug("Context cancelled", "agent_id", agentID, "session_id", session.GetID())
			return

		case chunk, ok := <-streamChan:
			if !ok {
				// Stream closed
				sendSSEMessage(c.Writer, "done", "Agent session completed")
				return
			}

			// Parse agent message chunk
			var agentChunk agent_session.AgentMessageChunk
			if err := json.Unmarshal(chunk, &agentChunk); err != nil {
				slog.Error("Failed to unmarshal agent chunk", "error", err)
				continue
			}

			// Determine SSE event type based on chunk type
			eventType := "chunk"
			switch strings.ToUpper(agentChunk.Type) {
			case "CONTENT":
				eventType = "content"
			case "TOOL_CALL":
				eventType = "tool_call"
			case "TOOL_RESULT":
				eventType = "tool_result"
			case "THINKING":
				eventType = "thinking"
			case "ERROR":
				eventType = "error"
			case "DONE":
				eventType = "done"
			}

			// Send chunk to client
			chunkJSON, _ := json.Marshal(agentChunk)
			sendSSEMessage(c.Writer, eventType, string(chunkJSON))

			// If final chunk, we're done
			if agentChunk.IsFinal {
				slog.Debug("Received final chunk", "agent_id", agentID, "session_id", session.GetID())
				return
			}

		case err, ok := <-errorChan:
			if !ok {
				continue
			}
			slog.Error("Error from agent queue", "error", err, "agent_id", agentID)
			sendSSEMessage(c.Writer, "error", err.Error())
		}
	}
}

// HandleListAgents handles GET /api/agents - lists available agent configs
func (a *API) HandleListAgents(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	// Get namespace filter if provided
	namespace := c.Query("namespace")

	// List agents with pagination
	var agents models.AgentConfigs
	total, totalPages, err := agents.ListWithPagination(a.service.DB, limit, page, true, namespace, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Database error", Detail: err.Error()}},
		})
		return
	}
	_ = totalPages // unused but returned by the method

	// Filter agents by user access (groups)
	accessibleAgents := make([]models.AgentConfig, 0)
	for _, agent := range agents {
		// Load groups for this agent
		a.service.DB.Preload("Groups").First(&agent, agent.ID)

		// Check access
		hasAccess := false
		if len(agent.Groups) == 0 {
			hasAccess = true // Public
		} else {
			for _, agentGroup := range agent.Groups {
				for _, userGroup := range thisUser.Groups {
					if agentGroup.ID == userGroup.ID {
						hasAccess = true
						break
					}
				}
				if hasAccess {
					break
				}
			}
		}

		if hasAccess {
			accessibleAgents = append(accessibleAgents, agent)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": accessibleAgents,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// HandleGetAgent handles GET /api/agents/:id - gets specific agent config
func (a *API) HandleGetAgent(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Parse agent ID
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid agent ID", Detail: "Agent ID must be a valid number"}},
		})
		return
	}

	// Get agent config
	var agentConfig models.AgentConfig
	if err := agentConfig.Get(a.service.DB, uint(agentID)); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
		})
		return
	}
	// Load relationships
	a.service.DB.Preload("Plugin").Preload("App").Preload("Groups").First(&agentConfig, agentConfig.ID)

	// Check user has access
	hasAccess := false
	if len(agentConfig.Groups) == 0 {
		hasAccess = true
	} else {
		for _, agentGroup := range agentConfig.Groups {
			for _, userGroup := range thisUser.Groups {
				if agentGroup.ID == userGroup.ID {
					hasAccess = true
					break
				}
			}
			if hasAccess {
				break
			}
		}
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Access denied", Detail: "You don't have access to this agent"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": agentConfig})
}

// HandleCreateAgent handles POST /api/agents - creates new agent config
func (a *API) HandleCreateAgent(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Check if user is admin
	if !thisUser.IsAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can create agent configs"}},
		})
		return
	}

	// Parse request body
	var req AgentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid request", Detail: err.Error()}},
		})
		return
	}

	// Verify plugin exists and is agent type
	var plugin models.Plugin
	if err := a.service.DB.First(&plugin, req.PluginID).Error; err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid plugin", Detail: "Plugin not found"}},
		})
		return
	}
	if !plugin.IsAgentPlugin() {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid plugin type", Detail: "Plugin is not an agent plugin"}},
		})
		return
	}

	// Verify app exists
	var app models.App
	if err := a.service.DB.First(&app, req.AppID).Error; err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid app", Detail: "App not found"}},
		})
		return
	}

	// Create agent config
	agentConfig := models.NewAgentConfig()
	agentConfig.Name = req.Name
	agentConfig.Slug = slug.Make(req.Name)
	agentConfig.Description = req.Description
	agentConfig.PluginID = req.PluginID
	agentConfig.AppID = req.AppID
	agentConfig.Config = req.Config
	if agentConfig.Config == nil {
		agentConfig.Config = make(map[string]interface{})
	}
	agentConfig.IsActive = req.IsActive
	agentConfig.Namespace = req.Namespace

	// Create in database
	if err := agentConfig.Create(a.service.DB); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Database error", Detail: err.Error()}},
		})
		return
	}

	// Add groups if specified
	if len(req.GroupIDs) > 0 {
		for _, groupID := range req.GroupIDs {
			var group models.Group
			if err := a.service.DB.First(&group, groupID).Error; err == nil {
				agentConfig.AddGroup(a.service.DB, &group)
			}
		}
	}

	// Reload with relationships
	a.service.DB.Preload("Plugin").Preload("App").Preload("Groups").First(&agentConfig, agentConfig.ID)

	c.JSON(http.StatusCreated, gin.H{"data": agentConfig})
}

// HandleUpdateAgent handles PUT /api/agents/:id - updates agent config
func (a *API) HandleUpdateAgent(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Check if user is admin
	if !thisUser.IsAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can update agent configs"}},
		})
		return
	}

	// Parse agent ID
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid agent ID", Detail: "Agent ID must be a valid number"}},
		})
		return
	}

	// Parse request body
	var req AgentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid request", Detail: err.Error()}},
		})
		return
	}

	// Get existing agent config
	var agentConfig models.AgentConfig
	if err := agentConfig.Get(a.service.DB, uint(agentID)); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
		})
		return
	}

	// Update fields
	agentConfig.Name = req.Name
	agentConfig.Slug = slug.Make(req.Name)
	agentConfig.Description = req.Description
	agentConfig.Config = req.Config
	agentConfig.IsActive = req.IsActive
	if req.Namespace != "" {
		agentConfig.Namespace = req.Namespace
	}

	// Update in database
	if err := agentConfig.Update(a.service.DB); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Database error", Detail: err.Error()}},
		})
		return
	}

	// Update groups if specified
	if req.GroupIDs != nil {
		// Remove all existing groups
		a.service.DB.Model(&agentConfig).Association("Groups").Clear()
		// Add new groups
		for _, groupID := range req.GroupIDs {
			var group models.Group
			if err := a.service.DB.First(&group, groupID).Error; err == nil {
				agentConfig.AddGroup(a.service.DB, &group)
			}
		}
	}

	// Reload with relationships
	a.service.DB.Preload("Plugin").Preload("App").Preload("Groups").First(&agentConfig, agentConfig.ID)

	c.JSON(http.StatusOK, gin.H{"data": agentConfig})
}

// HandleDeleteAgent handles DELETE /api/agents/:id - deletes agent config
func (a *API) HandleDeleteAgent(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Check if user is admin
	if !thisUser.IsAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can delete agent configs"}},
		})
		return
	}

	// Parse agent ID
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid agent ID", Detail: "Agent ID must be a valid number"}},
		})
		return
	}

	// Delete agent config
	var agentConfig models.AgentConfig
	if err := agentConfig.Get(a.service.DB, uint(agentID)); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
		})
		return
	}
	if err := agentConfig.Delete(a.service.DB); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Database error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Agent config deleted successfully"})
}

// HandleActivateAgent handles POST /api/agents/:id/activate - activates agent config
func (a *API) HandleActivateAgent(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Check if user is admin
	if !thisUser.IsAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can activate agent configs"}},
		})
		return
	}

	// Parse agent ID
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid agent ID", Detail: "Agent ID must be a valid number"}},
		})
		return
	}

	// Activate agent
	var agentConfig models.AgentConfig
	if err := agentConfig.Get(a.service.DB, uint(agentID)); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
		})
		return
	}
	if err := agentConfig.Activate(a.service.DB); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Database error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Agent config activated successfully"})
}

// HandleDeactivateAgent handles POST /api/agents/:id/deactivate - deactivates agent config
func (a *API) HandleDeactivateAgent(c *gin.Context) {
	// Get authenticated user
	uObj, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found"}},
		})
		return
	}
	thisUser := uObj.(*models.User)

	// Check if user is admin
	if !thisUser.IsAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can deactivate agent configs"}},
		})
		return
	}

	// Parse agent ID
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid agent ID", Detail: "Agent ID must be a valid number"}},
		})
		return
	}

	// Deactivate agent
	var agentConfig models.AgentConfig
	if err := agentConfig.Get(a.service.DB, uint(agentID)); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
		})
		return
	}
	if err := agentConfig.Deactivate(a.service.DB); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Database error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Agent config deactivated successfully"})
}
