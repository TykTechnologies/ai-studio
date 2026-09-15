package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/filereader"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/tmc/langchaingo/llms"
)

const (
	// SSE event types
	eventSession     = "session_id"
	eventMessage     = "message"
	eventStreamChunk = "stream_chunk"
	eventError       = "error"
	eventSystem      = "system"
)

type ChatMessage struct {
	Type        string               `json:"type"`
	Payload     string               `json:"payload"`
	FileRefs    []string             `json:"file_refs"`
	Tools       []models.Tool        `json:"tools,omitempty"`
	Datasources []*models.Datasource `json:"datasources,omitempty"`
}

var (
	chatHub     *SessionHub
	chatHubOnce sync.Once
)

// getChatHub returns the process-wide hub of live chat and agent sessions.
func getChatHub() *SessionHub {
	chatHubOnce.Do(func() {
		chatHub = NewSessionHub(sessionIdleTTL())
		chatHub.StartReaper(30 * time.Second)
	})
	return chatHub
}

// jsonError writes the JSON:API-style error envelope used across the chat handlers.
func jsonError(c *gin.Context, status int, title, detail string) {
	c.JSON(status, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: title, Detail: detail}},
	})
}

// acquireChatSession returns the live chat session for sessionID, loading it
// from the database and starting it when it is not in the hub. On failure it
// has already written the error response and returns ok=false. The caller
// must call release when done with the session.
func (a *API) acquireChatSession(c *gin.Context, sessionID string, mode chat_session.OutputMode) (session *chat_session.ChatSession, release func(), ok bool) {
	uObj, found := c.Get("user")
	if !found {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "User not found")
		return nil, nil, false
	}
	userID := uint(uObj.(*models.User).ID)

	hs, rel, err := getChatHub().Acquire(sessionID, func() (HubSession, error) {
		loaded, err := a.loadExistingSession(sessionID, userID)
		if err != nil {
			return nil, err
		}
		loaded.SetOutputMode(mode)
		if err := loaded.Start(); err != nil {
			loaded.Stop()
			return nil, fmt.Errorf("start: %w", err)
		}
		slog.Info("Loaded and started chat session from database", "session_id", sessionID)
		return loaded, nil
	})
	if err != nil {
		slog.Error("Failed to load chat session", "session_id", sessionID, "error", err)
		if strings.HasPrefix(err.Error(), "start:") {
			jsonError(c, http.StatusInternalServerError, "Session error", "Failed to start chat session")
		} else {
			jsonError(c, http.StatusNotFound, "Session not found", "Chat session does not exist and could not be loaded")
		}
		return nil, nil, false
	}

	cs, isChat := hs.(*chat_session.ChatSession)
	if !isChat {
		rel()
		jsonError(c, http.StatusConflict, "Session mismatch", "Session is not a chat session")
		return nil, nil, false
	}
	if cs.OutputMode() != mode {
		rel()
		jsonError(c, http.StatusConflict, "Session mismatch", "Session is attached to a different chat API version")
		return nil, nil, false
	}
	return cs, rel, true
}

