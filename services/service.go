package services

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

type Service struct {
	DB                  *gorm.DB
	Budget              *BudgetService
	NotificationService *NotificationService
	// Hub-and-Spoke Services
	EdgeService      *EdgeService
	NamespaceService *NamespaceService
	PluginService    *PluginService
}

func NewService(db *gorm.DB) *Service {
	secrets.SetDBRef(db)
	notificationService := NewNotificationService(db, "", "", 0, "", "", nil) // SMTP will be configured when needed
	budgetService := NewBudgetService(db, notificationService)
	
	// Initialize hub-and-spoke services
	edgeService := NewEdgeService(db)
	namespaceService := NewNamespaceService(db, edgeService)
	pluginService := NewPluginService(db)
	
	return &Service{
		DB:                  db,
		NotificationService: notificationService,
		Budget:              budgetService,
		EdgeService:         edgeService,
		NamespaceService:    namespaceService,
		PluginService:       pluginService,
	}
}

func (s *Service) GetDB() *gorm.DB {
	return s.DB
}
