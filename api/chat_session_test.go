package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// TestChatSSE tests SSE with multiline JSON events and ensures we don't get a 404
// after posting a user message.
func TestChatSSE(t *testing.T) {
	// Setup environment variables.
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("LOG_LEVEL", "info")
	os.Setenv("ENABLE_ANALYTICS", "false")
	os.Setenv("ENABLE_ECHO_CONVERSATION", "false")
	defer func() {
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("ENABLE_ANALYTICS")
		os.Unsetenv("ENABLE_ECHO_CONVERSATION")
	}()

	gin.SetMode(gin.TestMode)

	// Setup DB, service, config, auth, etc.
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := auth.NewAuthService(config, apitest.NewMockMailer(), service)
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile)

	// Create user.
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

	// Create default group & attach user.
	defaultGroup := &models.Group{Name: "Default"}
	err = defaultGroup.Create(db)
	assert.NoError(t, err)
	err = service.AddUserToGroup(user.ID, defaultGroup.ID)
	assert.NoError(t, err)

	// Create a Chat with a mock LLM.
	chat := &models.Chat{
		LLM: &models.LLM{
			Name:   "Dummy LLM",
			Vendor: models.MOCK_VENDOR,
		},
		LLMSettings: &models.LLMSettings{
			ModelName: "dummy",
		},
		Name:          "Test Chat",
		Groups:        []models.Group{*defaultGroup},
		SupportsTools: true,
		SystemPrompt:  "You are a helpful assistant.",
	}
	err = chat.Create(db)
	assert.NoError(t, err)

	// Create a router with an authenticated group.
	router := gin.New()
	authed := router.Group("/common")
	authed.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})
	a.SetupChatRoutes(authed)

	// Start a real in-memory test server.
	ts := httptest.NewUnstartedServer(router)
	// For SSE, you may want to avoid super-short timeouts.
	// ts.Config.ReadTimeout = 1 * time.Second
	// ts.Config.WriteTimeout = 1 * time.Second
	// ts.Config.IdleTimeout = 0
	ts.Start()

	t.Run("SSE_Connection", func(t *testing.T) {
		log.Println("Starting SSE Connection test")

		// Channels for coordination.
		sessionIDCh := make(chan string, 1) // receives session_id from SSE
		errCh := make(chan error, 1)
		doneCh := make(chan struct{}) // signals SSE goroutine that the stream_chunk was received
		// var wg sync.WaitGroup

		// Custom client with a slightly longer timeout for SSE.
		client := &http.Client{
			Timeout: 1 * time.Second,
		}

		// Start SSE-reading goroutine.
		// wg.Add(1)
		go func() {
			// defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

			req, err := http.NewRequestWithContext(
				ctx,
				"GET",
				fmt.Sprintf("%s/common/chat/%d", ts.URL, chat.ID),
				nil,
			)
			if err != nil {
				errCh <- fmt.Errorf("creating SSE request failed: %w", err)
				return
			}
			req.Header.Set("Accept", "text/event-stream")

			resp, err := client.Do(req)
			if err != nil {
				errCh <- fmt.Errorf("SSE request failed: %w", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("SSE status code %d (expected 200)", resp.StatusCode)
				return
			}

			scanner := bufio.NewScanner(resp.Body)
			var currentEvent string
			var dataLines []string

			for scanner.Scan() {
				line := scanner.Text()
				log.Printf("SSE line: %s", line)

				// A blank line indicates the end of one SSE event.
				if line == "" {
					switch currentEvent {
					case "session_id":
						raw := strings.Join(dataLines, "\n")
						raw = strings.ReplaceAll(raw, "data: ", "")
						log.Printf("Complete multiline JSON for session_id: %q", raw)
						var msg map[string]interface{}
						if err := json.Unmarshal([]byte(raw), &msg); err != nil {
							errCh <- fmt.Errorf("unmarshal error: %w", err)
							return
						}
						payload, _ := msg["payload"].(string)
						sessionIDCh <- payload

					case "stream_chunk":
						// We got the first stream chunk; signal completion.
						close(doneCh)
						resp.Body.Close()
						cancel()
						return
					}
					currentEvent = ""
					dataLines = nil
					continue
				}

				if strings.HasPrefix(line, "event: ") {
					currentEvent = strings.TrimPrefix(line, "event: ")
					dataLines = nil
				} else if strings.HasPrefix(line, "data: ") {
					dataLines = append(dataLines, line)
				}
			}
			if scErr := scanner.Err(); scErr != nil {
				errCh <- fmt.Errorf("scanner error: %w", scErr)
			}
		}()

		// Wait for the session_id SSE event or error.
		var sessionID string
		select {
		case sid := <-sessionIDCh:
			sessionID = sid
			log.Printf("Got session_id from SSE: %s", sessionID)
		case e := <-errCh:
			t.Fatalf("Error reading SSE stream: %v", e)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for session_id SSE event")
		}

		// Create ChatHistory in DB.
		chatHistory := &models.ChatHistoryRecord{
			SessionID: sessionID,
			ChatID:    chat.ID,
			UserID:    user.ID,
			Name:      "Test Session",
		}
		err = db.Create(chatHistory).Error
		assert.NoError(t, err)

		// POST a user message while SSE is still connected.
		messageInput := map[string]interface{}{
			"type":      "user_message",
			"payload":   "Hello, assistant!",
			"file_refs": []string{},
		}
		w2 := apitest.PerformRequest(
			router,
			"POST",
			fmt.Sprintf("/common/chat/%d/messages?session_id=%s", chat.ID, sessionID),
			messageInput,
		)
		log.Printf("Message response code: %d", w2.Code)
		assert.Equal(t, http.StatusOK, w2.Code)

		// Wait for the first stream_chunk SSE event or an error.
		select {
		case e := <-errCh:
			t.Fatalf("Error reading SSE stream: %v", e)
		case <-doneCh:
			log.Println("Got first stream chunk, test complete")
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for stream_chunk SSE event")
		}

		// If you really want to trigger a panic (for debugging), uncomment below.
		// panic("asdas")
	})
}
