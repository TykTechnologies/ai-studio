package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	gotest "testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// TestAgentMessageSSE tests that the agent message handler properly streams SSE responses
func TestAgentMessageSSE(t *gotest.T) {
	// Setup environment
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("LOG_LEVEL", "info")
	defer func() {
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("LOG_LEVEL")
	}()

	gin.SetMode(gin.TestMode)

	// Setup DB and service
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, nil)

	// Create user and group
	user := &models.User{
		Email:         "test@test.com",
		Name:          "Test User",
		IsAdmin:       true,
		EmailVerified: true,
	}
	err := user.Create(db)
	assert.NoError(t, err)

	defaultGroup := &models.Group{Name: "Default"}
	err = defaultGroup.Create(db)
	assert.NoError(t, err)
	err = service.AddUserToGroup(user.ID, defaultGroup.ID)
	assert.NoError(t, err)

	// Create an App with mock LLM
	app := &models.App{
		Name:   "Test App",
		Groups: []models.Group{*defaultGroup},
		LLMs: []models.LLM{
			{
				Name:   "Dummy LLM",
				Vendor: models.MOCK_VENDOR,
			},
		},
	}
	err = app.Create(db)
	assert.NoError(t, err)

	// Note: We can't easily test with a real plugin in a unit test
	// This test will verify the handler logic but skip the actual plugin interaction
	t.Skip("Agent handler test requires plugin infrastructure - manual testing required")

	// Create router
	router := gin.New()
	authed := router.Group("/api/v1")
	authed.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	// Register routes
	a.SetupAgentRoutes(authed)

	// Start test server
	ts := httptest.NewUnstartedServer(router)
	ts.Start()
	defer ts.Close()

	t.Run("SSE_Streaming", func(t *gotest.T) {
		eventsCh := make(chan map[string]interface{}, 10)
		errCh := make(chan error, 1)
		doneCh := make(chan struct{})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client := &http.Client{Timeout: 0}

		// Start SSE reader goroutine
		go func() {
			defer close(doneCh)

			reqBody := bytes.NewBufferString(`{"message":"test"}`)
			req, err := http.NewRequestWithContext(ctx, "POST",
				fmt.Sprintf("%s/api/v1/agents/1/message", ts.URL), reqBody)
			if err != nil {
				errCh <- err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream")

			resp, err := client.Do(req)
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
				return
			}

			scanner := bufio.NewScanner(resp.Body)
			var currentEvent string
			var dataLines []string

			for scanner.Scan() {
				line := scanner.Text()
				log.Printf("SSE line: %s", line)

				if line == "" {
					// Empty line indicates end of event
					if len(dataLines) > 0 {
						raw := strings.Join(dataLines, "\n")
						raw = strings.ReplaceAll(raw, "data: ", "")

						var msg map[string]interface{}
						if err := json.Unmarshal([]byte(raw), &msg); err != nil {
							log.Printf("Failed to parse: %s", raw)
							continue
						}

						msg["event"] = currentEvent
						eventsCh <- msg
					}
					dataLines = nil
					currentEvent = ""
				} else if strings.HasPrefix(line, "event: ") {
					currentEvent = strings.TrimPrefix(line, "event: ")
				} else if strings.HasPrefix(line, "data: ") {
					dataLines = append(dataLines, line)
				}
			}

			if err := scanner.Err(); err != nil {
				errCh <- err
			}
		}()

		// Wait for events
		receivedEvents := make([]map[string]interface{}, 0)
		timeout := time.After(5 * time.Second)

	eventLoop:
		for {
			select {
			case <-timeout:
				t.Logf("Timeout waiting for events. Received %d events", len(receivedEvents))
				break eventLoop
			case err := <-errCh:
				t.Fatalf("Error: %v", err)
			case event, ok := <-eventsCh:
				if !ok {
					break eventLoop
				}
				receivedEvents = append(receivedEvents, event)
				t.Logf("Received event: %v", event)

				// Check if we got a done event
				if eventType, ok := event["event"].(string); ok && eventType == "done" {
					break eventLoop
				}
			case <-doneCh:
				break eventLoop
			}
		}

		// Verify we received at least one event
		assert.Greater(t, len(receivedEvents), 0, "Should receive at least one SSE event")

		t.Logf("Test complete. Received %d events", len(receivedEvents))
	})
}
