package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// Session handler implementations for the TestHandlers struct
func (h *TestHandlers) CreateSession(c *gin.Context) {
	var input MCPSessionCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := fixUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	session, err := h.mock.CreateSession(input.MCPServerID, userID, input.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, session)
}

func (h *TestHandlers) EndSession(c *gin.Context) {
	sessionID := c.Param("session_id")

	if err := h.mock.EndSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.String(http.StatusNoContent, "")
}

func (h *TestHandlers) UpdateSessionActivity(c *gin.Context) {
	sessionID := c.Param("session_id")

	if err := h.mock.UpdateSessionActivity(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.String(http.StatusNoContent, "")
}

func TestMCPHandlersCreateSession(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case 1: Create session successfully
	testSession := &models.MCPSession{
		MCPServerID: 1,
		UserID:      1,
		SessionID:   "test-session-id",
		ClientID:    "test-client-id",
		Active:      true,
	}
	testSession.ID = 1

	mockService.On("CreateSession", uint(1), uint(1), "test-client-id").Return(testSession, nil).Once()

	// Create the test context with the request body
	c, w := setupTestContext()
	reqBody := MCPSessionCreateInput{
		MCPServerID: 1,
		ClientID:    "test-client-id",
	}
	reqBodyBytes, _ := json.Marshal(reqBody)
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call the handler
	handlers.CreateSession(c)

	// Assert response
	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.MCPSession
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test-session-id", response.SessionID)

	// Test case 2: Creation fails
	mockService.On("CreateSession", uint(2), uint(1), "test-client-id").Return(nil, errors.New("failed to create session")).Once()

	c, w = setupTestContext()
	reqBody = MCPSessionCreateInput{
		MCPServerID: 2,
		ClientID:    "test-client-id",
	}
	reqBodyBytes, _ = json.Marshal(reqBody)
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call the handler
	handlers.CreateSession(c)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}

func TestMCPHandlersEndSession(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case 1: End session successfully
	mockService.On("EndSession", "test-session-id").Return(nil).Once()

	// Create the test context
	c, w := setupTestContext()
	c.Params = gin.Params{{Key: "session_id", Value: "test-session-id"}}

	// Call the handler
	handlers.EndSession(c)

	// Assert response
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test case 2: End session fails
	mockService.On("EndSession", "bad-session-id").Return(errors.New("failed to end session")).Once()

	c, w = setupTestContext()
	c.Params = gin.Params{{Key: "session_id", Value: "bad-session-id"}}

	// Call the handler
	handlers.EndSession(c)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}

func TestMCPHandlersUpdateSessionActivity(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case 1: Update session activity successfully
	mockService.On("UpdateSessionActivity", "test-session-id").Return(nil).Once()

	// Create the test context
	c, w := setupTestContext()
	c.Params = gin.Params{{Key: "session_id", Value: "test-session-id"}}

	// Call the handler
	handlers.UpdateSessionActivity(c)

	// Assert response
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test case 2: Update session activity fails
	mockService.On("UpdateSessionActivity", "bad-session-id").Return(errors.New("failed to update session activity")).Once()

	c, w = setupTestContext()
	c.Params = gin.Params{{Key: "session_id", Value: "bad-session-id"}}

	// Call the handler
	handlers.UpdateSessionActivity(c)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}
