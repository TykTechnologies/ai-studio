package models

import (
	"time"

	"gorm.io/gorm"
)

type Notification struct {
	gorm.Model
	NotificationID string `gorm:"uniqueIndex"` // Unique ID to prevent duplicates
	Type           string // e.g. "budget_alert", "system_update", etc.
	Title          string
	Content        string
	UserID         uint
	Read           bool      // For UI display
	SentAt         time.Time // When the notification was sent
}

// TableName specifies the table name for the Notification model
func (Notification) TableName() string {
	return "notifications"
}
