package services

import (
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"gorm.io/gorm"
)

// TestNotificationService is a mock notification service for testing
type TestNotificationService = NotificationService

// NewTestNotificationService creates a new test notification service
func NewTestNotificationService(db *gorm.DB) *NotificationService {
	return NewNotificationService(db, notifications.NewTestMailService())
}
