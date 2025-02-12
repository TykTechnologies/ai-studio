package api_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// CustomResponseRecorder wraps httptest.ResponseRecorder and implements gin.ResponseWriter
type CustomResponseRecorder struct {
	*httptest.ResponseRecorder
	closeChannel chan bool
	size         int
	written      bool
	status       int
}

func (r *CustomResponseRecorder) CloseNotify() <-chan bool {
	return r.closeChannel
}

func (r *CustomResponseRecorder) Pusher() http.Pusher {
	return nil
}

func (r *CustomResponseRecorder) Size() int {
	return r.size
}

func (r *CustomResponseRecorder) Written() bool {
	return r.written
}

func (r *CustomResponseRecorder) WriteHeaderNow() {
	if !r.written {
		r.written = true
		r.ResponseRecorder.WriteHeader(r.status)
	}
}

func (r *CustomResponseRecorder) Status() int {
	return r.status
}

func (r *CustomResponseRecorder) WriteHeader(code int) {
	r.status = code
}

func (r *CustomResponseRecorder) Write(b []byte) (int, error) {
	r.WriteHeaderNow()
	n, err := r.ResponseRecorder.Write(b)
	r.size += n
	return n, err
}

func (r *CustomResponseRecorder) WriteString(s string) (int, error) {
	return r.Write([]byte(s))
}

func (r *CustomResponseRecorder) Flush() {
	r.ResponseRecorder.Flush()
}

func NewCustomResponseRecorder() *CustomResponseRecorder {
	return &CustomResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeChannel:     make(chan bool, 1),
		status:           http.StatusOK,
	}
}

func TestChatSSE(t *testing.T) {
	// Set up test environment
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("LOG_LEVEL", "info")
	os.Setenv("ENABLE_ANALYTICS", "false")
	os.Setenv("ENABLE_ECHO_CONVERSATION", "false")

	gin.SetMode(gin.TestMode)
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := auth.NewAuthService(config, apitest.NewMockMailer(), service)
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile)

	// Create test user
	user := &models.User{
		Email:         "test@test.com",
		Name:          "Test User",
		IsAdmin:       true,
		EmailVerified: true,
		ShowPortal:    true,
		ShowChat:      true,
	}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create default group
	defaultGroup := &models.Group{
		Name: "Default",
	}
	err = defaultGroup.Create(db)
	assert.NoError(t, err)

	// Add user to default group
	err = service.AddUserToGroup(user.ID, defaultGroup.ID)
	assert.NoError(t, err)

	// Create LLM settings
	llmSettings, err := service.CreateLLMSettings(&models.LLMSettings{
		ModelName:   "claude-3-sonnet-20240229",
		MaxTokens:   4000,
		Temperature: 0.7,
	})
	assert.NoError(t, err)

	// Create LLM
	llm, err := service.CreateLLM(
		"Default Anthropic", "api-key", "https://api.anthropic.com", 75,
		"Short desc", "Long desc", "http://logo.test",
		"anthropic", true, nil, "", []string{})
	assert.NoError(t, err)

	// Create test chat
	chat := &models.Chat{
		Name:          "Test Chat",
		Groups:        []models.Group{*defaultGroup},
		SupportsTools: true,
		SystemPrompt:  "You are a helpful assistant.",
		LLMSettingsID: llmSettings.ID,
		LLMID:         llm.ID,
	}
	err = chat.Create(db)
	assert.NoError(t, err)

	// Set up router with auth middleware
	router := gin.New()
	authed := router.Group("/common")
	authed.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})
	a.SetupChatRoutes(authed)

	t.Run("SSE Connection", func(t *testing.T) {
		log.Println("Starting SSE Connection test")
		w := NewCustomResponseRecorder()
		req := httptest.NewRequest("GET", fmt.Sprintf("/common/chat/%d", chat.ID), nil)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Connection", "keep-alive")

		// Create channels for synchronization
		done := make(chan bool)
		sessionReady := make(chan string)

		// Start serving the request in a goroutine
		go func() {
			log.Println("Starting to serve SSE request")
			router.ServeHTTP(w, req)
			log.Println("Finished serving SSE request")
			done <- true
		}()

		// Parse response in a goroutine
		go func() {
			for {
				// Read the response body
				body := w.Body.String()
				if body == "" {
					time.Sleep(100 * time.Millisecond)
					continue
				}

				log.Printf("Response body: %s", body)

				// Parse the response line by line
				scanner := bufio.NewScanner(strings.NewReader(body))
				for scanner.Scan() {
					line := scanner.Text()
					log.Printf("Scanning line: %s", line)
					if strings.HasPrefix(line, "event: session_id") {
						if scanner.Scan() {
							data := scanner.Text()
							if strings.HasPrefix(data, "data: ") {
								var msg api.ChatMessage
								err := json.Unmarshal([]byte(data[6:]), &msg)
								if err == nil {
									sessionReady <- msg.Payload
									return
								}
							}
						}
					}
				}
			}
		}()

		// Wait for session to be ready or timeout
		var sessionID string
		select {
		case sessionID = <-sessionReady:
			log.Printf("Session ready: %s", sessionID)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for session")
		}

		// Create chat history record
		chatHistory := &models.ChatHistoryRecord{
			SessionID: sessionID,
			ChatID:    chat.ID,
			UserID:    user.ID,
			Name:      "Test Session",
		}
		err = db.Create(chatHistory).Error
		assert.NoError(t, err)

		// Wait for session to be fully initialized
		time.Sleep(500 * time.Millisecond)

		// Test sending a message
		messageInput := map[string]interface{}{
			"type":      "user_message",
			"payload":   "Hello, assistant!",
			"file_refs": []string{},
		}

		w2 := apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat/%d/messages?session_id=%s", chat.ID, sessionID), messageInput)
		log.Printf("Message response code: %d", w2.Code)
		assert.Equal(t, http.StatusOK, w2.Code)

		// Close the SSE connection
		w.closeChannel <- true

		// Wait for completion
		<-done
		log.Println("SSE request completed")
	})

	t.Run("SSE Error Cases", func(t *testing.T) {
		log.Println("Starting SSE Error Cases test")
		// Test invalid chat ID
		w := apitest.PerformRequest(router, "GET", "/common/chat/999999", nil)
		log.Printf("Invalid chat ID response code: %d", w.Code)
		assert.Equal(t, http.StatusNotFound, w.Code)

		// Test missing session ID when sending message
		messageInput := map[string]interface{}{
			"type":      "user_message",
			"payload":   "Hello, assistant!",
			"file_refs": []string{},
		}

		w2 := apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat/%d/messages", chat.ID), messageInput)
		log.Printf("Missing session ID response code: %d", w2.Code)
		assert.Equal(t, http.StatusBadRequest, w2.Code)

		// Test invalid message type with non-existent session
		messageInput["type"] = "invalid_type"
		w3 := apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat/%d/messages?session_id=test", chat.ID), messageInput)
		log.Printf("Invalid message type response code: %d", w3.Code)
		assert.Equal(t, http.StatusNotFound, w3.Code)
	})

	// Clean up test environment
	os.Unsetenv("ENVIRONMENT")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("ENABLE_ANALYTICS")
	os.Unsetenv("ENABLE_ECHO_CONVERSATION")
}
