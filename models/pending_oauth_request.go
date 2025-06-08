package models

import (
	"time"

	"gorm.io/gorm"
)

// PendingOAuthRequest stores the details of an OAuth authorization request
// that is awaiting user consent.
type PendingOAuthRequest struct {
	gorm.Model
	ID                  string    `gorm:"type:varchar(255);uniqueIndex;not null"` // The auth_req_id (e.g., UUID)
	ClientID            string    `gorm:"type:varchar(255);not null"`
	UserID              uint      `gorm:"not null"` // The user who needs to consent
	RedirectURI         string    `gorm:"type:text;not null"`
	Scope               string    `gorm:"type:varchar(255)"`
	State               string    `gorm:"type:varchar(255)"` // Optional
	CodeChallenge       string    `gorm:"type:varchar(255);not null"`
	CodeChallengeMethod string    `gorm:"type:varchar(50);not null"`
	ExpiresAt           time.Time `gorm:"not null"` // When this pending request becomes invalid
}
