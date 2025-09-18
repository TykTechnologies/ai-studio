// internal/auth/interface.go
package auth

import (
	"time"
)

// AuthProvider defines the interface for authentication providers
type AuthProvider interface {
	// ValidateToken validates an API token and returns auth result
	ValidateToken(token string) (*AuthResult, error)
	
	// GenerateToken creates a new API token
	GenerateToken(appID uint, name string, scopes []string, expiresIn time.Duration) (string, error)
	
	// RevokeToken deactivates an API token
	RevokeToken(token string) error
	
	// GetTokenInfo returns information about a token without validating it
	GetTokenInfo(token string) (*TokenInfo, error)
}

// AuthResult represents the result of token validation
type AuthResult struct {
	Valid     bool
	AppID     uint
	Scopes    []string
	ExpiresAt *time.Time
	Error     string
}

// TokenInfo contains information about an API token
type TokenInfo struct {
	ID        uint
	Name      string
	AppID     uint
	Scopes    []string
	IsActive  bool
	ExpiresAt *time.Time
	CreatedAt time.Time
	LastUsed  *time.Time
}

// CachedToken represents a cached authentication token
type CachedToken struct {
	Token     string
	AppID     uint
	Scopes    []string
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// CachedCredential represents a cached credential
type CachedCredential struct {
	KeyID      string
	AppID      uint
	SecretHash string
	CachedAt   time.Time
	TTL        time.Duration
}