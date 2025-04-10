package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// TestHandlers is a wrapper that implements handler functions
// necessary for testing, redirecting to the MCPServiceMock
type TestHandlers struct {
	mock *MCPServiceMock
}

func (h *TestHandlers) CreateMCPServer(c *gin.Context) {
	var input MCPServerCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, exists := fixUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	server, err := h.mock.CreateMCPServer(userID, input.Name, input.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, server)
}

func (h *TestHandlers) GetMCPServers(c *gin.Context) {
	userID, exists := fixUserIDFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	servers, err := h.mock.GetMCPServersByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, servers)
}

func (h *TestHandlers) GetMCPServer(c *gin.Context) {
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	server, err := h.mock.GetMCPServerByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, server)
}

func (h *TestHandlers) UpdateMCPServer(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid server ID"})
		return
	}

	var input MCPServerUpdateInput
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	server, err := h.mock.UpdateMCPServer(uint(id), input.Name, input.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, server)
}

func (h *TestHandlers) DeleteMCPServer(c *gin.Context) {
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.mock.DeleteMCPServer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.String(http.StatusNoContent, "")
}

func TestMCPHandlersCreateServer(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case 1: Successful creation
	testServer := &models.MCPServer{
		UserID:      1,
		Name:        "Test Server",
		Description: "Test Description",
		Status:      "stopped",
	}
	testServer.ID = 1

	mockService.On("CreateMCPServer", uint(1), "Test Server", "Test Description").Return(testServer, nil).Once()

	// Create the test context with the request body
	c, w := setupTestContext()
	reqBody := MCPServerCreateInput{
		Name:        "Test Server",
		Description: "Test Description",
	}
	reqBodyBytes, _ := json.Marshal(reqBody)
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call the handler
	handlers.CreateMCPServer(c)

	// Assert response
	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.MCPServer
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Server", response.Name)

	// Test case 2: Creation fails
	mockService.On("CreateMCPServer", uint(1), "Bad Server", "Bad Description").Return(nil, errors.New("failed to create server")).Once()

	c, w = setupTestContext()
	reqBody = MCPServerCreateInput{
		Name:        "Bad Server",
		Description: "Bad Description",
	}
	reqBodyBytes, _ = json.Marshal(reqBody)
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBuffer(reqBodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call the handler
	handlers.CreateMCPServer(c)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}

func TestMCPHandlersGetServers(t *testing.T) {
	// Set up mock service and handlers
	mockService := new(MCPServiceMock)
	handlers := &TestHandlers{
		mock: mockService,
	}

	// Test case: Get servers for user
	testServers := []models.MCPServer{
		{
			Model: gorm.Model{
				ID: 1,
			},
			UserID:      1,
			Name:        "Server 1",
			Description: "Description 1",
			Status:      "stopped",
		},
		{
			Model: gorm.Model{
				ID: 2,
			},
			UserID:      1,
			Name:        "Server 2",
			Description: "Description 2",
			Status:      "running",
		},
	}

	mockService.On("GetMCPServersByUserID", uint(1)).Return(testServers, nil).Once()

	// Create the test context
	c, w := setupTestContext()

	// Call the handler
	handlers.GetMCPServers(c)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.MCPServer
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, "Server 1", response[0].Name)
	assert.Equal(t, "Server 2", response[1].Name)

	// Verify all expectations were met
	mockService.AssertExpectations(t)
}
