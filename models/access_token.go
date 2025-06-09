package models

import (
	"time"

	"gorm.io/gorm"
)

// AccessToken represents an OAuth 2.0 access token.
type AccessToken struct {
	gorm.Model
	Token     string    `gorm:"type:varchar(255);uniqueIndex;not null"` // The access token itself
	ClientID  string    `gorm:"type:varchar(255);not null"`
	UserID    uint      `gorm:"not null"`
	Scope     string    `gorm:"type:varchar(255)"`
	ExpiresAt time.Time `gorm:"not null"`
}
