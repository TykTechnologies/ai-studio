package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/agent_session"
	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// v2 agent API: the same session/run/history shape as the v2 chat API, so
// the browser uses one runtime for chat rooms and plugin agents. Agent
// sessions live in the SessionHub between turns and keep an in-memory
// transcript for the session's lifetime (agents have no persisted history).

// SetupAgentV2Routes registers the v2 agent endpoints.
func (a *API) SetupAgentV2Routes(r *gin.RouterGroup) {
	r.POST("/agents/:id/sessions", a.createAgentSessionV2)
	r.POST("/agent-sessions/:session_id/runs", a.runAgentTurnV2)
	r.POST("/agent-sessions/:session_id/cancel", a.cancelAgentRunV2)
	r.GET("/agent-sessions/:session_id/messages/v2", a.getAgentMessagesV2)
}

// currentUser returns the authenticated user from the context (set by the
// /common middleware) or from the auth service.
func (a *API) currentUser(c *gin.Context) *models.User {
	if uObj, ok := c.Get("user"); ok {
		if u, ok := uObj.(*models.User); ok {
			return u
		}
	}
	if a.auth != nil {
		return a.auth.GetAuthenticatedUser(c)
	}
	return nil
}

// loadAccessibleAgent loads an active agent the user may use, writing the
// error response itself on failure.
func (a *API) loadAccessibleAgent(c *gin.Context, user *models.User) (*models.AgentConfig, bool) {
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid agent ID", "Agent ID must be a valid number")
		return nil, false
	}
	var agentConfig models.AgentConfig
	if err := a.service.DB.
		Preload("App.LLMs").
		Preload("App.Tools").
		Preload("App.Datasources").
		Preload("App.Credential").
		Preload("Plugin").
		Preload("Groups").
		First(&agentConfig, uint(agentID)).Error; err != nil {
		jsonError(c, http.StatusNotFound, "Agent not found", "No agent found with the provided ID")
		return nil, false
	}
	if !agentConfig.IsActive {
		jsonError(c, http.StatusForbidden, "Agent inactive", "This agent is not currently active")
		return nil, false
	}
	if agentConfig.Plugin == nil || !agentConfig.Plugin.IsActive {
		jsonError(c, http.StatusForbidden, "Plugin inactive", "The agent's plugin is not active")
		return nil, false
	}
	if !agentConfig.Plugin.SupportsHookType(models.HookTypeAgent) {
		jsonError(c, http.StatusBadRequest, "Invalid plugin type", "Plugin does not support agent hook type")
		return nil, false
	}
	if len(agentConfig.Groups) > 0 {
		a.service.DB.Preload("Groups").First(user, user.ID)
		hasAccess := false
		for _, ag := range agentConfig.Groups {
			for _, ug := range user.Groups {
				if ag.ID == ug.ID {
					hasAccess = true
					break
				}
			}
			if hasAccess {
				break
			}
		}
		if !hasAccess {
			jsonError(c, http.StatusForbidden, "Access denied", "You don't have access to this agent")
			return nil, false
		}
	}
	return &agentConfig, true
}

func agentSessionResponse(as *agent_session.AgentSession) V2SessionResponse {
	cfg := as.AgentConfig()
	resp := V2SessionResponse{
		SessionID:   as.ID(),
		Tools:       []V2ToolSummary{},
		Datasources: []V2SourceSummary{},
		ClientTools: []V2ClientToolInfo{},
		Chat: V2ChatSummary{
			ID: cfg.ID, Name: cfg.Name, Description: cfg.Description,
			ToolSupport: false, PromptTemplates: []models.PromptTemplate{},
		},
	}
	if cfg.App != nil {
		for _, t := range cfg.App.Tools {
			resp.Tools = append(resp.Tools, V2ToolSummary{ID: t.ID, Name: t.Name, Description: t.Description, ToolType: t.ToolType})
		}
		for _, ds := range cfg.App.Datasources {
			resp.Datasources = append(resp.Datasources, V2SourceSummary{ID: ds.ID, Name: ds.Name, ShortDescription: ds.ShortDescription})
		}
	}
	return resp
}

