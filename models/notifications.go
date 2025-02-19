package models

import (
	"time"

	"gorm.io/gorm"
)

// NotifyAdmins is a flag used to indicate that a notification should be sent to all admin users
const NotifyAdmins uint = 1 << 31 // Using the highest bit: 0x80000000

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
