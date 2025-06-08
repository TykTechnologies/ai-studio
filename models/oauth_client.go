package models

import "gorm.io/gorm"

// OAuthClient represents an OAuth 2.0 client application.
type OAuthClient struct {
	gorm.Model
	ClientID     string `gorm:"type:varchar(255);uniqueIndex;not null"`
	ClientSecret string `gorm:"type:varchar(255);not null"` // Store hashed
	ClientName   string `gorm:"type:varchar(255);not null"`
	RedirectURIs string `gorm:"type:text;not null"`         // Comma-separated or JSON array
	UserID       uint   `gorm:"not null"`                   // Foreign key to users table
	User         User   // GORM association
	Scope        string `gorm:"type:varchar(255)"` // Space-separated scopes
}
