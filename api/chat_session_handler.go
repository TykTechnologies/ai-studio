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

type ChatHub struct {
	sessions map[string]*chat_session.ChatSession
	mutex    sync.RWMutex
}

func NewChatHub() *ChatHub {
	return &ChatHub{
		sessions: make(map[string]*chat_session.ChatSession),
	}
}

func (h *ChatHub) AddSession(sessionID string, session *chat_session.ChatSession) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.sessions[sessionID] = session
}

func (h *ChatHub) RemoveSession(sessionID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	delete(h.sessions, sessionID)
}

func (h *ChatHub) GetSession(sessionID string) (*chat_session.ChatSession, bool) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	session, exists := h.sessions[sessionID]
	return session, exists
}

func (h *ChatHub) UpdateSession(sessionID string, updateFunc func(*chat_session.ChatSession) error) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	session, exists := h.sessions[sessionID]
	if !exists {
		// Instead of returning an error, we'll try to load or create a new session
		slog.Info("Session not found in memory cache, attempting to load or create", "session_id", sessionID)
		
		// We need to create a new session with the given ID
		// Since we don't have direct access to the API instance here,
		// we'll return a special error that can be handled by the caller
		return fmt.Errorf("session_not_in_cache:%s", sessionID)
	}
	return updateFunc(session)
}

var (
	chatHub *ChatHub
	once    sync.Once
)

func getChatHub() *ChatHub {
	once.Do(func() {
		chatHub = NewChatHub()
	})
	return chatHub
}

// HandleChatSSE handles Server-Sent Events for chat sessions
func (a *API) HandleChatSSE(c *gin.Context) {
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
	userID := int(thisUser.ID)

	chatID, err := strconv.ParseUint(c.Param("chat_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid chat ID", Detail: "Chat ID must be a valid number"}},
		})
		return
	}

	chat := &models.Chat{}
	err = chat.Get(a.service.DB, uint(chatID))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Chat not found", Detail: "No chat found with the provided ID"}},
		})
		return
	}

	// Set headers for SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-transform")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")  // Disable buffering in Nginx
	c.Writer.Header().Set("Content-Encoding", "none") // Prevent compression

	// Create a channel for client disconnection
	clientGone := c.Writer.CloseNotify()

	sessionID := c.Query("session_id")

	var chatSession *chat_session.ChatSession
	if sessionID != "" {
		chatSession, err = a.loadExistingSession(sessionID, uint(userID))
		if err != nil {
			log.Println("Error loading existing session:", err)
			sendSSEMessage(c.Writer, eventError, "Failed to load existing session")
			return
		}
	}

	if chatSession == nil {
		chatSession, err = a.createNewSession(chat, uint(userID))
		if err != nil {
			sendSSEMessage(c.Writer, eventError, "Failed to create new session")
			return
		}
	}

	err = chatSession.Start()
	if err != nil {
		sendSSEMessage(c.Writer, eventError, "Failed to start chat session")
		return
	}
	defer chatSession.Stop()

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

	hub := getChatHub()
	hub.AddSession(chatSession.ID(), chatSession)
	defer hub.RemoveSession(chatSession.ID())

	// Start keep-alive goroutine
	go func() {
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
				sendSSEMessage(c.Writer, "ping", "")
				c.Writer.Flush()
			case <-clientGone:
				return
			}
		}
	}()

	handleSSEOutgoingMessages(c.Writer, chatSession, clientGone)
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

func handleSSEOutgoingMessages(w http.ResponseWriter, cs *chat_session.ChatSession, done <-chan bool) {
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
		case chunk := <-cs.OutputStream():
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

		case err := <-cs.Errors():
			// Errors
			sendSSEMessage(w, eventError, err.Error())
			currentMessage.Reset()
			isStreaming = false

		case msg := <-cs.OutputMessage():
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
	chat, err := history.GetAssociatedChat(context.Background())
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
}