// HandleChatSSE handles Server-Sent Events for chat sessions
func (a *API) HandleChatSSE(c *gin.Context) {
	uObj, ok := c.Get("user")
	if !ok {
		jsonError(c, http.StatusUnauthorized, "Unauthorized", "User not found")
		return
	}
	thisUser := uObj.(*models.User)
	userID := int(thisUser.ID)

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

	// Set headers for SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-transform")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")  // Disable buffering in Nginx
	c.Writer.Header().Set("Content-Encoding", "none") // Prevent compression

	// Create a context with cancellation for this request
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// Client disconnection
	clientGone := c.Request.Context().Done()

	sessionID := c.Query("session_id")
	hub := getChatHub()

	var (
		chatSession *chat_session.ChatSession
		release     func()
	)
	if sessionID != "" {
		hs, rel, err := hub.Acquire(sessionID, func() (HubSession, error) {
			loaded, err := a.loadExistingSession(sessionID, uint(userID))
			if err != nil {
				return nil, err
			}
			if err := loaded.Start(); err != nil {
				loaded.Stop()
				return nil, err
			}
			return loaded, nil
		})
		if err != nil {
			log.Println("Error loading existing session:", err)
			sendSSEMessage(c.Writer, eventError, "Failed to load existing session")
			return
		}
		cs, isChat := hs.(*chat_session.ChatSession)
		if !isChat || cs.OutputMode() != chat_session.OutputModeRaw {
			rel()
			sendSSEMessage(c.Writer, eventError, "Session is attached to a different chat API version")
			return
		}
		chatSession, release = cs, rel
	} else {
		created, err := a.createNewSession(chat, uint(userID))
		if err != nil {
			sendSSEMessage(c.Writer, eventError, "Failed to create new session")
			return
		}
		if err := created.Start(); err != nil {
			created.Stop()
			sendSSEMessage(c.Writer, eventError, "Failed to start chat session")
			return
		}
		hs, rel := hub.Add(created)
		chatSession, release = hs.(*chat_session.ChatSession), rel
	}
	// The hub keeps the session alive while this connection holds it and for
	// the idle TTL afterwards, so a reconnect continues the same session.
	defer release()

	// Send session ID with current tools and datasources
	tools := make([]models.Tool, 0)
	for _, tool := range chatSession.CurrentTools() {
		tools = append(tools, tool)
	}
	msg := ChatMessage{
		Type:        eventSession,
		Payload:     chatSession.ID(),
		Tools:       tools,
		Datasources: chatSession.GetCurrentDatasources(),
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Println("Error marshaling session message:", err)
		return
	}
	sendSSEMessage(c.Writer, eventSession, string(msgBytes))

	// Use a WaitGroup to ensure keep-alive goroutine exits before handler returns
	var wg sync.WaitGroup
	wg.Add(1)

	// Start keep-alive goroutine
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in keep-alive goroutine: %v", r)
			}
		}()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Safely send ping message with additional checks
				if c.Writer != nil {
					sendSSEMessage(c.Writer, "ping", "")
					// Safely flush with error handling
					if flusher, ok := c.Writer.(http.Flusher); ok {
						flusher.Flush()
					}
				}
			case <-clientGone:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	handleSSEOutgoingMessages(c.Writer, chatSession, clientGone)

	// Ensure keep-alive goroutine has exited before handler returns
	// to prevent race condition with server closing the connection
	cancel()
	wg.Wait()
}

func sendSSEMessage(w http.ResponseWriter, event, data string) {
	// Encode newlines in data to ensure proper SSE format
	encodedData := strings.ReplaceAll(data, "\n", "\\n")
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encodedData)

	// Add a safe flush with error handling
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func handleSSEOutgoingMessages(w http.ResponseWriter, cs *chat_session.ChatSession, done <-chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in SSE message handler: %v", r)
		}
	}()

	var currentMessage strings.Builder
	var isStreaming bool
	for {
		select {
		case <-done:
			return
		case chunk, ok := <-cs.OutputStream():
			if !ok {
				return
			}
			// Try to parse as JSON to check if it's a combined message
			var mc llms.MessageContent
			err := json.Unmarshal(chunk, &mc)
			if err == nil && mc.Role == llms.ChatMessageTypeAI {
				// If it's a valid AI message, send as a regular message
				// and mark that we're not streaming
				sendSSEMessage(w, eventMessage, string(chunk))
				currentMessage.Reset()
				isStreaming = false
			} else if err != nil {
				// Only send stream chunks for non-JSON content (actual streaming text)
				sendSSEMessage(w, eventStreamChunk, string(chunk))
				isStreaming = true
			}

		case err, ok := <-cs.Errors():
			if !ok {
				return
			}
			// Errors
			sendSSEMessage(w, eventError, err.Error())
			currentMessage.Reset()
			isStreaming = false

		case msg, ok := <-cs.OutputMessage():
			if !ok {
				return
			}
			// Only send system messages or messages when we're not in streaming mode
			if strings.Contains(msg.Payload, ":::system") {
				// If already wrapped in :::system::: tags, send as is
				sendSSEMessage(w, eventSystem, msg.Payload)
			} else if strings.Contains(msg.Payload, "Tool") || strings.Contains(msg.Payload, "Datasource") {
				// If it's a tool/datasource message, wrap it in :::system::: tags
				sendSSEMessage(w, eventSystem, fmt.Sprintf(":::system %s:::", msg.Payload))
			} else if !isStreaming {
				// Only send as message if we're not currently streaming
				sendSSEMessage(w, eventMessage, msg.Payload)
			}
			currentMessage.Reset()
		}
	}
}

