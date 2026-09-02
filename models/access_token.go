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
	// AppID is the app the user selected at consent. It is what the token is
	// authorised against: the gateway resolves this app and checks the requested
	// tool against app.Tools, exactly as the app-secret and API-key paths do.
	// Nullable so pre-existing rows migrate, but a token without it authorises
	// nothing - see the OAuth branch of proxy.CredentialValidator.
	AppID *uint `gorm:"column:app_id;index"`
}
