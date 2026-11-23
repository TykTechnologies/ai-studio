package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/agent_session"
	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug"
)

// agentSessions stores active agent sessions for message handling
var agentSessions sync.Map

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
	// IsActive is the live switch. Omitted = active on create (the historical
	// default), unchanged on update. Setting it needs agents:publish.
	IsActive  *bool  `json:"is_active"`
	Namespace string `json:"namespace"`
	// WorkspaceID places the agent (create) or moves it (update); omitted
	// means the landing rule on create and no move on update. Admin API only.
	WorkspaceID *uint `json:"workspace_id"`
}

// wantsActive reports the requested live state, defaulting to true on create.
func (r *AgentConfigRequest) wantsActive() bool { return r.IsActive == nil || *r.IsActive }

// Workspaces (B4d). The agent handlers serve both the admin API (/api/v1)
// and the portal and chat (/common). Workspace checks apply on the admin
// API only: on /common an authorization context exists too, and portal
// visibility stays with agent groups.

// onAdminAPI reports whether the request came in on the admin API: it
// passed the admin route gate (requireRoutePermission marks it). /common
// requests never pass that gate.
func onAdminAPI(c *gin.Context) bool {
	_, gated := c.Get(authzGatedKey)
	return gated
}

// requireTeamsWrite: an agent's teams decide who may use it in the Portal
// and Chat. Teams are global, so changing an agent's team set needs the
// global groups:write (.48); an unchanged or omitted set needs nothing.
func (a *API) requireTeamsWrite(c *gin.Context, stored []models.Group, requested []uint) bool {
	if requested == nil || sameTeamSet(stored, requested) || a.holds(c, authz.Write("groups")) {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Write("groups")))
	return false
}

func sameTeamSet(stored []models.Group, requested []uint) bool {
	have := map[uint]bool{}
	for _, g := range stored {
		have[g.ID] = true
	}
	want := map[uint]bool{}
	for _, id := range requested {
		want[id] = true
	}
	if len(have) != len(want) {
		return false
	}
	for id := range want {
		if !have[id] {
			return false
		}
	}
	return true
}

// agentNotFound is the not-found answer for an agent, also given for an
// agent in a Workspace the caller cannot see.
func agentNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
	})
}

// requireAgentInWorkspace runs the admin API's object checks on a loaded
// agent: hidden (404) unless the caller's read covers its Workspace, then
// perm in that Workspace (403). No-op off the admin API.
func (a *API) requireAgentInWorkspace(c *gin.Context, ag *models.AgentConfig, perm authz.Permission) bool {
	if !onAdminAPI(c) {
		return true
	}
	if !a.requireVisible(c, "agents", ag.WorkspaceID, agentNotFound) {
		return false
	}
	return a.requireInWorkspace(c, perm, ag.WorkspaceID)
}

// redactAgent withholds credential material from an admin API response
// (.46): the agent's plugin configuration needs agents:write in its
// Workspace, and the embedded plugin's own configuration the same rule as
// everywhere else (mayReadPluginConfig). Readers get the metadata. The
// embedded App needs read in the App's Workspace (.55).
//
// On the portal (/common) the caller is a chat user, not an administrator:
// the response names the agent and nothing it is built from. The agent's
// configuration, its plugin and its App are left out (their ids kept), as
// they can carry credentials (tgoe).
func (a *API) redactAgent(c *gin.Context, ag *models.AgentConfig) {
	a.redactAgentWith(c, ag, a.agentRedactor(ag))
}

