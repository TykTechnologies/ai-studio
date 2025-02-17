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
	notificationService := NewNotificationService(db, nil) // We'll add mail service later if needed
	return &Service{
		DB:                  db,
		NotificationService: notificationService,
	}
}

func (s *Service) GetDB() *gorm.DB {
	return s.DB
}
