package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	"github.com/gorilla/websocket"
	"github.com/tmc/langchaingo/llms"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Add logic here to check the origin of the request
		return true
	},
}

const (
	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10
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
		return fmt.Errorf("session not found")
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

// HandleChatWebSocket sets up the WebSocket for the given chat session.
func (a *API) HandleChatWebSocket(c *gin.Context) {
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

	log.Println("Attempting to upgrade connection to WebSocket")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	// Set up ping/pong handlers
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					log.Printf("Error sending ping: %v", err)
					return
				}
			}
		}
	}()

	log.Println("WebSocket connection established with ping/pong handlers")

	sessionID := c.Query("session_id")

	var chatSession *chat_session.ChatSession
	if sessionID != "" {
		chatSession, err = a.loadExistingSession(sessionID, uint(userID))
		if err != nil {
			log.Println("Error loading existing session:", err)
			sendWSMessage(conn, "error", "Failed to load existing session")
			return
		}
	}

	if chatSession == nil {
		chatSession, err = a.createNewSession(chat, uint(userID))
		if err != nil {
			sendWSMessage(conn, "error", "Failed to create new session")
			return
		}
	}

	err = chatSession.Start()
	if err != nil {
		sendWSMessage(conn, "error", "Failed to start chat session")
		return
	}
	defer chatSession.Stop()

	// Send session ID with current tools and datasources
	tools := make([]models.Tool, 0)
	for _, tool := range chatSession.CurrentTools() {
		tools = append(tools, tool)
	}
	msg := ChatMessage{
		Type:        "session_id",
		Payload:     chatSession.ID(),
		Tools:       tools,
		Datasources: chatSession.GetCurrentDatasources(),
	}
	if err := conn.WriteJSON(msg); err != nil {
		log.Println("Error writing session message:", err)
	}

	hub := getChatHub()
	hub.AddSession(chatSession.ID(), chatSession)
	defer hub.RemoveSession(chatSession.ID())

	go handleIncomingMessages(conn, chatSession)

	handleOutgoingMessages(conn, chatSession)
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

func handleIncomingMessages(conn *websocket.Conn, cs *chat_session.ChatSession) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			return
		}
		var chatMessage ChatMessage
		err = json.Unmarshal(message, &chatMessage)
		if err != nil {
			log.Println("Error unmarshalling message:", err)
			continue
		}
		if chatMessage.Type == "user_message" {
			cs.Input() <- &models.UserMessage{Payload: chatMessage.Payload, FileRef: chatMessage.FileRefs}
		}
	}
}

func sendWSMessage(conn *websocket.Conn, msgType string, payload string) {
	message := ChatMessage{
		Type:    msgType,
		Payload: payload,
	}
	if err := conn.WriteJSON(message); err != nil {
		log.Println("Error writing message:", err)
	}
}

func handleOutgoingMessages(conn *websocket.Conn, cs *chat_session.ChatSession) {
	var currentMessage strings.Builder
	var isStreaming bool
	for {
		select {
		case chunk := <-cs.OutputStream():
			// Try to parse as JSON to check if it's a combined message
			var mc llms.MessageContent
			err := json.Unmarshal(chunk, &mc)
			if err == nil && mc.Role == llms.ChatMessageTypeAI {
				// If it's a valid AI message, send as a regular message
				// and mark that we're not streaming
				sendWSMessage(conn, "message", string(chunk))
				currentMessage.Reset()
				isStreaming = false
			} else if err != nil {
				// Only send stream chunks for non-JSON content (actual streaming text)
				sendWSMessage(conn, "stream_chunk", string(chunk))
				isStreaming = true
			}

		case err := <-cs.Errors():
			// Errors
			sendWSMessage(conn, "error", err.Error())
			currentMessage.Reset()
			isStreaming = false

		case msg := <-cs.OutputMessage():
			// Only send system messages or messages when we're not in streaming mode
			if strings.Contains(msg.Payload, ":::system") {
				sendWSMessage(conn, "system", msg.Payload)
			} else if !isStreaming {
				// Only send as message if we're not currently streaming
				sendWSMessage(conn, "message", msg.Payload)
			}
			currentMessage.Reset()
		}
	}
}

func (a *API) SetupChatRoutes(r *gin.RouterGroup) {
	r.GET("/ws/chat/:chat_id", func(c *gin.Context) {
		a.HandleChatWebSocket(c)
	})
	r.POST("/chat-sessions/:session_id/datasources", a.addDatasourceToChatSession)
	r.DELETE("/chat-sessions/:session_id/datasources/:datasource_id", a.removeDatasourceFromChatSession)
	r.POST("/chat-sessions/:session_id/tools", a.addToolToChatSession)
	r.DELETE("/chat-sessions/:session_id/tools/:tool_id", a.removeToolFromChatSession)
	r.POST("/chat-sessions/:session_id/upload", a.UploadFileToSession)
	r.PUT("/chat-sessions/:session_id/messages/:message_id", a.editMessageInChatSession)
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
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error adding datasource", Detail: err.Error()}},
		})
		return
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
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error removing datasource", Detail: err.Error()}},
		})
		return
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
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error adding tool", Detail: err.Error()}},
		})
		return
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
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error removing tool", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tool removed successfully"})
}

func (a *API) UploadFileToSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	hub := getChatHub()
	session, exists := hub.GetSession(sessionID)
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Session not found", Detail: "Chat session does not exist"}},
		})
		return
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