// agentRedactor loads the secret-field classifiers of the plugins the agents
// are built on: one query for the classifiers, and one more for the commands
// of plugins that were not preloaded with their agent (they are then set on
// the agent's Plugin-less copy as a command only, see agentPluginCommand).
func (a *API) agentRedactor(agents ...*models.AgentConfig) pluginRedactor {
	plugins := make([]*models.Plugin, 0, len(agents))
	var missing []uint
	for _, ag := range agents {
		if ag == nil {
			continue
		}
		if ag.Plugin != nil {
			plugins = append(plugins, ag.Plugin)
		} else if ag.PluginID != 0 {
			missing = append(missing, ag.PluginID)
		}
	}
	if len(missing) > 0 && a.service != nil && a.service.DB != nil {
		var rows []models.Plugin
		if err := a.service.DB.Select("id", "command").Where("id IN ?", missing).Find(&rows).Error; err == nil {
			for i := range rows {
				plugins = append(plugins, &rows[i])
			}
		}
	}
	red := a.pluginRedactorFor(plugins...)
	if len(missing) == 0 {
		return red
	}
	// Remember the commands looked up, under a key no real command has.
	out := make(pluginRedactor, len(red)+len(plugins))
	for k, v := range red {
		out[k] = v
	}
	for _, p := range plugins {
		out[agentPluginKey(p.ID)] = red.secrets(p.Command)
	}
	return out
}

// agentPluginKey is where agentRedactor keeps the classifier of a plugin id.
func agentPluginKey(id uint) string { return "\x00plugin-id:" + strconv.FormatUint(uint64(id), 10) }

// agentSecrets is the classifier for an agent's own configuration, which
// follows its plugin's schema. A plugin agentRedactor could not find is
// classified by key name.
func agentSecrets(ag *models.AgentConfig, red pluginRedactor) *services.PluginConfigSecrets {
	if ag.Plugin != nil {
		return red.secrets(ag.Plugin.Command)
	}
	return red[agentPluginKey(ag.PluginID)]
}

// redactAgentWith is redactAgent with the redactor already loaded (a list).
func (a *API) redactAgentWith(c *gin.Context, ag *models.AgentConfig, red pluginRedactor) {
	if !onAdminAPI(c) {
		ag.Config = nil
		ag.Plugin = nil
		ag.App = nil
		return
	}
	if !a.holdsIn(c, ag.WorkspaceID, authz.Write("agents")) {
		ag.Config = nil
	} else {
		// The agent's configuration follows its plugin's schema, so its
		// secret fields are redacted like the plugin's own
		// (plugin_config_redaction.go); a write that sends the marker back
		// keeps the stored value (restoreAgentConfig).
		ag.Config = agentSecrets(ag, red).RedactPluginConfig(ag.Config)
	}
	if ag.Plugin != nil {
		// A copy: the secret fields of the plugin's configuration are
		// redacted for every reader (plugin_config_redaction.go).
		plugin := *ag.Plugin
		if a.mayReadPluginConfig(c) {
			plugin.Config = red.config(&plugin)
		} else {
			plugin.Config = nil
		}
		ag.Plugin = &plugin
	}
	// The linked App is shown only to a caller who may see it (.55): one in
	// another Workspace is left out (app_hidden), its id kept.
	if ag.AppID != 0 {
		appWS, found := uint(0), ag.App != nil
		if found {
			appWS = ag.App.WorkspaceID
		} else {
			var err error
			if appWS, found, err = a.storedWorkspaceID(&models.App{}, ag.AppID); err != nil {
				found, appWS = true, ^uint(0) // unreadable: hide (fail closed)
			}
		}
		if found && !a.visibleIn(c, "apps", appWS) {
			ag.App = nil
			ag.AppHidden = true
		}
	}
	// A stored 0 (a row not yet backfilled) is reported as the Default's
	// id, as for every other stamped object, so the UI never reads it as
	// another Workspace (or a move).
	ag.WorkspaceID = a.responseWorkspaceID(ag.WorkspaceID)
}

