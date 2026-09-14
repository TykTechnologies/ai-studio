package models

import (
	"time"

	"gorm.io/gorm"
)

// NotifyAdmins is a flag used to indicate that a notification should be sent to all admin users
const NotifyAdmins uint = 1 << 31 // Using the highest bit: 0x80000000

// Notification is one in-app notification for one recipient. The JSON
// names are what the bell panel consumes; the same columns as gorm.Model
// are spelled out so they serialise in lowercase too.
type Notification struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
	NotificationID string         `json:"notification_id" gorm:"uniqueIndex"` // Unique ID to prevent duplicates
	Type           string         `json:"type"`                               // e.g. "budget_alert", "system_update", etc.
	Title          string         `json:"title"`
	Content        string         `json:"content"`
	UserID         uint           `json:"user_id"`
	Read           bool           `json:"read"`    // For UI display
	SentAt         time.Time      `json:"sent_at"` // When the notification was sent
	// Link is the in-app path the notification points at, e.g.
	// "/admin/apps/3"; empty when there is nothing to open.
	Link string `json:"link"`
}

// TableName specifies the table name for the Notification model
func (Notification) TableName() string {
	return "notifications"
}
