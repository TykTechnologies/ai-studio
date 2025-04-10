package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

type Service struct {
	DB                  *gorm.DB
	Budget              *BudgetService
	NotificationService *NotificationService
	MCP                 *MCPService
}

func NewService(db *gorm.DB) *Service {
	secrets.SetDBRef(db)
	notificationService := NewNotificationService(db, "", "", 0, "", "", nil) // SMTP will be configured when needed
	budgetService := NewBudgetService(db, notificationService)
	secretService := secrets.NewSecretService(db)
	mcpService := NewMCPService(db, secretService, notificationService)

	return &Service{
		DB:                  db,
		NotificationService: notificationService,
		Budget:              budgetService,
		MCP:                 mcpService,
	}
}

func (s *Service) GetDB() *gorm.DB {
	return s.DB
}

// CreateMCPServer forwards to the MCP service
func (s *Service) CreateMCPServer(userID uint, name, description string) (*models.MCPServer, error) {
	return s.MCP.CreateMCPServer(userID, name, description)
}

// GetMCPServerByID forwards to the MCP service
func (s *Service) GetMCPServerByID(id uint) (*models.MCPServer, error) {
	return s.MCP.GetMCPServerByID(id)
}

// GetMCPServersByUserID forwards to the MCP service
func (s *Service) GetMCPServersByUserID(userID uint) ([]models.MCPServer, error) {
	return s.MCP.GetMCPServersByUserID(userID)
}

// UpdateMCPServer forwards to the MCP service
func (s *Service) UpdateMCPServer(id uint, name, description string) (*models.MCPServer, error) {
	return s.MCP.UpdateMCPServer(id, name, description)
}

// DeleteMCPServer forwards to the MCP service
func (s *Service) DeleteMCPServer(id uint) error {
	return s.MCP.DeleteMCPServer(id)
}

// UserHasAccessToMCPServer forwards to the MCP service
func (s *Service) UserHasAccessToMCPServer(userID, mcpServerID uint) (bool, error) {
	return s.MCP.UserHasAccessToMCPServer(userID, mcpServerID)
}

// AddToolToMCPServer forwards to the MCP service
func (s *Service) AddToolToMCPServer(mcpServerID, toolID uint) error {
	return s.MCP.AddToolToMCPServer(mcpServerID, toolID)
}

// RemoveToolFromMCPServer forwards to the MCP service
func (s *Service) RemoveToolFromMCPServer(mcpServerID, toolID uint) error {
	return s.MCP.RemoveToolFromMCPServer(mcpServerID, toolID)
}

// GetToolsForMCPServer forwards to the MCP service
func (s *Service) GetToolsForMCPServer(mcpServerID uint) ([]models.Tool, error) {
	return s.MCP.GetToolsForMCPServer(mcpServerID)
}