// HandleAgentMessage handles POST /api/agents/:id/message - sends a message to an active agent session
func (a *API) HandleAgentMessage(c *gin.Context) {
	// Get session ID from query parameter (consistent with chat handler)
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Missing session ID", Detail: "Session ID is required"}},
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

	// Get the active session from storage
	sessionInterface, ok := agentSessions.Load(sessionID)
	if !ok {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Session not found", Detail: "No active session found. Please establish an SSE connection first."}},
		})
		return
	}

	session := sessionInterface.(*agent_session.AgentSession)

	// On the admin API the session must belong to the agent in the path,
	// which the caller must see and may execute in its Workspace.
	if onAdminAPI(c) {
		agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
		ag := session.AgentConfig()
		if err != nil || ag == nil || ag.ID != uint(agentID) {
			agentNotFound(c)
			return
		}
		var stored models.AgentConfig
		if err := a.service.DB.Select("id", "workspace_id").First(&stored, ag.ID).Error; err != nil {
			agentNotFound(c)
			return
		}
		if !a.requireAgentInWorkspace(c, &stored, authz.Execute("agents")) {
			return
		}
	}

	// Send message to agent
	if err := session.SendMessage(req.Message, req.History); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Failed to send message", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Message sent successfully",
		"session_id": sessionID,
	})
}

