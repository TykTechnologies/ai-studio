package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"unicode/utf8"

	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/filereader"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Add logic here to check the origin of the request
		return true // For now, we're allowing all origins
	},
}

type ChatMessage struct {
	Type     string   `json:"type"`
	Payload  string   `json:"payload"`
	FileRefs []string `json:"file_refs"`
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

func (a *API) HandleChatWebSocket(c *gin.Context) {
	// ws://your-server/ws/chat/:chat_id?session_id=<optional_previously_received_session_id>

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
	log.Println("WebSocket connection established")
	defer conn.Close()

	// Get session ID from query parameter
	sessionID := c.Query("session_id")

	var chatSession *chat_session.ChatSession

	if sessionID != "" {
		// Attempt to load existing session
		chatSession, err = a.loadExistingSession(sessionID, uint(userID))
		if err != nil {
			log.Println("Error loading existing session:", err)
			sendWSMessage(conn, "error", "Failed to load existing session")
			return
		}
	}

	if chatSession == nil {
		// Create a new session if no existing session was loaded
		chatSession, err = a.createNewSession(chat, uint(userID))
		if err != nil {
			log.Println("Error creating new session:", err)
			sendWSMessage(conn, "error", "Failed to create new session")
			return
		}
	}

	err = chatSession.Start()
	if err != nil {
		log.Println("Error starting chat session:", err)
		sendWSMessage(conn, "error", "Failed to start chat session")
		return
	}
	defer chatSession.Stop()

	// Send the session ID to the client
	fmt.Println("sending session ID")
	sendWSMessage(conn, "session_id", chatSession.ID())
	// Use the singleton ChatHub
	hub := getChatHub()
	fmt.Println("Adding session to chat hub")
	hub.AddSession(chatSession.ID(), chatSession)
	defer hub.RemoveSession(chatSession.ID())

	// Handle incoming messages
	fmt.Println("listening for inbound messages")
	go handleIncomingMessages(conn, chatSession)

	// Handle outgoing messages
	fmt.Println("Handling outgoing messages")
	handleOutgoingMessages(conn, chatSession)
	fmt.Println("CLOSED")
}

func (a *API) loadExistingSession(sessionID string, userID uint) (*chat_session.ChatSession, error) {
	history := chat_session.NewGormChatMessageHistory(a.service.DB, sessionID, 0, userID, "") // ignore system prompt as we are pulling from DB

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
		nil, // No session ID
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

func handleOutgoingMessages(conn *websocket.Conn, cs *chat_session.ChatSession) {
	for {
		select {
		// case msg := <-cs.OutputMessage():
		// 	sendWSMessage(conn, "ai_message", msg.Payload)
		case chunk := <-cs.OutputStream():
			sendWSMessage(conn, "stream_chunk", string(chunk))
		case err := <-cs.Errors():
			sendWSMessage(conn, "error", err.Error())
		}
	}
}

func sendWSMessage(conn *websocket.Conn, msgType string, payload string) {
	message := ChatMessage{
		Type:    msgType,
		Payload: payload,
	}
	err := conn.WriteJSON(message)
	if err != nil {
		log.Println("Error writing message:", err)
	}
}

// Add this method to your API struct to set up the WebSocket route
func (a *API) SetupWebSocketRoute(r *gin.RouterGroup) {
	r.GET("/ws/chat/:chat_id", func(c *gin.Context) {
		a.HandleChatWebSocket(c)
	})
}

func (a *API) AddDatasourceToChatSession(sessionID string, datasourceID uint) error {
	hub := getChatHub()
	return hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		return session.AddDatasource(datasourceID)
	})
}

func (a *API) RemoveDatasourceFromChatSession(sessionID string, datasourceID uint) error {
	hub := getChatHub()
	return hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		session.RemoveDatasource(datasourceID)
		return nil
	})
}

func (a *API) AddToolToChatSession(sessionID string, toolID string, tool models.Tool) error {
	hub := getChatHub()
	return hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		return session.AddTool(toolID, tool)
	})
}

func (a *API) RemoveToolFromChatSession(sessionID string, toolID string) error {
	hub := getChatHub()
	return hub.UpdateSession(sessionID, func(session *chat_session.ChatSession) error {
		session.RemoveTool(toolID)
		return nil
	})
}

// addDatasourceToChatSession handles adding a datasource to an active chat session
func (a *API) addDatasourceToChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	var input struct {
		DatasourceID uint `json:"datasource_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Invalid input", Detail: err.Error()}}})
		return
	}

	err := a.AddDatasourceToChatSession(sessionID, input.DatasourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error adding datasource", Detail: err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Datasource added successfully"})
}

// removeDatasourceFromChatSession handles removing a datasource from an active chat session
func (a *API) removeDatasourceFromChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	datasourceID, err := strconv.ParseUint(c.Param("datasource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Invalid datasource ID", Detail: "Datasource ID must be a valid number"}}})
		return
	}

	err = a.RemoveDatasourceFromChatSession(sessionID, uint(datasourceID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error removing datasource", Detail: err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Datasource removed successfully"})
}

// addToolToChatSession handles adding a tool to an active chat session
func (a *API) addToolToChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	var input struct {
		ToolID string      `json:"tool_id" binding:"required"`
		Tool   models.Tool `json:"tool" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Invalid input", Detail: err.Error()}}})
		return
	}

	toolId, err := strconv.Atoi(input.ToolID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Invalid tool ID", Detail: "Tool ID must be a valid number"}}})
		return
	}

	tool, err := a.service.GetToolByID(uint(toolId))
	tool.OASSpec, err = helpers.DecodeToUTF8(tool.OASSpec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error decoding OAS spec", Detail: err.Error()}}})
		return
	}

	err = a.AddToolToChatSession(sessionID, input.ToolID, *tool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error adding tool", Detail: err.Error()}}})
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

// removeToolFromChatSession handles removing a tool from an active chat session
func (a *API) removeToolFromChatSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	toolID := c.Param("tool_id")

	err := a.RemoveToolFromChatSession(sessionID, toolID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error removing tool", Detail: err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tool removed successfully"})
}

func (a *API) UploadFileToSession(c *gin.Context) {
	fmt.Println("File upload called")
	sessionID := c.Param("session_id")

	// Get the chat session
	hub := getChatHub()
	session, exists := hub.GetSession(sessionID)
	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Session not found", Detail: "Chat session does not exist"}}})
		return
	}

	// Get the file from the request
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Invalid file", Detail: err.Error()}}})
		return
	}
	defer file.Close()
	fmt.Println("File uploaded:", header.Filename)

	// Read the file contents
	raw, err := readFileContents(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error reading file", Detail: err.Error()}}})
		return
	}
	fmt.Println("File contents read")

	contents, err := filereader.Read(header.Filename, raw)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error parsing file", Detail: err.Error()}}})
		return
	}
	fmt.Println("filereader completed")

	// Add the file reference to the chat session
	session.AddFileReference(header.Filename, contents)
	fmt.Println("File reference added to chat session")

	c.JSON(http.StatusOK, gin.H{"message": "File uploaded and added to the chat session successfully"})
}

func readFileContents(file multipart.File) ([]byte, error) {
	contents, err := io.ReadAll(file)
	if err != nil {
		return []byte{}, fmt.Errorf("error reading file: %v", err)
	}
	return contents, nil
}
