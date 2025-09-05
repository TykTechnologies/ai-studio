// internal/services/gateway_service.go
package services

import (
	"fmt"
	"sync"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"gorm.io/gorm"
)

// DatabaseGatewayService implements GatewayServiceInterface using database storage
type DatabaseGatewayService struct {
	db    *gorm.DB
	repo  *database.Repository
	cache *auth.TokenCache
	mu    sync.RWMutex
}

// NewDatabaseGatewayService creates a new database-backed gateway service
func NewDatabaseGatewayService(db *gorm.DB, repo *database.Repository, cache *auth.TokenCache) GatewayServiceInterface {
	return &DatabaseGatewayService{
		db:    db,
		repo:  repo,
		cache: cache,
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

	// Convert to interface slice
	result := make([]interface{}, len(llms))
	for i, llm := range llms {
		result[i] = llm
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
func (s *DatabaseGatewayService) GetCredentialBySecret(secret string) (interface{}, error) {
	// Check cache first
	if cached := s.cache.GetCredential(secret); cached != nil {
		return cached, nil
	}

	// Hash the secret for lookup
	secretHash := s.hashSecret(secret)

	// Query database
	cred, err := s.repo.GetCredentialBySecret(secretHash)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, fmt.Errorf("credential lookup failed: %w", err)
	}

	// Update last used timestamp
	go s.repo.UpdateCredentialLastUsed(cred.ID)

	// Cache the result
	s.cache.SetCredential(secret, cred.KeyID, cred.AppID, s.cache.GetStats().TTL)

	return cred, nil
}

// GetAppByCredentialID returns the app associated with a credential
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

// Reload reloads the gateway configuration (clears cache)
func (s *DatabaseGatewayService) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear cache to force reload from database
	s.cache.Clear()

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