func (a *API) loadExistingSession(sessionID string, userID uint) (*chat_session.ChatSession, error) {
	history := chat_session.NewGormChatMessageHistory(a.service.DB, sessionID, 0, userID, "")
	chatFromHistory, err := history.GetAssociatedChat(context.Background())
	if err != nil {
		return nil, err
	}

	// Reload via service layer to resolve secret references ($SECRET/NAME)
	// GetAssociatedChat uses the model layer directly which bypasses secret interpolation
	chat, err := a.service.GetChatByID(chatFromHistory.ID)
	if err != nil {
		return nil, err
	}

	chatSession, err := chat_session.NewChatSession(
		chat,
		chat_session.ChatStream,
		a.service.DB,
		a.service,
		chat.Filters,
		&userID,
		&sessionID,
	)
	if err != nil {
		return nil, err
	}
	return chatSession, nil
}

func (a *API) createNewSession(chat *models.Chat, userID uint) (*chat_session.ChatSession, error) {
	chatSession, err := chat_session.NewChatSession(
		chat,
		chat_session.ChatStream,
		a.service.DB,
		a.service,
		chat.Filters,
		&userID,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return chatSession, nil
}

func (a *API) SetupChatRoutes(r *gin.RouterGroup) {
	r.GET("/chat/:chat_id", a.HandleChatSSE)
	r.POST("/chat/:chat_id/messages", a.handleSSEUserMessage)
	r.POST("/chat-sessions/:session_id/datasources", a.addDatasourceToChatSession)
	r.DELETE("/chat-sessions/:session_id/datasources/:datasource_id", a.removeDatasourceFromChatSession)
	r.POST("/chat-sessions/:session_id/tools", a.addToolToChatSession)
	r.DELETE("/chat-sessions/:session_id/tools/:tool_id", a.removeToolFromChatSession)
	r.POST("/chat-sessions/:session_id/upload", a.UploadFileToSession)
	r.PUT("/chat-sessions/:session_id/messages/:message_id", a.editMessageInChatSession)
	a.SetupChatV2Routes(r)
}

// handleSSEUserMessage handles user messages sent via POST for SSE connections
func (a *API) handleSSEUserMessage(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		jsonError(c, http.StatusBadRequest, "Missing session ID", "Session ID is required")
		return
	}

	var chatMessage ChatMessage
	if err := c.ShouldBindJSON(&chatMessage); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid message", err.Error())
		return
	}

	session, release, ok := a.acquireChatSession(c, sessionID, chat_session.OutputModeRaw)
	if !ok {
		return
	}
	defer release()

	if chatMessage.Type != "user_message" {
		jsonError(c, http.StatusBadRequest, "Invalid message type", "Only user_message type is supported")
		return
	}

	select {
	case session.Input() <- &models.UserMessage{Payload: chatMessage.Payload, FileRef: chatMessage.FileRefs}:
		c.JSON(http.StatusOK, gin.H{"message": "Message sent successfully"})
	case <-c.Request.Context().Done():
		jsonError(c, http.StatusRequestTimeout, "Cancelled", "Request cancelled before the message was queued")
	}
}

