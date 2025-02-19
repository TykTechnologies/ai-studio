package services

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

type Service struct {
	DB                  *gorm.DB
	Budget              *BudgetService
	NotificationService *NotificationService
}

func NewService(db *gorm.DB) *Service {
	secrets.SetDBRef(db)
	notificationService := NewNotificationService(db, "", "", 0, "", "", nil) // SMTP will be configured when needed
	budgetService := NewBudgetService(db, notificationService)
	return &Service{
		DB:                  db,
		NotificationService: notificationService,
		Budget:              budgetService,
	}
}

func (s *Service) GetDB() *gorm.DB {
	return s.DB
}
