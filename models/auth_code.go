package models

import (
	"time"

	"gorm.io/gorm"
)

// AuthCode represents an OAuth 2.0 authorization code.
type AuthCode struct {
	gorm.Model
	Code                string    `gorm:"type:varchar(255);uniqueIndex;not null"` // The authorization code itself
	ClientID            string    `gorm:"type:varchar(255);not null"`
	UserID              uint      `gorm:"not null"`
	RedirectURI         string    `gorm:"type:text;not null"`
	Scope               string    `gorm:"type:varchar(255)"`
	ExpiresAt           time.Time `gorm:"not null"`
	CodeChallenge       string    `gorm:"type:varchar(255)"` // For PKCE
	CodeChallengeMethod string    `gorm:"type:varchar(50)"`  // For PKCE (e.g., "S256")
	Used                bool      `gorm:"default:false"`
	AppID               *uint     `gorm:"column:app_id"` // Selected app ID for MCP OAuth
}