// sessionMutationHandler wraps the shared "acquire session, mutate, respond"
// shape of the tool/datasource endpoints. mutate returns the success message.
func (a *API) withChatSession(c *gin.Context, sessionID string, mutate func(*chat_session.ChatSession) (string, error), failTitle string) {
	session, release, ok := a.acquireChatSession(c, sessionID, chat_session.OutputModeRaw)
	if !ok {
		return
	}
	defer release()

	msg, err := mutate(session)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, failTitle, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

func (a *API) addDatasourceToChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	var input struct {
		DatasourceID uint `json:"datasource_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid input", err.Error())
		return
	}

	a.withChatSession(c, sessionID, func(session *chat_session.ChatSession) (string, error) {
		if err := session.AddDatasource(input.DatasourceID); err != nil {
			return "", err
		}
		return "Datasource added successfully", nil
	}, "Error adding datasource")
}

func (a *API) removeDatasourceFromChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	datasourceID, err := strconv.ParseUint(c.Param("datasource_id"), 10, 32)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid datasource ID", "Datasource ID must be a valid number")
		return
	}

	a.withChatSession(c, sessionID, func(session *chat_session.ChatSession) (string, error) {
		session.RemoveDatasource(uint(datasourceID))
		return "Datasource removed successfully", nil
	}, "Error removing datasource")
}

func (a *API) addToolToChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	var input struct {
		ToolID string `json:"tool_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid input", err.Error())
		return
	}

	toolId, err := strconv.Atoi(input.ToolID)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid tool ID", "Tool ID must be a valid number")
		return
	}

	tool, err := a.service.GetToolByID(uint(toolId))
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Error retrieving tool", err.Error())
		return
	}
	tool.OASSpec, err = helpers.DecodeToUTF8(tool.OASSpec)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Error decoding OAS spec", err.Error())
		return
	}

	a.withChatSession(c, sessionID, func(session *chat_session.ChatSession) (string, error) {
		if err := session.AddTool(input.ToolID, *tool); err != nil {
			return "", err
		}
		return "Tool added successfully", nil
	}, "Error adding tool")
}

func byteArrayToUTF8StringBuffer(data []byte) string {
	buf := bytes.Buffer{}
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		buf.WriteRune(r)
		data = data[size:]
	}
	return buf.String()
}

func (a *API) removeToolFromChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	toolID := c.Param("tool_id")

	a.withChatSession(c, sessionID, func(session *chat_session.ChatSession) (string, error) {
		session.RemoveTool(toolID)
		return "Tool removed successfully", nil
	}, "Error removing tool")
}

func (a *API) UploadFileToSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	session, release, ok := a.acquireChatSession(c, sessionID, chat_session.OutputModeRaw)
	if !ok {
		return
	}
	defer release()

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid file", err.Error())
		return
	}
	defer file.Close()

	raw, err := readFileContents(file)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Error reading file", err.Error())
		return
	}

	contents, err := filereader.Read(header.Filename, raw)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Error parsing file", err.Error())
		return
	}

	session.AddFileReference(header.Filename, contents)

	c.JSON(http.StatusOK, gin.H{"message": "File uploaded and added to the chat session successfully"})
}

func readFileContents(file multipart.File) ([]byte, error) {
	contents, err := io.ReadAll(file)
	if err != nil {
		return []byte{}, fmt.Errorf("error reading file: %v", err)
	}
	return contents, nil
}

// editMessageInChatSession updates a user message in a session, then removes subsequent messages
func (a *API) editMessageInChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	msgIDStr := c.Param("message_id")

	var req struct {
		NewContent json.RawMessage `json:"new_content" binding:"required"`
		Index      *int            `json:"index,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	// If index is provided, use EditUserMessageByIndex
	if req.Index != nil {
		if err := a.service.EditUserMessageByIndex(sessionID, *req.Index); err != nil {
			jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Messages removed from index"})
		return
	}

	// Otherwise, handle normal message editing
	if strings.HasPrefix(msgIDStr, "temp_") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot edit a temporary (unsaved) message."})
		return
	}

	msgID, err := strconv.ParseUint(msgIDStr, 10, 64)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Bad Request", "Invalid message ID")
		return
	}

	if err := a.service.EditUserMessage(sessionID, uint(msgID), string(req.NewContent)); err != nil {
		jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Message updated"})
}