// handleSSEUserMessage handles user messages sent via POST for SSE connections
func (a *API) handleSSEUserMessage(c *gin.Context) {
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

	var chatMessage ChatMessage
	if err := c.ShouldBindJSON(&chatMessage); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid message", Detail: err.Error()}},
		})
		return
	}

	hub := getChatHub()
	session, exists := hub.GetSession(sessionID)
	if !exists {
		// Try to load the existing session from the database
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
		userID := uint(thisUser.ID)
		
		// Try to load the session from the database
		loadedSession, err := a.loadExistingSession(sessionID, userID)
		if err != nil {
			slog.Error("Failed to load session from database", "session_id", sessionID, "error", err)
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Session not found", Detail: "Chat session does not exist and could not be loaded"}},
			})
			return
		}
		
		// Start the session and add it to the hub
		err = loadedSession.Start()
		if err != nil {
			slog.Error("Failed to start loaded session", "session_id", sessionID, "error", err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Session error", Detail: "Failed to start chat session"}},
			})
			return
		}
		
		hub.AddSession(sessionID, loadedSession)
		session = loadedSession
		slog.Info("Successfully loaded and started session from database", "session_id", sessionID)
	}

	if chatMessage.Type == "user_message" {
		session.Input() <- &models.UserMessage{Payload: chatMessage.Payload, FileRef: chatMessage.FileRefs}
		c.JSON(http.StatusOK, gin.H{"message": "Message sent successfully"})
	} else {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid message type", Detail: "Only user_message type is supported"}},
		})
	}
}

func (a *API) addDatasourceToChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	var input struct {
		DatasourceID uint `json:"datasource_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid input", Detail: err.Error()}},
		})
		return
	}

	hub := getChatHub()
	err := hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		errInner := session.AddDatasource(input.DatasourceID)
		if errInner != nil {
			return errInner
		}
		return nil
	})
	
	if err != nil {
		// Check if this is our special error indicating session not in cache
		if strings.HasPrefix(err.Error(), "session_not_in_cache:") {
			// Try to load the existing session from the database
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
			userID := uint(thisUser.ID)
			
			// Try to load the session from the database
			loadedSession, err := a.loadExistingSession(sessionID, userID)
			if err != nil {
				slog.Error("Failed to load session from database", "session_id", sessionID, "error", err)
				c.JSON(http.StatusNotFound, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session not found", Detail: "Chat session does not exist and could not be loaded"}},
				})
				return
			}
			
			// Start the session and add it to the hub
			err = loadedSession.Start()
			if err != nil {
				slog.Error("Failed to start loaded session", "session_id", sessionID, "error", err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session error", Detail: "Failed to start chat session"}},
				})
				return
			}
			
			hub.AddSession(sessionID, loadedSession)
			
			// Now try the update again
			err = hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
				errInner := session.AddDatasource(input.DatasourceID)
				if errInner != nil {
					return errInner
				}
				return nil
			})
			
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Error adding datasource", Detail: err.Error()}},
				})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Error adding datasource", Detail: err.Error()}},
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Datasource added successfully"})
}

func (a *API) removeDatasourceFromChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	datasourceID, err := strconv.ParseUint(c.Param("datasource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid datasource ID", Detail: "Datasource ID must be a valid number"}},
		})
		return
	}

	hub := getChatHub()
	err = hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		session.RemoveDatasource(uint(datasourceID))
		return nil
	})
	
	if err != nil {
		// Check if this is our special error indicating session not in cache
		if strings.HasPrefix(err.Error(), "session_not_in_cache:") {
			// Try to load the existing session from the database
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
			userID := uint(thisUser.ID)
			
			// Try to load the session from the database
			loadedSession, err := a.loadExistingSession(sessionID, userID)
			if err != nil {
				slog.Error("Failed to load session from database", "session_id", sessionID, "error", err)
				c.JSON(http.StatusNotFound, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session not found", Detail: "Chat session does not exist and could not be loaded"}},
				})
				return
			}
			
			// Start the session and add it to the hub
			err = loadedSession.Start()
			if err != nil {
				slog.Error("Failed to start loaded session", "session_id", sessionID, "error", err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session error", Detail: "Failed to start chat session"}},
				})
				return
			}
			
			hub.AddSession(sessionID, loadedSession)
			
			// Now try the update again
			err = hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
				session.RemoveDatasource(uint(datasourceID))
				return nil
			})
			
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Error removing datasource", Detail: err.Error()}},
				})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Error removing datasource", Detail: err.Error()}},
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Datasource removed successfully"})
}

func (a *API) addToolToChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	var input struct {
		ToolID string `json:"tool_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid input", Detail: err.Error()}},
		})
		return
	}

	toolId, err := strconv.Atoi(input.ToolID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid tool ID", Detail: "Tool ID must be a valid number"}},
		})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolId))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error retrieving tool", Detail: err.Error()}},
		})
		return
	}
	tool.OASSpec, err = helpers.DecodeToUTF8(tool.OASSpec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error decoding OAS spec", Detail: err.Error()}},
		})
		return
	}

	hub := getChatHub()
	err = hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		if e := session.AddTool(input.ToolID, *tool); e != nil {
			return e
		}
		return nil
	})
	
	if err != nil {
		// Check if this is our special error indicating session not in cache
		if strings.HasPrefix(err.Error(), "session_not_in_cache:") {
			// Try to load the existing session from the database
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
			userID := uint(thisUser.ID)
			
			// Try to load the session from the database
			loadedSession, err := a.loadExistingSession(sessionID, userID)
			if err != nil {
				slog.Error("Failed to load session from database", "session_id", sessionID, "error", err)
				c.JSON(http.StatusNotFound, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session not found", Detail: "Chat session does not exist and could not be loaded"}},
				})
				return
			}
			
			// Start the session and add it to the hub
			err = loadedSession.Start()
			if err != nil {
				slog.Error("Failed to start loaded session", "session_id", sessionID, "error", err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session error", Detail: "Failed to start chat session"}},
				})
				return
			}
			
			hub.AddSession(sessionID, loadedSession)
			
			// Now try the update again
			err = hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
				if e := session.AddTool(input.ToolID, *tool); e != nil {
					return e
				}
				return nil
			})
			
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Error adding tool", Detail: err.Error()}},
				})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Error adding tool", Detail: err.Error()}},
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tool added successfully"})
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

	hub := getChatHub()
	err := hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		session.RemoveTool(toolID)
		return nil
	})
	
	if err != nil {
		// Check if this is our special error indicating session not in cache
		if strings.HasPrefix(err.Error(), "session_not_in_cache:") {
			// Try to load the existing session from the database
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
			userID := uint(thisUser.ID)
			
			// Try to load the session from the database
			loadedSession, err := a.loadExistingSession(sessionID, userID)
			if err != nil {
				slog.Error("Failed to load session from database", "session_id", sessionID, "error", err)
				c.JSON(http.StatusNotFound, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session not found", Detail: "Chat session does not exist and could not be loaded"}},
				})
				return
			}
			
			// Start the session and add it to the hub
			err = loadedSession.Start()
			if err != nil {
				slog.Error("Failed to start loaded session", "session_id", sessionID, "error", err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Session error", Detail: "Failed to start chat session"}},
				})
				return
			}
			
			hub.AddSession(sessionID, loadedSession)
			
			// Now try the update again
			err = hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
				session.RemoveTool(toolID)
				return nil
			})
			
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{{Title: "Error removing tool", Detail: err.Error()}},
				})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Error removing tool", Detail: err.Error()}},
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tool removed successfully"})
}

