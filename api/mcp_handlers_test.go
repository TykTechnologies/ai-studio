package api

import (
	"net/http/httptest"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MCPServiceMock is a mock implementation of the MCPService API
type MCPServiceMock struct {
	mock.Mock
}

// These methods implement all the functions we need from the MCPService
func (m *MCPServiceMock) CreateMCPServer(userID uint, name, description string) (*models.MCPServer, error) {
	args := m.Called(userID, name, description)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MCPServer), args.Error(1)
}

func (m *MCPServiceMock) GetMCPServerByID(id uint) (*models.MCPServer, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MCPServer), args.Error(1)
}

func (m *MCPServiceMock) GetMCPServersByUserID(userID uint) ([]models.MCPServer, error) {
	args := m.Called(userID)
	return args.Get(0).([]models.MCPServer), args.Error(1)
}

func (m *MCPServiceMock) UpdateMCPServer(id uint, name, description string) (*models.MCPServer, error) {
	args := m.Called(id, name, description)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MCPServer), args.Error(1)
}

func (m *MCPServiceMock) DeleteMCPServer(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MCPServiceMock) UserHasAccessToMCPServer(userID, mcpServerID uint) (bool, error) {
	args := m.Called(userID, mcpServerID)
	return args.Bool(0), args.Error(1)
}

func (m *MCPServiceMock) AddToolToMCPServer(mcpServerID, toolID uint) error {
	args := m.Called(mcpServerID, toolID)
	return args.Error(0)
}

func (m *MCPServiceMock) RemoveToolFromMCPServer(mcpServerID, toolID uint) error {
	args := m.Called(mcpServerID, toolID)
	return args.Error(0)
}

func (m *MCPServiceMock) GetToolsForMCPServer(mcpServerID uint) ([]models.Tool, error) {
	args := m.Called(mcpServerID)
	return args.Get(0).([]models.Tool), args.Error(1)
}

func (m *MCPServiceMock) StartMCPServer(mcpServerID uint) error {
	args := m.Called(mcpServerID)
	return args.Error(0)
}

func (m *MCPServiceMock) StopMCPServer(mcpServerID uint) error {
	args := m.Called(mcpServerID)
	return args.Error(0)
}

func (m *MCPServiceMock) RestartMCPServer(mcpServerID uint) error {
	args := m.Called(mcpServerID)
	return args.Error(0)
}

func (m *MCPServiceMock) CreateSession(mcpServerID, userID uint, clientID string) (*models.MCPSession, error) {
	args := m.Called(mcpServerID, userID, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.MCPSession), args.Error(1)
}

func (m *MCPServiceMock) EndSession(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

func (m *MCPServiceMock) UpdateSessionActivity(sessionID string) error {
	args := m.Called(sessionID)
	return args.Error(0)
}

// Setup test context with a user
func setupTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Set up a test request
	c.Request = httptest.NewRequest("GET", "/", nil)

	// Set up a test user
	user := &models.User{
		Model: gorm.Model{
			ID: 1,
		},
		Email: "testuser@example.com",
		Name:  "testuser",
	}
	c.Set("user", user)

	return c, w
}

// Type assertion to ensure our mock implements the necessary interface
var _ MCPServiceInterface = (*MCPServiceMock)(nil)

// MCPServiceInterface defines the interface that both the real service and our mock implement
type MCPServiceInterface interface {
	CreateMCPServer(userID uint, name, description string) (*models.MCPServer, error)
	GetMCPServerByID(id uint) (*models.MCPServer, error)
	GetMCPServersByUserID(userID uint) ([]models.MCPServer, error)
	UpdateMCPServer(id uint, name, description string) (*models.MCPServer, error)
	DeleteMCPServer(id uint) error
	UserHasAccessToMCPServer(userID, mcpServerID uint) (bool, error)
	AddToolToMCPServer(mcpServerID, toolID uint) error
	RemoveToolFromMCPServer(mcpServerID, toolID uint) error
	GetToolsForMCPServer(mcpServerID uint) ([]models.Tool, error)
	StartMCPServer(mcpServerID uint) error
	StopMCPServer(mcpServerID uint) error
	RestartMCPServer(mcpServerID uint) error
	CreateSession(mcpServerID, userID uint, clientID string) (*models.MCPSession, error)
	EndSession(sessionID string) error
	UpdateSessionActivity(sessionID string) error
}