// acquireAgentSession returns the live agent session or writes a 404: agent
// sessions cannot be reloaded once they leave the hub.
func (a *API) acquireAgentSession(c *gin.Context, sessionID string, user *models.User) (*agent_session.AgentSession, func(), bool) {
	hs, release, err := getChatHub().Acquire(sessionID, func() (HubSession, error) {
		return nil, fmt.Errorf("expired")
	})
	if err != nil {
		jsonError(c, http.StatusNotFound, "Session not found", "The agent session has expired. Start a new session.")
		return nil, nil, false
	}
	as, ok := hs.(*agent_session.AgentSession)
	if !ok {
		release()
		jsonError(c, http.StatusConflict, "Session mismatch", "Session is not an agent session")
		return nil, nil, false
	}
	if as.AgentConfig() == nil || as.OwnerID() != user.ID {
		release()
		jsonError(c, http.StatusForbidden, "Forbidden", "You don't have permission to access this session")
		return nil, nil, false
	}
	return as, release, true
}

// createAgentSessionV2 handles POST /agents/:id/sessions.
func (a *API) createAgentSessionV2(c *gin.Context) {
	user := a.currentUser(c)
	if user == nil {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "Authentication required")
		return
	}
	agentConfig, ok := a.loadAccessibleAgent(c, user)
	if !ok {
		return
	}

	var body struct {
		SessionID string `json:"session_id"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			jsonError(c, http.StatusBadRequest, "Invalid input", err.Error())
			return
		}
	}
	if body.SessionID != "" {
		as, release, ok := a.acquireAgentSession(c, body.SessionID, user)
		if !ok {
			return
		}
		defer release()
		c.JSON(http.StatusOK, agentSessionResponse(as))
		return
	}

	pluginClient, err := a.service.GetPluginClient(agentConfig.PluginID)
	if err != nil {
		slog.Error("Failed to get plugin client", "error", err, "plugin_id", agentConfig.PluginID)
		jsonError(c, http.StatusServiceUnavailable, "Agent unavailable", fmt.Sprintf("Failed to connect to agent plugin: %v", err))
		return
	}
	var serviceBrokerID uint32
	if brokered, ok := pluginClient.(interface{ SetupServiceBroker() (uint32, error) }); ok {
		serviceBrokerID, err = brokered.SetupServiceBroker()
		if err != nil {
			slog.Error("Failed to setup service broker", "error", err, "plugin_id", agentConfig.PluginID)
			jsonError(c, http.StatusServiceUnavailable, "Agent unavailable", "Failed to setup service broker")
			return
		}
	}

	sessionID := agent_session.NewSessionID()
	queue, err := chat_session.CreateDefaultQueueFactoryWithSharedDB(a.service.DB).CreateQueue(sessionID, nil)
	if err != nil {
		slog.Error("Failed to create message queue", "error", err, "session_id", sessionID)
		jsonError(c, http.StatusInternalServerError, "Session error", "Failed to create message queue")
		return
	}
	as, err := agent_session.NewAgentSessionWithID(sessionID, agentConfig, pluginClient, serviceBrokerID, queue, a.service.DB)
	if err != nil {
		queue.Close()
		jsonError(c, http.StatusInternalServerError, "Session error", "Failed to create agent session")
		return
	}
	as.SetOwnerID(user.ID)
	hs, release := getChatHub().Add(as)
	defer release()
	c.JSON(http.StatusCreated, agentSessionResponse(hs.(*agent_session.AgentSession)))
}

// getAgentMessagesV2 handles GET /agent-sessions/:session_id/messages/v2.
func (a *API) getAgentMessagesV2(c *gin.Context) {
	user := a.currentUser(c)
	if user == nil {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "Authentication required")
		return
	}
	sessionID := c.Param("session_id")
	as, release, ok := a.acquireAgentSession(c, sessionID, user)
	if !ok {
		return
	}
	defer release()
	c.JSON(http.StatusOK, gin.H{"session_id": sessionID, "messages": transcriptToV2(as.Transcript())})
}

// cancelAgentRunV2 handles POST /agent-sessions/:session_id/cancel.
func (a *API) cancelAgentRunV2(c *gin.Context) {
	user := a.currentUser(c)
	if user == nil {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "Authentication required")
		return
	}
	hs, ok := getChatHub().Get(c.Param("session_id"))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"cancelled": false})
		return
	}
	as, isAgent := hs.(*agent_session.AgentSession)
	if !isAgent || as.OwnerID() != user.ID {
		c.JSON(http.StatusOK, gin.H{"cancelled": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": as.CancelRun()})
}

// runAgentTurnV2 handles POST /agent-sessions/:session_id/runs.
func (a *API) runAgentTurnV2(c *gin.Context) {
	user := a.currentUser(c)
	if user == nil {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "Authentication required")
		return
	}
	var req V2RunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid input", err.Error())
		return
	}
	if req.Regenerate || len(req.ToolResults) > 0 || req.AfterMessageID != nil {
		jsonError(c, http.StatusBadRequest, "Unsupported", "agents support plain turns only")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		jsonError(c, http.StatusBadRequest, "Invalid input", "message is required")
		return
	}

	sessionID := c.Param("session_id")
	as, release, ok := a.acquireAgentSession(c, sessionID, user)
	if !ok {
		return
	}
	defer release()

	if !as.TryLockRun() {
		jsonError(c, http.StatusConflict, "Run in progress", "Another turn is being processed for this session")
		return
	}
	defer as.UnlockRun()

	// Subscribe before starting so no chunk is missed.
	streamCtx, streamCancel := context.WithCancel(c.Request.Context())
	defer streamCancel()
	chunks := as.GetQueue().ConsumeStream(streamCtx)
	errs := as.GetQueue().ConsumeErrors(streamCtx)

	done, err := as.StartRun(req.Message)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Failed to send message", err.Error())
		return
	}

	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	h.Set("Content-Encoding", "none")
	h.Set(uiMessageStreamHeader, "v1")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	runID := chat_session.NewRunID()
	w := newUIStreamWriter(c.Writer, runID)
	w.handle(chat_session.ChatEvent{Kind: chat_session.EventStart})

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	clientGone := c.Request.Context().Done()

	finish := func(reason string) {
		w.handle(chat_session.ChatEvent{Kind: chat_session.EventFinish, Data: mustJSON(chat_session.FinishData{Reason: reason})})
	}

	for {
		select {
		case b, ok := <-chunks:
			if !ok {
				finish(chat_session.FinishError)
				return
			}
			var chunk agent_session.AgentMessageChunk
			if err := json.Unmarshal(b, &chunk); err != nil {
				continue
			}
			for _, ev := range agentChunkEvents(chunk) {
				if w.handle(ev) {
					return
				}
			}
			if chunk.IsFinal || strings.EqualFold(chunk.Type, agent_session.ChunkDone) {
				finish(chat_session.FinishStop)
				return
			}
		case err, ok := <-errs:
			if !ok {
				finish(chat_session.FinishError)
				return
			}
			w.handle(chat_session.ChatEvent{Kind: chat_session.EventError, Data: mustJSON(chat_session.ErrorData{Code: chat_session.ClassifyError(err), Message: err.Error()})})
		case err := <-done:
			// The plugin closed its stream (possibly without a DONE chunk).
			// Drain anything still queued, then finish.
			drainAgentChunks(w, chunks)
			if err != nil {
				w.handle(chat_session.ChatEvent{Kind: chat_session.EventError, Data: mustJSON(chat_session.ErrorData{Code: chat_session.ClassifyError(err), Message: err.Error()})})
				finish(chat_session.FinishError)
			} else {
				finish(chat_session.FinishStop)
			}
			return
		case <-keepalive.C:
			w.comment("keepalive")
		case <-clientGone:
			as.CancelRun()
			return
		case <-deadline.C:
			as.CancelRun()
			w.error("turn exceeded the maximum duration")
			finish(chat_session.FinishError)
			return
		}
	}
}

func drainAgentChunks(w *uiStreamWriter, chunks <-chan []byte) {
	for {
		select {
		case b, ok := <-chunks:
			if !ok {
				return
			}
			var chunk agent_session.AgentMessageChunk
			if err := json.Unmarshal(b, &chunk); err != nil {
				continue
			}
			for _, ev := range agentChunkEvents(chunk) {
				w.handle(ev)
			}
		default:
			return
		}
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// agentChunkEvents maps one plugin chunk onto the v2 event vocabulary.
func agentChunkEvents(chunk agent_session.AgentMessageChunk) []chat_session.ChatEvent {
	id, _ := chunk.Metadata[agent_session.MetadataToolCallID].(string)
	name, _ := chunk.Metadata["tool_name"].(string)
	if name == "" {
		name = "tool"
	}
	switch strings.ToUpper(chunk.Type) {
	case agent_session.ChunkContent:
		if chunk.Content == "" {
			return nil
		}
		return []chat_session.ChatEvent{{Kind: chat_session.EventTextDelta, Data: mustJSON(chat_session.TextDeltaData{Delta: chunk.Content})}}
	case agent_session.ChunkThinking:
		if chunk.Content == "" {
			return nil
		}
		return []chat_session.ChatEvent{{Kind: chat_session.EventReasoningDelta, Data: mustJSON(chat_session.TextDeltaData{Delta: chunk.Content})}}
	case agent_session.ChunkToolCall:
		argsText := chunk.Content
		if params, ok := chunk.Metadata["parameters"]; ok && params != nil {
			if b, err := json.Marshal(params); err == nil {
				argsText = string(b)
			}
		}
		if id == "" {
			id = "call_" + chat_session.NewRunID()
		}
		return []chat_session.ChatEvent{
			{Kind: chat_session.EventToolCallStart, Data: mustJSON(chat_session.ToolCallStartData{ToolCallID: id, ToolName: name})},
			{Kind: chat_session.EventToolCallDelta, Data: mustJSON(chat_session.ToolCallDeltaData{ToolCallID: id, ArgsText: argsText})},
			{Kind: chat_session.EventToolCallEnd, Data: mustJSON(chat_session.ToolCallEndData{ToolCallID: id})},
		}
	case agent_session.ChunkToolResult:
		isErr, _ := chunk.Metadata["is_error"].(bool)
		if id == "" {
			id = "call_" + chat_session.NewRunID()
		}
		return []chat_session.ChatEvent{{Kind: chat_session.EventToolResult, Data: mustJSON(chat_session.ToolResultData{ToolCallID: id, Result: chunk.Content, IsError: isErr, Bytes: len(chunk.Content)})}}
	case agent_session.ChunkError:
		return []chat_session.ChatEvent{{Kind: chat_session.EventError, Data: mustJSON(chat_session.ErrorData{Code: chat_session.ErrCodeInternal, Message: chunk.Content})}}
	}
	return nil
}

// transcriptToV2 converts the session's transcript into v2 messages.
func transcriptToV2(msgs []agent_session.TranscriptMessage) []V2Message {
	out := make([]V2Message, 0, len(msgs))
	for _, m := range msgs {
		v := V2Message{ID: m.ID, Role: m.Role, CreatedAt: m.CreatedAt, Parts: []V2Part{}}
		for _, p := range m.Parts {
			switch p.Type {
			case "text":
				v.Parts = append(v.Parts, V2Part{Type: "text", Text: p.Text})
			case "reasoning":
				// The v2 history part vocabulary has no reasoning part; keep it as a data part.
				v.Parts = append(v.Parts, V2Part{Type: "data", Name: "reasoning", Data: mustJSON(map[string]string{"text": p.Text})})
			case "tool-call":
				part := V2Part{Type: "tool-call", ToolCallID: p.ToolCallID, ToolName: p.ToolName, ArgsText: p.ArgsText, IsError: p.IsError}
				if p.Args != nil {
					part.Args = mustJSON(p.Args)
				} else {
					part.Args = json.RawMessage(`{}`)
				}
				if p.HasResult {
					part.Result = mustJSON(p.Result)
				}
				v.Parts = append(v.Parts, part)
			case "data":
				v.Parts = append(v.Parts, V2Part{Type: "data", Name: p.Name, Data: mustJSON(p.Data)})
			}
		}
		out = append(out, v)
	}
	return out
}
