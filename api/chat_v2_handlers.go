package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// v2 chat API
//
// The v1 API keeps one long-lived SSE per browser tab and POSTs messages
// beside it. v2 is shaped for assistant-ui's LocalRuntime instead: a session is
// created (or resumed) with a plain JSON call, and every user turn is one POST
// whose response streams that turn alone, in the AI SDK "UI message stream"
// format (SSE, `data: {"type": ...}` events, terminated by `data: [DONE]`).
// Sessions live in the SessionHub between turns.

const uiMessageStreamHeader = "x-vercel-ai-ui-message-stream"

// V2SessionResponse is returned by POST /chat/:chat_id/sessions.
type V2SessionResponse struct {
	SessionID   string             `json:"session_id"`
	Tools       []V2ToolSummary    `json:"tools"`
	Datasources []V2SourceSummary  `json:"datasources"`
	ClientTools []V2ClientToolInfo `json:"client_tools"`
	// PendingToolCallIDs lists client tool calls still waiting for an answer
	// when a session is resumed, so the UI can offer them again.
	PendingToolCallIDs []string      `json:"pending_tool_call_ids,omitempty"`
	Chat               V2ChatSummary `json:"chat"`
}

type V2ToolSummary struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ToolType    string `json:"tool_type"`
}

type V2SourceSummary struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	ShortDescription string `json:"short_description"`
}

// V2ClientToolInfo describes a tool the browser executes (human-in-the-loop).
type V2ClientToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	UI          json.RawMessage `json:"ui,omitempty"`
}

type V2ChatSummary struct {
	ID              uint                    `json:"id"`
	Name            string                  `json:"name"`
	Description     string                  `json:"description"`
	ToolSupport     bool                    `json:"tool_support"`
	PromptTemplates []models.PromptTemplate `json:"prompt_templates"`
}

// V2RunRequest is the body of POST /chat-sessions/:session_id/runs.
//
// Exactly one of these shapes is expected:
//   - {message, file_refs?}: append a user turn and run.
//   - {message, after_message_id}: an edit; rewind the history to just after
//     the given message id ("" or "root" = the start) first.
//   - {regenerate: true, after_message_id?}: re-run the model on the history
//     rewound to after the given id (default: after the last user turn).
//   - {tool_results}: resume a turn waiting on client-side tools.
type V2RunRequest struct {
	Message        string         `json:"message"`
	FileRefs       []string       `json:"file_refs"`
	Regenerate     bool           `json:"regenerate"`
	AfterMessageID *string        `json:"after_message_id"`
	ToolResults    []V2ToolResult `json:"tool_results"`
}

// chatUIV2Enabled reads CHAT_UI_V2_ENABLED; the new chat UI is on unless the
// variable is explicitly false.
func chatUIV2Enabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CHAT_UI_V2_ENABLED")))
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

type V2ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Result     json.RawMessage `json:"result"`
	IsError    bool            `json:"is_error"`
}

// SetupChatV2Routes registers the v2 endpoints on the same authenticated group
// as the v1 chat routes.
func (a *API) SetupChatV2Routes(r *gin.RouterGroup) {
	r.POST("/chat/:chat_id/sessions", a.createChatSessionV2)
	r.POST("/chat-sessions/:session_id/runs", a.runChatTurnV2)
	r.POST("/chat-sessions/:session_id/cancel", a.cancelChatRunV2)
	r.GET("/chat-sessions/:id/messages/v2", a.getChatMessagesV2)
}

func idString(id uint) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(id), 10)
}

func chatSummary(chat *models.Chat) V2ChatSummary {
	templates, err := chat.GetPromptTemplates()
	if err != nil || templates == nil {
		templates = []models.PromptTemplate{}
	}
	return V2ChatSummary{
		ID: chat.ID, Name: chat.Name, Description: chat.Description,
		ToolSupport: chat.SupportsTools, PromptTemplates: templates,
	}
}

func sessionResponse(cs *chat_session.ChatSession, chat *models.Chat) V2SessionResponse {
	resp := V2SessionResponse{
		SessionID:   cs.ID(),
		Tools:       []V2ToolSummary{},
		Datasources: []V2SourceSummary{},
		ClientTools: []V2ClientToolInfo{},
		Chat:        chatSummary(chat),
	}
	for _, t := range cs.CurrentTools() {
		resp.Tools = append(resp.Tools, V2ToolSummary{ID: t.ID, Name: t.Name, Description: t.Description, ToolType: t.ToolType})
	}
	for _, ds := range cs.GetCurrentDatasources() {
		resp.Datasources = append(resp.Datasources, V2SourceSummary{ID: ds.ID, Name: ds.Name, ShortDescription: ds.ShortDescription})
	}
	for _, ct := range cs.ClientTools() {
		schema, _ := json.Marshal(ct.Schema)
		ui, _ := json.Marshal(ct.UI)
		resp.ClientTools = append(resp.ClientTools, V2ClientToolInfo{Name: ct.Name, Description: ct.Description, Schema: schema, UI: ui})
	}
	resp.PendingToolCallIDs = cs.PendingClientCallIDs()
	return resp
}

