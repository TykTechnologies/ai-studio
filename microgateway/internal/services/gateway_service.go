// internal/services/gateway_service.go
package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// DatabaseGatewayService implements GatewayServiceInterface using database storage  
type DatabaseGatewayService struct {
	db   *gorm.DB
	repo *database.Repository
	mu   sync.RWMutex
}

// NewDatabaseGatewayService creates a new database-backed gateway service
func NewDatabaseGatewayService(db *gorm.DB, repo *database.Repository) GatewayServiceInterface {
	return &DatabaseGatewayService{
		db:   db,
		repo: repo,
	}
}

// GetActiveLLMs returns all active LLM configurations
func (s *DatabaseGatewayService) GetActiveLLMs() ([]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	llms, err := s.repo.GetActiveLLMs()
	if err != nil {
		return nil, fmt.Errorf("failed to get active LLMs: %w", err)
	}

	// Convert to interface slice (store pointers)
	result := make([]interface{}, len(llms))
	for i := range llms {
		result[i] = &llms[i]
	}

	return result, nil
}

// GetLLMBySlug retrieves an LLM configuration by its slug
func (s *DatabaseGatewayService) GetLLMBySlug(slug string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	llm, err := s.repo.GetLLMBySlug(slug)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("LLM not found: %s", slug)
		}
		return nil, fmt.Errorf("failed to get LLM by slug: %w", err)
	}

	return llm, nil
}

// GetCredentialBySecret validates a credential secret and returns the credential  
// NOTE: This is legacy - we use token authentication only now
func (s *DatabaseGatewayService) GetCredentialBySecret(secret string) (interface{}, error) {
	return nil, fmt.Errorf("credential authentication not supported - use token authentication")
}

// GetAppByCredentialID returns the app associated with a credential
func (s *DatabaseGatewayService) GetAppByID(id uint) (interface{}, error) {
	var app database.App
	err := s.db.Where("id = ?", id).
		Preload("LLMs").
		First(&app).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("app not found")
		}
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	return &app, nil
}

func (s *DatabaseGatewayService) GetAppByCredentialID(credID uint) (interface{}, error) {
	// Get credential first
	var cred database.Credential
	err := s.db.Where("id = ? AND is_active = ?", credID, true).
		Preload("App").
		First(&cred).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("credential not found")
		}
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}

	if cred.App == nil {
		return nil, fmt.Errorf("app not found for credential")
	}

	if !cred.App.IsActive {
		return nil, fmt.Errorf("app is inactive")
	}

	return cred.App, nil
}

// ValidateAppAccess validates if an app can access a specific LLM
func (s *DatabaseGatewayService) ValidateAppAccess(appID uint, llmSlug string) error {
	// Get LLM by slug
	llm, err := s.repo.GetLLMBySlug(llmSlug)
	if err != nil {
		return fmt.Errorf("LLM not found: %s", llmSlug)
	}

	if !llm.IsActive {
		return fmt.Errorf("LLM is inactive: %s", llmSlug)
	}

	// Check if app has access to this LLM
	var appLLM database.AppLLM
	err = s.db.Where("app_id = ? AND llm_id = ? AND is_active = ?", appID, llm.ID, true).
		First(&appLLM).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("app %d does not have access to LLM %s", appID, llmSlug)
		}
		return fmt.Errorf("failed to check app access: %w", err)
	}

	return nil
}

// Reload reloads the gateway configuration
func (s *DatabaseGatewayService) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// No cache to clear in simplified version
	return nil
}

// hashSecret creates a hash of a secret for database storage
func (s *DatabaseGatewayService) hashSecret(secret string) string {
	// Simple hash for now - in production use proper crypto service
	return fmt.Sprintf("hash:%s", secret) // Simplified for testing
}

// GetLLMStats returns statistics for a specific LLM
func (s *DatabaseGatewayService) GetLLMStats(llmID uint) (map[string]interface{}, error) {
	// Get request count from analytics
	var requestCount int64
	err := s.db.Model(&database.AnalyticsEvent{}).
		Where("llm_id = ?", llmID).
		Count(&requestCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get request count: %w", err)
	}

	// Get total tokens used
	var totalTokens int64
	err = s.db.Model(&database.AnalyticsEvent{}).
		Where("llm_id = ?", llmID).
		Select("COALESCE(SUM(total_tokens), 0)").
		Scan(&totalTokens).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total tokens: %w", err)
	}

	// Get total cost
	var totalCost float64
	err = s.db.Model(&database.AnalyticsEvent{}).
		Where("llm_id = ?", llmID).
		Select("COALESCE(SUM(cost), 0)").
		Scan(&totalCost).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total cost: %w", err)
	}

	return map[string]interface{}{
		"request_count": requestCount,
		"total_tokens":  totalTokens,
		"total_cost":    totalCost,
	}, nil
}

// ValidateAPIToken validates an API token and returns token information
func (s *DatabaseGatewayService) ValidateAPIToken(token string) (*TokenValidationResult, error) {
	var apiToken database.APIToken
	err := s.db.Where("token = ? AND is_active = ?", token, true).
		Preload("App").
		First(&apiToken).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("invalid or expired token")
		}
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	// Check expiration
	if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("token expired")
	}

	// Check if app is active
	if apiToken.App != nil && !apiToken.App.IsActive {
		return nil, fmt.Errorf("app is inactive")
	}

	return &TokenValidationResult{
		TokenID:   apiToken.ID,
		TokenName: apiToken.Name,
		AppID:     apiToken.AppID,
		App:       apiToken.App,
	}, nil
}

// TokenValidationResult represents the result of token validation
type TokenValidationResult struct {
	TokenID   uint
	TokenName string
	AppID     uint
	App       *database.App
}

// GetAppByTokenID returns the app associated with a token ID
func (s *DatabaseGatewayService) GetAppByTokenID(tokenID uint) (*database.App, error) {
	var token database.APIToken
	err := s.db.Where("id = ? AND is_active = ?", tokenID, true).
		Preload("App.LLMs").
		First(&token).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	if token.App == nil {
		return nil, fmt.Errorf("app not found for token")
	}

	if !token.App.IsActive {
		return nil, fmt.Errorf("app is inactive")
	}

	// Debug: Log the LLM associations that were loaded
	log.Debug().
		Uint("token_id", tokenID).
		Uint("app_id", token.App.ID).
		Str("app_name", token.App.Name).
		Int("llm_associations_count", len(token.App.LLMs)).
		Msg("Retrieved app by token ID with LLM associations")
		
	for _, llm := range token.App.LLMs {
		log.Debug().
			Uint("app_id", token.App.ID).
			Uint("llm_id", llm.ID).
			Str("llm_slug", llm.Slug).
			Msg("App-LLM association loaded from database")
	}

	return token.App, nil
}