func (a *API) UploadFileToSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	hub := getChatHub()
	session, exists := hub.GetSession(sessionID)
	if !exists {
		// Try to load the existing session from the database
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
		userID := uint(thisUser.ID)
		
		// Try to load the session from the database
		loadedSession, err := a.loadExistingSession(sessionID, userID)
		if err != nil {
			slog.Error("Failed to load session from database", "session_id", sessionID, "error", err)
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Session not found", Detail: "Chat session does not exist and could not be loaded"}},
			})
			return
		}
		
		// Start the session and add it to the hub
		err = loadedSession.Start()
		if err != nil {
			slog.Error("Failed to start loaded session", "session_id", sessionID, "error", err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Session error", Detail: "Failed to start chat session"}},
			})
			return
		}
		
		hub.AddSession(sessionID, loadedSession)
		session = loadedSession
		slog.Info("Successfully loaded and started session from database", "session_id", sessionID)
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Invalid file", Detail: err.Error()}},
		})
		return
	}
	defer file.Close()

	raw, err := readFileContents(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error reading file", Detail: err.Error()}},
		})
		return
	}

	contents, err := filereader.Read(header.Filename, raw)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error parsing file", Detail: err.Error()}},
		})
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
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	// If index is provided, use EditUserMessageByIndex
	if req.Index != nil {
		if err := a.service.EditUserMessageByIndex(sessionID, *req.Index); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
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
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid message ID"}},
		})
		return
	}

	if err := a.service.EditUserMessage(sessionID, uint(msgID), string(req.NewContent)); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Message updated"})
}