// assertSessionOwner rejects access to a persisted session that belongs to
// another user. A session with no history record yet is allowed through (it
// is brand new and only reachable through the hub).
func (a *API) assertSessionOwner(c *gin.Context, sessionID string) bool {
	uObj, ok := c.Get("user")
	if !ok {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "User not found")
		return false
	}
	record, err := a.service.GetChatHistoryRecordBySessionID(sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true
		}
		jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return false
	}
	if record.UserID != uObj.(*models.User).ID {
		jsonError(c, http.StatusForbidden, "Forbidden", "You don't have permission to access this session")
		return false
	}
	return true
}

// createChatSessionV2 handles POST /chat/:chat_id/sessions. With a session_id
// in the body it resumes that session; otherwise it starts a new one.
func (a *API) createChatSessionV2(c *gin.Context) {
	uObj, ok := c.Get("user")
	if !ok {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "User not found")
		return
	}
	userID := uObj.(*models.User).ID

	chatID, err := strconv.ParseUint(c.Param("chat_id"), 10, 32)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid chat ID", "Chat ID must be a valid number")
		return
	}
	chat, err := a.service.GetChatByID(uint(chatID))
	if err != nil {
		jsonError(c, http.StatusNotFound, "Chat not found", "No chat found with the provided ID")
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
		if !a.assertSessionOwner(c, body.SessionID) {
			return
		}
		cs, release, ok := a.acquireChatSession(c, body.SessionID, chat_session.OutputModeEvents)
		if !ok {
			return
		}
		defer release()
		c.JSON(http.StatusOK, sessionResponse(cs, chat))
		return
	}

	created, err := a.createNewSession(chat, userID)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Session error", "Failed to create new session")
		return
	}
	created.SetOutputMode(chat_session.OutputModeEvents)
	if err := created.Start(); err != nil {
		created.Stop()
		slog.Error("failed to start chat session", "chat_id", chat.ID, "error", err)
		// The reason is a configuration problem the user can act on (missing
		// provider key, privacy score mismatch), so surface it.
		jsonError(c, http.StatusInternalServerError, "Session error", "Failed to start chat session: "+err.Error())
		return
	}
	hs, release := getChatHub().Add(created)
	defer release()
	c.JSON(http.StatusCreated, sessionResponse(hs.(*chat_session.ChatSession), chat))
}

// getChatMessagesV2 handles GET /chat-sessions/:session_id/messages/v2.
func (a *API) getChatMessagesV2(c *gin.Context) {
	sessionID := c.Param("id") // the GET tree already uses :id under /chat-sessions
	uObj, ok := c.Get("user")
	if !ok {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "User not found")
		return
	}
	record, err := a.service.GetChatHistoryRecordBySessionID(sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			jsonError(c, http.StatusNotFound, "Not Found", "Session not found")
		} else {
			jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		}
		return
	}
	if record.UserID != uObj.(*models.User).ID {
		jsonError(c, http.StatusForbidden, "Forbidden", "You don't have permission to access this session")
		return
	}
	// Pages backwards from the newest row: ?limit=N (default 200, max 1000)
	// and ?before=<row id> for the page preceding an earlier one.
	limit := 200
	if v, err := strconv.Atoi(c.DefaultQuery("limit", "200")); err == nil && v > 0 {
		limit = v
	}
	if limit > 1000 {
		limit = 1000
	}
	var before uint
	if v, err := strconv.ParseUint(c.DefaultQuery("before", "0"), 10, 64); err == nil {
		before = uint(v)
	}
	rows, hasMore, err := a.service.GetCMessagesForSessionPage(sessionID, before, limit)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	resp := gin.H{"session_id": sessionID, "messages": materialiseHistory(rows), "has_more": hasMore}
	if hasMore && len(rows) > 0 {
		resp["next_before"] = rows[0].ID
	}
	c.JSON(http.StatusOK, resp)
}

