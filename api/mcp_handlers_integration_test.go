package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// Integration test that demonstrates how the MCP API flow would work
func TestMCPAPIIntegration(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Set up the router
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Add middleware to set user
	r.Use(func(c *gin.Context) {
		user := &models.User{
			Model: gorm.Model{
				ID: 1,
			},
			Email: "testuser@example.com",
			Name:  "testuser",
		}
		c.Set("user", user)
		c.Next()
	})

	// Register routes
	mcpGroup := r.Group("/mcp")
	{
		// Server management
		mcpGroup.POST("/servers", handlers.CreateMCPServer)
		mcpGroup.GET("/servers/:id", handlers.GetMCPServer)

		// Tool management
		mcpGroup.POST("/servers/:id/tools", handlers.AddToolToMCPServer)

		// Server control
		mcpGroup.POST("/servers/:id/start", handlers.StartMCPServer)
		mcpGroup.POST("/servers/:id/stop", handlers.StopMCPServer)

		// Session management
		mcpGroup.POST("/sessions", handlers.CreateSession)
		mcpGroup.DELETE("/sessions/:session_id", handlers.EndSession)
	}

	// 1. Create a server
	testServer := &models.MCPServer{
		UserID:      1,
		Name:        "Test Server",
		Description: "Test Description",
		Status:      "stopped",
	}
	testServer.ID = 1

	mockService.On("CreateMCPServer", uint(1), "Test Server", "Test Description").Return(testServer, nil).Once()

	reqBody := MCPServerCreateInput{
		Name:        "Test Server",
		Description: "Test Description",
	}
	reqBodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/mcp/servers", bytes.NewBuffer(reqBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusCreated, resp.Code)

	var serverResponse models.MCPServer
	err := json.Unmarshal(resp.Body.Bytes(), &serverResponse)
	assert.NoError(t, err)
	assert.Equal(t, "Test Server", serverResponse.Name)

	// 2. Add a tool to the server
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(1)).Return(true, nil).Once()
	mockService.On("AddToolToMCPServer", uint(1), uint(2)).Return(nil).Once()

	toolReqBody := MCPServerToolInput{
		ToolID: 2,
	}
	toolReqBodyBytes, _ := json.Marshal(toolReqBody)
	req = httptest.NewRequest("POST", fmt.Sprintf("/mcp/servers/%d/tools", serverResponse.ID), bytes.NewBuffer(toolReqBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()

	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNoContent, resp.Code)

	// 3. Start the server
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(1)).Return(true, nil).Once()
	mockService.On("StartMCPServer", uint(1)).Return(nil).Once()

	req = httptest.NewRequest("POST", fmt.Sprintf("/mcp/servers/%d/start", serverResponse.ID), nil)
	resp = httptest.NewRecorder()

	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNoContent, resp.Code)

	// 4. Create a session
	testSession := &models.MCPSession{
		MCPServerID: 1,
		UserID:      1,
		SessionID:   "test-session-id",
		ClientID:    "test-client-id",
		Active:      true,
	}
	testSession.ID = 1

	mockService.On("CreateSession", uint(1), uint(1), "test-client-id").Return(testSession, nil).Once()

	sessionReqBody := MCPSessionCreateInput{
		MCPServerID: 1,
		ClientID:    "test-client-id",
	}
	sessionReqBodyBytes, _ := json.Marshal(sessionReqBody)
	req = httptest.NewRequest("POST", "/mcp/sessions", bytes.NewBuffer(sessionReqBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()

	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusCreated, resp.Code)

	var sessionResponse models.MCPSession
	err = json.Unmarshal(resp.Body.Bytes(), &sessionResponse)
	assert.NoError(t, err)
	assert.Equal(t, "test-session-id", sessionResponse.SessionID)

	// 5. End the session
	mockService.On("EndSession", "test-session-id").Return(nil).Once()

	req = httptest.NewRequest("DELETE", "/mcp/sessions/test-session-id", nil)
	resp = httptest.NewRecorder()

	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNoContent, resp.Code)

	// 6. Stop the server
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(1)).Return(true, nil).Once()
	mockService.On("StopMCPServer", uint(1)).Return(nil).Once()

	req = httptest.NewRequest("POST", fmt.Sprintf("/mcp/servers/%d/stop", serverResponse.ID), nil)
	resp = httptest.NewRecorder()

	r.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNoContent, resp.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}