// HandleAgentSSE handles GET /api/agents/:id/stream - establishes SSE connection for agent communication
func (a *API) HandleAgentSSE(c *gin.Context) {
	// For SSE, the auth middleware should handle token from query parameter
	// The GetAuthenticatedUser in auth.go already checks for token in query params
	thisUser := a.auth.GetAuthenticatedUser(c)
	if thisUser == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "Authentication required"}},
		})
		return
	}

	// Load user's groups
	a.service.DB.Preload("Groups").First(thisUser, thisUser.ID)

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

	// Get session ID from query params if provided
	sessionID := c.Query("session_id")

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

	if !a.requireAgentInWorkspace(c, &agentConfig, authz.Execute("agents")) {
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
	if !agentConfig.Plugin.SupportsHookType(models.HookTypeAgent) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid plugin type", Detail: "Plugin does not support agent hook type"}},
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
		errorChunk := agent_session.AgentMessageChunk{
			Type:    "ERROR",
			Content: fmt.Sprintf("Failed to connect to agent plugin: %v", err),
			IsFinal: true,
		}
		errorJSON, _ := json.Marshal(errorChunk)
		sendSSEMessage(c.Writer, "error", string(errorJSON))
		return
	}

	// Setup service broker for this agent session
	var serviceBrokerID uint32
	if aiStudioClient, ok := pluginClient.(interface{ SetupServiceBroker(uint) (uint32, error) }); ok {
		serviceBrokerID, err = aiStudioClient.SetupServiceBroker(agentConfig.PluginID)
		if err != nil {
			slog.Error("Failed to setup service broker", "error", err, "plugin_id", agentConfig.PluginID)
			errorChunk := agent_session.AgentMessageChunk{
				Type:    "ERROR",
				Content: fmt.Sprintf("Failed to setup service broker: %v", err),
				IsFinal: true,
			}
			errorJSON, _ := json.Marshal(errorChunk)
			sendSSEMessage(c.Writer, "error", string(errorJSON))
			return
		}
		slog.Info("Service broker setup for agent", "broker_id", serviceBrokerID, "agent_id", agentID)
	} else {
		slog.Warn("Plugin client does not support service broker setup", "plugin_id", agentConfig.PluginID)
	}

	// Create queue for this session
	factory := chat_session.CreateDefaultQueueFactoryWithSharedDB(a.service.DB)
	if sessionID == "" {
		sessionID = fmt.Sprintf("agent-%d-%d", agentID, thisUser.ID)
	}
	queue, err := factory.CreateQueue(sessionID, nil)
	if err != nil {
		slog.Error("Failed to create message queue", "error", err, "session_id", sessionID)
		errorChunk := agent_session.AgentMessageChunk{
			Type:    "ERROR",
			Content: "Failed to create message queue",
			IsFinal: true,
		}
		errorJSON, _ := json.Marshal(errorChunk)
		sendSSEMessage(c.Writer, "error", string(errorJSON))
		return
	}
	defer queue.Close()

	// Create agent session with service broker ID
	session, err := agent_session.NewAgentSession(&agentConfig, pluginClient, serviceBrokerID, queue, a.service.DB)
	if err != nil {
		slog.Error("Failed to create agent session", "error", err, "agent_id", agentID)
		errorChunk := agent_session.AgentMessageChunk{
			Type:    "ERROR",
			Content: "Failed to create agent session",
			IsFinal: true,
		}
		errorJSON, _ := json.Marshal(errorChunk)
		sendSSEMessage(c.Writer, "error", string(errorJSON))
		return
	}
	defer session.Close()

	// Send session ID to client
	sessionMsg := map[string]interface{}{
		"session_id":            session.GetID(),
		"agent_id":              agentConfig.ID,
		"agent_name":            agentConfig.Name,
		"available_llms":        len(agentConfig.App.LLMs),
		"available_tools":       len(agentConfig.App.Tools),
		"available_datasources": len(agentConfig.App.Datasources),
	}
	sessionJSON, _ := json.Marshal(sessionMsg)
	slog.Info("Sending session message", "session_id", session.GetID())

	// Send session message using SSE helper
	sendSSEMessage(c.Writer, "session", string(sessionJSON))

	// Explicitly flush to ensure the session message is sent immediately
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	slog.Info("Session message sent and flushed", "session_id", session.GetID())

	// Store session in memory for message handling
	agentSessions.Store(session.GetID(), session)
	defer agentSessions.Delete(session.GetID())

	// Stream responses from queue
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	streamChan := queue.ConsumeStream(streamCtx)
	errorChan := queue.ConsumeErrors(streamCtx)

	slog.Info("Starting to consume from queue", "session_id", session.GetID(), "agent_id", agentID)

	clientGone := c.Writer.CloseNotify()

	for {
		select {
		case <-clientGone:
			slog.Info("Client disconnected", "agent_id", agentID, "session_id", session.GetID())
			return

		case <-ctx.Done():
			slog.Info("Context cancelled", "agent_id", agentID, "session_id", session.GetID())
			return

		case chunk, ok := <-streamChan:
			slog.Info("Received chunk from queue", "ok", ok, "chunk_size", len(chunk), "session_id", session.GetID())
			if !ok {
				// Stream closed - agent is done sending messages
				// But keep connection alive for client to disconnect
				slog.Info("Stream channel closed, waiting for client disconnect")
				continue
			}

			// Parse agent message chunk
			var agentChunk agent_session.AgentMessageChunk
			if err := json.Unmarshal(chunk, &agentChunk); err != nil {
				slog.Error("Failed to unmarshal agent chunk", "error", err)
				continue
			}

			slog.Info("Parsed agent chunk", "type", agentChunk.Type, "content_length", len(agentChunk.Content), "is_final", agentChunk.IsFinal)

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
			slog.Info("Sending SSE message", "event_type", eventType, "content_length", len(agentChunk.Content))

			// Send using SSE helper to properly encode newlines
			sendSSEMessage(c.Writer, eventType, string(chunkJSON))

			// Note: Don't return here even if IsFinal is true
			// Keep the connection open like chat handler does
			// The connection will close when client disconnects or context is cancelled

		case err, ok := <-errorChan:
			if !ok {
				continue
			}
			slog.Error("Error from agent queue", "error", err, "agent_id", agentID)
			errorChunk := agent_session.AgentMessageChunk{
				Type:    "ERROR",
				Content: err.Error(),
				IsFinal: true,
			}
			errorJSON, _ := json.Marshal(errorChunk)
			sendSSEMessage(c.Writer, "error", string(errorJSON))
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

	// Load user's groups
	a.service.DB.Preload("Groups").First(thisUser, thisUser.ID)

	// List agents with pagination. On the admin API the Workspace filter
	// runs in SQL, before pagination.
	db := a.service.DB
	if onAdminAPI(c) {
		filter, ok := a.workspaceFilterParam(c, "")
		if !ok {
			return
		}
		db = db.Scopes(a.workspaceListScope(c, "agents", ""), filter)
	}
	var agents models.AgentConfigs
	// Relationships load with the page (no per-agent query).
	db = db.Preload("Groups").Preload("Plugin").Preload("App")
	total, totalPages, err := agents.ListWithPagination(db, limit, page, true, namespace, nil)
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
	// One redactor for the page: the plugins' secret-field classifiers.
	listed := make([]*models.AgentConfig, len(agents))
	for i := range agents {
		listed[i] = &agents[i]
	}
	red := a.agentRedactor(listed...)
	for i := range agents {
		// Check access. On the admin API the Workspace scope (in SQL, above)
		// is the visibility rule, so totals match the page; the team check
		// is the Portal and Chat audience rule.
		hasAccess := onAdminAPI(c)
		if !hasAccess && len(agents[i].Groups) == 0 {
			hasAccess = true // Public
		} else if !hasAccess {
			for _, agentGroup := range agents[i].Groups {
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
			a.redactAgentWith(c, &agents[i], red)
			accessibleAgents = append(accessibleAgents, agents[i])
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

	// Load user's groups
	a.service.DB.Preload("Groups").First(thisUser, thisUser.ID)

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
	if err := a.service.DB.Preload("Plugin").Preload("App").Preload("Groups").First(&agentConfig, uint(agentID)).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Agent not found", Detail: "No agent found with the provided ID"}},
		})
		return
	}
	if !a.requireAgentInWorkspace(c, &agentConfig, authz.Read("agents")) {
		return
	}

	// Check user has access: on the admin API the Workspace checks above
	// are the rule; elsewhere the agent's teams.
	hasAccess := onAdminAPI(c)
	if !hasAccess && len(agentConfig.Groups) == 0 {
		hasAccess = true
	} else if !hasAccess {
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

	a.redactAgent(c, &agentConfig)
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
	_ = uObj // identity is checked; authorization is the permission check below

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
	if !plugin.SupportsHookType(models.HookTypeAgent) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid plugin type", Detail: "Plugin does not support agent hook type"}},
		})
		return
	}
	// Saving an agent needs no plugin read (plugins are global, .36); the
	// plugin must be enabled and agent-capable instead (security .33).
	if !plugin.IsActive {
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid plugin", Detail: "Plugin is not enabled"}},
		})
		return
	}

	// The Workspace the agent lands in (decision D5): agents:write there.
	workspaceID, ok := a.resolveCreateWorkspace(c, "agents", req.WorkspaceID)
	if !ok {
		return
	}
	if len(req.GroupIDs) > 0 && !a.requireTeamsWrite(c, nil, req.GroupIDs) {
		return
	}

	// Verify app exists and the caller can see it: an App in a Workspace
	// the caller cannot see is reported as not found, never as forbidden.
	var app models.App
	if err := a.service.DB.First(&app, req.AppID).Error; err != nil || !a.visibleIn(c, "apps", app.WorkspaceID) {
		if c.IsAborted() {
			return
		}
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid app", Detail: "App not found"}},
		})
		return
	}

	// Asking for an active agent explicitly is the publish action on agents,
	// in the agent's Workspace; an omitted switch means "active if allowed,
	// draft otherwise".
	if req.IsActive != nil && !a.requirePublishToCreateLiveIn(c, "agents", workspaceID, *req.IsActive) {
		return
	}

	// Create agent config
	agentConfig := models.NewAgentConfig()
	agentConfig.Name = req.Name
	agentConfig.Slug = slug.Make(req.Name)
	agentConfig.Description = req.Description
	agentConfig.PluginID = req.PluginID
	agentConfig.AppID = req.AppID
	// A redaction marker in a new agent's secret field has no stored value
	// to stand for; it is dropped, never stored as the credential.
	newConfig, ok := a.restorePluginConfig(c, a.pluginCommand(req.PluginID), req.Config, nil)
	if !ok {
		return
	}
	agentConfig.Config = newConfig
	if agentConfig.Config == nil {
		agentConfig.Config = make(map[string]interface{})
	}
	agentConfig.IsActive = req.wantsActive()
	agentConfig.Namespace = req.Namespace
	agentConfig.WorkspaceID = workspaceID

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

	// The is_active column defaults to true on insert, which turns an
	// explicit false into true; write the requested value back when it was
	// false, and never leave a caller without publish holding a live agent.
	if !req.wantsActive() || !a.holdsIn(c, workspaceID, authz.Publish("agents")) {
		if err := agentConfig.Deactivate(a.service.DB); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Database error", Detail: err.Error()}},
			})
			return
		}
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

	a.redactAgent(c, agentConfig)
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
	_ = uObj // identity is checked; authorization is the permission check below

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

	// Hidden (404) unless the caller sees its Workspace; write there.
	if !a.requireAgentInWorkspace(c, &agentConfig, authz.Write("agents")) {
		return
	}
	// agentConfig.Get preloaded the agent's teams.
	if req.GroupIDs != nil && !a.requireTeamsWrite(c, agentConfig.Groups, req.GroupIDs) {
		return
	}
	// A different workspace_id is a move: write in both Workspaces.
	storedWorkspaceID := agentConfig.WorkspaceID // raw: the move is guarded on it
	move := req.WorkspaceID != nil && *req.WorkspaceID != agentConfig.WorkspaceID
	if move && !a.requireMove(c, "agents", agentConfig.WorkspaceID, *req.WorkspaceID) {
		return
	}

	// Flipping the active switch is the publish action on agents, in the
	// agent's Workspace; an omitted switch keeps its value.
	if req.IsActive != nil {
		if !a.requirePublishIfChangedIn(c, "agents", agentConfig.WorkspaceID, agentConfig.IsActive, *req.IsActive) {
			return
		}
	}

	// Update fields
	agentConfig.Name = req.Name
	agentConfig.Slug = slug.Make(req.Name)
	agentConfig.Description = req.Description
	// The API returns the configuration's secret fields redacted; a write
	// that sends the marker back keeps the stored value.
	restoredConfig, ok := a.restorePluginConfig(c, a.pluginCommand(agentConfig.PluginID), req.Config, agentConfig.Config)
	if !ok {
		return
	}
	agentConfig.Config = restoredConfig
	if req.IsActive != nil {
		agentConfig.IsActive = *req.IsActive
	}
	if req.Namespace != "" {
		agentConfig.Namespace = req.Namespace
	}

	// A move goes first (workspace_id with updated_at, guarded on the stored
	// value) and is undone if the update then fails.
	var moveTo *uint
	if move {
		moveTo = req.WorkspaceID
	}
	finishMove, ok := a.beginMove(c, &models.AgentConfig{}, agentConfig.ID, storedWorkspaceID, moveTo)
	if !ok {
		return
	}

	// Update in database
	if err := agentConfig.Update(a.service.DB); err != nil {
		finishMove(false)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Database error", Detail: err.Error()}},
		})
		return
	}
	finishMove(true)

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
	agentConfig = models.AgentConfig{}
	a.service.DB.Preload("Plugin").Preload("App").Preload("Groups").First(&agentConfig, uint(agentID))

	a.redactAgent(c, &agentConfig)
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
	_ = uObj // identity is checked; authorization is the permission check below

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
	if !a.requireAgentInWorkspace(c, &agentConfig, authz.Delete("agents")) {
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
	_ = uObj // identity is checked; authorization is the permission check below

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
	if !a.requireAgentInWorkspace(c, &agentConfig, authz.Publish("agents")) {
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
	_ = uObj // identity is checked; authorization is the permission check below

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
	if !a.requireAgentInWorkspace(c, &agentConfig, authz.Publish("agents")) {
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