// cancelChatRunV2 handles POST /chat-sessions/:session_id/cancel.
func (a *API) cancelChatRunV2(c *gin.Context) {
	sessionID := c.Param("session_id")
	if !a.assertSessionOwner(c, sessionID) {
		return
	}
	hs, ok := getChatHub().Get(sessionID)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"cancelled": false})
		return
	}
	cs, isChat := hs.(*chat_session.ChatSession)
	if !isChat {
		c.JSON(http.StatusOK, gin.H{"cancelled": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": cs.CancelRun()})
}

// runChatTurnV2 handles POST /chat-sessions/:session_id/runs: it queues one
// user turn and streams that turn's events until it finishes.
func (a *API) runChatTurnV2(c *gin.Context) {
	sessionID := c.Param("session_id")

	var req V2RunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid input", err.Error())
		return
	}
	resuming := len(req.ToolResults) > 0
	if !resuming && !req.Regenerate && strings.TrimSpace(req.Message) == "" && len(req.FileRefs) == 0 {
		jsonError(c, http.StatusBadRequest, "Invalid input", "message is required")
		return
	}
	var afterID *uint
	if req.AfterMessageID != nil {
		raw := strings.TrimSpace(*req.AfterMessageID)
		var id uint
		if raw != "" && raw != "root" {
			n, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				jsonError(c, http.StatusBadRequest, "Invalid input", "after_message_id must be a message id")
				return
			}
			id = uint(n)
		}
		afterID = &id
	}
	if !a.assertSessionOwner(c, sessionID) {
		return
	}

	cs, release, ok := a.acquireChatSession(c, sessionID, chat_session.OutputModeEvents)
	if !ok {
		return
	}
	defer release()

	if !cs.TryLockRun() {
		jsonError(c, http.StatusConflict, "Run in progress", "Another turn is being processed for this session")
		return
	}
	defer cs.UnlockRun()

	var toolResults []models.ToolResult
	if resuming {
		if !cs.AwaitingClientTools() {
			jsonError(c, http.StatusConflict, "Nothing to resume", "No client tool call is waiting for a result")
			return
		}
		for _, r := range req.ToolResults {
			result := strings.TrimSpace(string(r.Result))
			// A JSON string is unwrapped so the model sees the text itself.
			var s string
			if json.Unmarshal(r.Result, &s) == nil {
				result = s
			}
			toolResults = append(toolResults, models.ToolResult{ToolCallID: r.ToolCallID, Result: result, IsError: r.IsError})
		}
	}

	if req.Regenerate && afterID == nil {
		last, err := a.service.LastUserMessageID(sessionID)
		if err != nil {
			jsonError(c, http.StatusInternalServerError, "Regenerate failed", err.Error())
			return
		}
		if last == 0 {
			jsonError(c, http.StatusBadRequest, "Invalid input", "nothing to regenerate")
			return
		}
		afterID = &last
	}
	if afterID != nil {
		if _, err := a.service.TruncateAfterMessage(sessionID, *afterID); err != nil {
			jsonError(c, http.StatusInternalServerError, "Rewind failed", err.Error())
			return
		}
	}

	runID := chat_session.NewRunID()
	events, unsubscribe := cs.Subscribe(runID, 1024)
	defer unsubscribe()

	select {
	case cs.Input() <- &models.UserMessage{Payload: req.Message, FileRef: req.FileRefs, RunID: runID, Regenerate: req.Regenerate, ToolResults: toolResults}:
	case <-c.Request.Context().Done():
		return
	case <-time.After(10 * time.Second):
		jsonError(c, http.StatusServiceUnavailable, "Busy", "Session input queue is full")
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

	w := newUIStreamWriter(c.Writer, runID)
	clientGone := c.Request.Context().Done()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				w.finish(chat_session.FinishError)
				return
			}
			if done := w.handle(ev); done {
				return
			}
		case <-keepalive.C:
			w.comment("keepalive")
		case <-clientGone:
			cs.CancelRun()
			return
		case <-deadline.C:
			cs.CancelRun()
			w.error("turn exceeded the maximum duration")
			w.finish(chat_session.FinishError)
			return
		}
	}
}

// uiStreamWriter turns ChatEvents into AI SDK UI-message-stream chunks.
type uiStreamWriter struct {
	w        http.ResponseWriter
	flusher  http.Flusher
	runID    string
	textSeq  int
	textOpen string // id of the open text part, "" when none
	args     map[string]*strings.Builder
	finished bool
}

func newUIStreamWriter(w http.ResponseWriter, runID string) *uiStreamWriter {
	f, _ := w.(http.Flusher)
	return &uiStreamWriter{w: w, flusher: f, runID: runID, args: map[string]*strings.Builder{}}
}

