package api_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestChatSSE(t *testing.T) {
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

	t.Run("SSE Connection", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/sse/chat/%d", chat.ID), nil)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Connection", "keep-alive")

		// Create a channel to signal when we're done reading events
		done := make(chan bool)

		go func() {
			a.Router().ServeHTTP(w, req)
			done <- true
		}()

		// Wait for a short time to allow connection to be established
		time.Sleep(100 * time.Millisecond)

		// Check response headers
		assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
		assert.Equal(t, "keep-alive", w.Header().Get("Connection"))

		// Read the response body line by line
		scanner := bufio.NewScanner(w.Body)
		var sessionID string
		var foundSessionEvent bool

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: session") {
				foundSessionEvent = true
				// Next line should be the data
				if scanner.Scan() {
					data := scanner.Text()
					if strings.HasPrefix(data, "data: ") {
						var msg api.ChatMessage
						err := json.Unmarshal([]byte(data[6:]), &msg)
						assert.NoError(t, err)
						sessionID = msg.Payload
						assert.NotEmpty(t, sessionID)
						break
					}
				}
			}
		}

		assert.True(t, foundSessionEvent, "Should receive session event")
		assert.NotEmpty(t, sessionID, "Should receive session ID")

		// Test sending a message
		messageInput := api.ChatMessage{
			Type:    "user_message",
			Payload: "Hello, assistant!",
		}

		w2 := apitest.PerformRequest(a.Router(), "POST", fmt.Sprintf("/api/v1/sse/chat/%d/messages?session_id=%s", chat.ID, sessionID), messageInput)
		assert.Equal(t, http.StatusOK, w2.Code)

		// Wait a bit for the message to be processed
		time.Sleep(100 * time.Millisecond)

		// Close the connection
		done <- true
	})

	t.Run("SSE Error Cases", func(t *testing.T) {
		// Test invalid chat ID
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/sse/chat/999999", nil)
		req.Header.Set("Accept", "text/event-stream")
		a.Router().ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)

		// Test missing session ID when sending message
		messageInput := api.ChatMessage{
			Type:    "user_message",
			Payload: "Hello, assistant!",
		}
		w = apitest.PerformRequest(a.Router(), "POST", fmt.Sprintf("/api/v1/sse/chat/%d/messages", chat.ID), messageInput)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Test invalid message type
		messageInput.Type = "invalid_type"
		w = apitest.PerformRequest(a.Router(), "POST", fmt.Sprintf("/api/v1/sse/chat/%d/messages?session_id=test", chat.ID), messageInput)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
