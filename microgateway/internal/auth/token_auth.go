// internal/auth/token_auth.go
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"gorm.io/gorm"
)

// TokenAuthProvider implements AuthProvider using database-backed token authentication
type TokenAuthProvider struct {
	db *gorm.DB
}

// NewTokenAuthProvider creates a new token authentication provider
func NewTokenAuthProvider(db *gorm.DB) *TokenAuthProvider {
	return &TokenAuthProvider{
		db: db,
	}
}

// ValidateToken checks if a token is valid and returns authentication result
func (p *TokenAuthProvider) ValidateToken(token string) (*AuthResult, error) {
	// Query database directly (no caching for simplicity)
	var apiToken database.APIToken
	err := p.db.Where("token = ? AND is_active = ?", token, true).
		Preload("App").
		First(&apiToken).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &AuthResult{
				Valid: false,
				Error: "Invalid token",
			}, nil
		}
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	// Check expiration
	if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
		return &AuthResult{
			Valid: false,
			Error: "Token expired",
		}, nil
	}

	// Check if app is active
	if apiToken.App != nil && !apiToken.App.IsActive {
		return &AuthResult{
			Valid: false,
			Error: "App is inactive",
		}, nil
	}

	// Update last used timestamp asynchronously
	go func() {
		p.db.Model(&apiToken).Update("last_used_at", time.Now())
	}()

	// Parse scopes
	var scopes []string
	if len(apiToken.Scopes) > 0 {
		if err := json.Unmarshal(apiToken.Scopes, &scopes); err != nil {
			scopes = []string{}
		}
	}

	return &AuthResult{
		Valid:     true,
		AppID:     apiToken.AppID,
		Scopes:    scopes,
		ExpiresAt: apiToken.ExpiresAt,
	}, nil
}

// GenerateToken creates a new API token
func (p *TokenAuthProvider) GenerateToken(appID uint, name string, scopes []string, expiresIn time.Duration) (string, error) {
	// Verify app exists and is active
	var app database.App
	if err := p.db.Where("id = ? AND is_active = ?", appID, true).First(&app).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("app not found or inactive")
		}
		return "", fmt.Errorf("failed to verify app: %w", err)
	}

	// Generate secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	token := hex.EncodeToString(tokenBytes)

	// Calculate expiration
	var expiresAt *time.Time
	if expiresIn > 0 {
		exp := time.Now().Add(expiresIn)
		expiresAt = &exp
	}

	// Marshal scopes to JSON
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return "", fmt.Errorf("failed to marshal scopes: %w", err)
	}

	// Store in database
	apiToken := database.APIToken{
		Token:     token,
		Name:      name,
		AppID:     appID,
		Scopes:    scopesJSON,
		IsActive:  true,
		ExpiresAt: expiresAt,
	}

	if err := p.db.Create(&apiToken).Error; err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	return token, nil
}

// RevokeToken deactivates an API token
func (p *TokenAuthProvider) RevokeToken(token string) error {
	result := p.db.Model(&database.APIToken{}).
		Where("token = ?", token).
		Update("is_active", false)

	if result.Error != nil {
		return fmt.Errorf("failed to revoke token: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// GetTokenInfo returns information about a token without validating it
func (p *TokenAuthProvider) GetTokenInfo(token string) (*TokenInfo, error) {
	var apiToken database.APIToken
	err := p.db.Where("token = ?", token).
		Preload("App").
		First(&apiToken).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get token info: %w", err)
	}

	// Parse scopes
	var scopes []string
	if len(apiToken.Scopes) > 0 {
		if err := json.Unmarshal(apiToken.Scopes, &scopes); err != nil {
			scopes = []string{}
		}
	}

	return &TokenInfo{
		ID:        apiToken.ID,
		Name:      apiToken.Name,
		AppID:     apiToken.AppID,
		Scopes:    scopes,
		IsActive:  apiToken.IsActive,
		ExpiresAt: apiToken.ExpiresAt,
		CreatedAt: apiToken.CreatedAt,
		LastUsed:  apiToken.LastUsedAt,
	}, nil
}

// ListTokensForApp returns all tokens for a specific app
func (p *TokenAuthProvider) ListTokensForApp(appID uint) ([]TokenInfo, error) {
	var apiTokens []database.APIToken
	err := p.db.Where("app_id = ?", appID).
		Order("created_at DESC").
		Find(&apiTokens).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	tokens := make([]TokenInfo, len(apiTokens))
	for i, apiToken := range apiTokens {
		// Parse scopes
		var scopes []string
		if len(apiToken.Scopes) > 0 {
			if err := json.Unmarshal(apiToken.Scopes, &scopes); err != nil {
				scopes = []string{}
			}
		}

		tokens[i] = TokenInfo{
			ID:        apiToken.ID,
			Name:      apiToken.Name,
			AppID:     apiToken.AppID,
			Scopes:    scopes,
			IsActive:  apiToken.IsActive,
			ExpiresAt: apiToken.ExpiresAt,
			CreatedAt: apiToken.CreatedAt,
			LastUsed:  apiToken.LastUsedAt,
		}
	}

	return tokens, nil
}

// CleanupExpiredTokens removes expired tokens from the database
func (p *TokenAuthProvider) CleanupExpiredTokens() error {
	result := p.db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Delete(&database.APIToken{})

	if result.Error != nil {
		return fmt.Errorf("failed to cleanup expired tokens: %w", result.Error)
	}

	// Also remove from cache if needed
	// This would require additional cache methods to iterate and remove expired items

	return nil
}

// GetTokenStats returns statistics about tokens
type TokenStats struct {
	TotalTokens   int64
	ActiveTokens  int64
	ExpiredTokens int64
	AppsWithTokens int64
}

// GetStats returns token statistics
func (p *TokenAuthProvider) GetStats() (*TokenStats, error) {
	var stats TokenStats

	// Total tokens
	if err := p.db.Model(&database.APIToken{}).Count(&stats.TotalTokens).Error; err != nil {
		return nil, fmt.Errorf("failed to count total tokens: %w", err)
	}

	// Active tokens
	if err := p.db.Model(&database.APIToken{}).Where("is_active = ?", true).Count(&stats.ActiveTokens).Error; err != nil {
		return nil, fmt.Errorf("failed to count active tokens: %w", err)
	}

	// Expired tokens
	if err := p.db.Model(&database.APIToken{}).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Count(&stats.ExpiredTokens).Error; err != nil {
		return nil, fmt.Errorf("failed to count expired tokens: %w", err)
	}

	// Apps with tokens
	if err := p.db.Model(&database.APIToken{}).
		Distinct("app_id").
		Count(&stats.AppsWithTokens).Error; err != nil {
		return nil, fmt.Errorf("failed to count apps with tokens: %w", err)
	}

	return &stats, nil
}