func (u *uiStreamWriter) chunk(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("ui stream marshal failed", "error", err)
		return
	}
	fmt.Fprintf(u.w, "data: %s\n\n", b)
	if u.flusher != nil {
		u.flusher.Flush()
	}
}

func (u *uiStreamWriter) comment(s string) {
	fmt.Fprintf(u.w, ": %s\n\n", s)
	if u.flusher != nil {
		u.flusher.Flush()
	}
}

func (u *uiStreamWriter) error(msg string) {
	u.chunk(gin.H{"type": "error", "errorText": msg})
}

func (u *uiStreamWriter) closeText() {
	if u.textOpen != "" {
		u.chunk(gin.H{"type": "text-end", "id": u.textOpen})
		u.textOpen = ""
	}
}

func (u *uiStreamWriter) finish(reason string) {
	if u.finished {
		return
	}
	u.finished = true
	u.closeText()
	u.chunk(gin.H{"type": "finish-step"})
	u.chunk(gin.H{"type": "finish", "finishReason": reason})
	fmt.Fprint(u.w, "data: [DONE]\n\n")
	if u.flusher != nil {
		u.flusher.Flush()
	}
}

// handle writes the chunk(s) for one event and reports whether the turn ended.
func (u *uiStreamWriter) handle(ev chat_session.ChatEvent) bool {
	switch ev.Kind {
	case chat_session.EventStart:
		u.chunk(gin.H{"type": "start", "messageId": u.runID})
		u.chunk(gin.H{"type": "start-step"})

	case chat_session.EventTextDelta:
		var d chat_session.TextDeltaData
		_ = json.Unmarshal(ev.Data, &d)
		if u.textOpen == "" {
			u.textSeq++
			u.textOpen = fmt.Sprintf("%s-t%d", u.runID, u.textSeq)
			u.chunk(gin.H{"type": "text-start", "id": u.textOpen})
		}
		u.chunk(gin.H{"type": "text-delta", "id": u.textOpen, "delta": d.Delta})

	case chat_session.EventTextEnd:
		u.closeText()

	case chat_session.EventReasoningDelta:
		var d chat_session.TextDeltaData
		_ = json.Unmarshal(ev.Data, &d)
		u.chunk(gin.H{"type": "reasoning-delta", "id": u.runID + "-r", "delta": d.Delta})

	case chat_session.EventToolCallStart:
		u.closeText()
		var d chat_session.ToolCallStartData
		_ = json.Unmarshal(ev.Data, &d)
		u.args[d.ToolCallID] = &strings.Builder{}
		u.chunk(gin.H{"type": "tool-input-start", "toolCallId": d.ToolCallID, "toolName": d.ToolName})

	case chat_session.EventToolCallDelta:
		var d chat_session.ToolCallDeltaData
		_ = json.Unmarshal(ev.Data, &d)
		if b, ok := u.args[d.ToolCallID]; ok {
			b.WriteString(d.ArgsText)
		}
		u.chunk(gin.H{"type": "tool-input-delta", "toolCallId": d.ToolCallID, "inputTextDelta": d.ArgsText})

	case chat_session.EventToolCallEnd:
		var d chat_session.ToolCallEndData
		_ = json.Unmarshal(ev.Data, &d)
		argsText := ""
		if b, ok := u.args[d.ToolCallID]; ok {
			argsText = b.String()
		}
		u.chunk(gin.H{"type": "tool-input-available", "toolCallId": d.ToolCallID, "input": jsonObjectOrWrap(argsText)})

	case chat_session.EventToolResult:
		var d chat_session.ToolResultData
		_ = json.Unmarshal(ev.Data, &d)
		if d.IsError {
			u.chunk(gin.H{"type": "tool-output-error", "toolCallId": d.ToolCallID, "errorText": d.Result})
		} else {
			u.chunk(gin.H{"type": "tool-output-available", "toolCallId": d.ToolCallID, "output": jsonValueOrString(d.Result)})
		}

	case chat_session.EventStatus, chat_session.EventContext, chat_session.EventError:
		u.chunk(gin.H{"type": ev.Kind, "data": ev.Data})

	case chat_session.EventFinish:
		var d chat_session.FinishData
		_ = json.Unmarshal(ev.Data, &d)
		u.closeText()
		// Transient: reaches the client's onData hook without becoming a part.
		u.chunk(gin.H{"type": "data-message-ids", "transient": true, "data": gin.H{
			"user_message_id":      idString(d.UserMessageID),
			"assistant_message_id": idString(d.AssistantMessageID),
		}})
		u.finish(d.Reason)
		return true
	}
	return false
}
