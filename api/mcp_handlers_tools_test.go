package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// Tool handler implementations for the TestHandlers struct
func (h *TestHandlers) AddToolToMCPServer(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	var input MCPServerToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := fixUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	hasAccess, err := h.mock.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.mock.AddToolToMCPServer(uint(id), input.ToolID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.String(http.StatusNoContent, "")
}

func (h *TestHandlers) RemoveToolFromMCPServer(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	toolID, err := strconv.Atoi(c.Param("tool_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool ID"})
		return
	}

	userID, exists := fixUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	hasAccess, err := h.mock.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.mock.RemoveToolFromMCPServer(uint(id), uint(toolID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.String(http.StatusNoContent, "")
}

func (h *TestHandlers) GetMCPServerTools(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	userID, exists := fixUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	hasAccess, err := h.mock.UserHasAccessToMCPServer(userID, uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	tools, err := h.mock.GetToolsForMCPServer(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tools)
}

func TestMCPHandlersAddToolToServer(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case 1: Add tool successfully
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(1)).Return(true, nil).Once()
	mockService.On("AddToolToMCPServer", uint(1), uint(2)).Return(nil).Once()

	// Create the test context with the request body
	c, w := setupTestContext()
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	reqBody := MCPServerToolInput{
		ToolID: 2,
	}
	reqBodyBytes, _ := json.Marshal(reqBody)
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call the handler
	handlers.AddToolToMCPServer(c)

	// Assert response
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test case 2: Access denied
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(2)).Return(false, nil).Once()

	c, w = setupTestContext()
	c.Params = gin.Params{{Key: "id", Value: "2"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call the handler
	handlers.AddToolToMCPServer(c)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}

func TestMCPHandlersRemoveToolFromServer(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case 1: Remove tool successfully
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(1)).Return(true, nil).Once()
	mockService.On("RemoveToolFromMCPServer", uint(1), uint(2)).Return(nil).Once()

	// Create the test context
	c, w := setupTestContext()
	c.Params = gin.Params{
		{Key: "id", Value: "1"},
		{Key: "tool_id", Value: "2"},
	}

	// Call the handler
	handlers.RemoveToolFromMCPServer(c)

	// Assert response
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test case 2: Access denied
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(2)).Return(false, nil).Once()

	c, w = setupTestContext()
	c.Params = gin.Params{
		{Key: "id", Value: "2"},
		{Key: "tool_id", Value: "2"},
	}

	// Call the handler
	handlers.RemoveToolFromMCPServer(c)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}

func TestMCPHandlersGetServerTools(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case 1: Get tools successfully
	testTools := []models.Tool{
		{
			Model: gorm.Model{
				ID: 1,
			},
			Name:        "Tool 1",
			Description: "Tool 1 Description",
		},
		{
			Model: gorm.Model{
				ID: 2,
			},
			Name:        "Tool 2",
			Description: "Tool 2 Description",
		},
	}

	mockService.On("UserHasAccessToMCPServer", uint(1), uint(1)).Return(true, nil).Once()
	mockService.On("GetToolsForMCPServer", uint(1)).Return(testTools, nil).Once()

	// Create the test context
	c, w := setupTestContext()
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	// Call the handler
	handlers.GetMCPServerTools(c)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.Tool
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, "Tool 1", response[0].Name)
	assert.Equal(t, "Tool 2", response[1].Name)

	// Test case 2: Access denied
	mockService.On("UserHasAccessToMCPServer", uint(1), uint(2)).Return(false, nil).Once()

	c, w = setupTestContext()
	c.Params = gin.Params{{Key: "id", Value: "2"}}

	// Call the handler
	handlers.GetMCPServerTools(c)

	// Assert response
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}
