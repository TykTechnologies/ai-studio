package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/chat_session"
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
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type ChatHub struct {
	sessions map[string]*websocket.Conn
	mutex    sync.Mutex
}

func NewChatHub() *ChatHub {
	return &ChatHub{
		sessions: make(map[string]*websocket.Conn),
	}
}

func (h *ChatHub) AddSession(sessionID string, conn *websocket.Conn) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.sessions[sessionID] = conn
}

func (h *ChatHub) RemoveSession(sessionID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	delete(h.sessions, sessionID)
}

func (a *API) HandleChatWebSocket(c *gin.Context) {
	// ws://your-server/ws/chat?session_id=<previously_received_session_id>

	userID := c.GetInt("user_id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}
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
		chatSession, err = a.createNewSession(uint(userID))
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
	sendWSMessage(conn, "session_id", chatSession.ID())

	// Add this connection to the hub
	hub := NewChatHub()
	hub.AddSession(chatSession.ID(), conn)
	defer hub.RemoveSession(chatSession.ID())

	// Handle incoming messages
	go handleIncomingMessages(conn, chatSession)

	// Handle outgoing messages
	handleOutgoingMessages(conn, chatSession)
}

func (a *API) loadExistingSession(sessionID string, userID uint) (*chat_session.ChatSession, error) {
	history := chat_session.NewGormChatMessageHistory(a.service.DB, sessionID, nil, &userID, "") // ignore system prompt as we are pulling from DB

	chat, err := history.GetAssociatedChat(context.Background())
	if err != nil {
		return nil, err
	}

	chatSession, err := chat_session.NewChatSession(
		chat,
		chat_session.ChatStream,
		a.service.DB,
		a.service,
		nil, // Add filters if needed
		&userID,
		&sessionID,
	)
	if err != nil {
		return nil, err
	}

	return chatSession, nil
}

func (a *API) createNewSession(userID uint) (*chat_session.ChatSession, error) {
	// Create a new Chat model
	chat := &models.Chat{
		// Set default values for the chat
		// You might want to set the LLM, LLMSettings, etc. here
	}
	err := chat.Create(a.service.DB)
	if err != nil {
		return nil, err
	}

	chatSession, err := chat_session.NewChatSession(
		chat,
		chat_session.ChatStream,
		a.service.DB,
		a.service,
		nil, // Add filters if needed
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
			cs.Input() <- &models.UserMessage{Payload: chatMessage.Payload}
		}
	}
}

func handleOutgoingMessages(conn *websocket.Conn, cs *chat_session.ChatSession) {
	for {
		select {
		case msg := <-cs.OutputMessage():
			sendWSMessage(conn, "ai_message", msg.Payload)
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
func (a *API) SetupWebSocketRoute() {
	a.router.GET("/ws/chat", func(c *gin.Context) {
		a.HandleChatWebSocket(c)
	})
}
