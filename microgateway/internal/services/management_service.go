// internal/services/management_service.go
package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/gosimple/slug"
	"gorm.io/gorm"
)

// ManagementService implements ManagementServiceInterface for CRUD operations
type ManagementService struct {
	db     *gorm.DB
	repo   *database.Repository
	crypto CryptoServiceInterface
}

// NewManagementService creates a new management service
func NewManagementService(db *gorm.DB, repo *database.Repository, crypto CryptoServiceInterface) ManagementServiceInterface {
	return &ManagementService{
		db:     db,
		repo:   repo,
		crypto: crypto,
	}
}

// LLM Management Implementation

// CreateLLM creates a new LLM configuration
func (s *ManagementService) CreateLLM(req *CreateLLMRequest) (*database.LLM, error) {
	// Validate request
	if err := s.validateCreateLLMRequest(req); err != nil {
		return nil, err
	}

	// Generate slug from name
	llmSlug := slug.Make(req.Name)

	// Check if slug already exists
	exists, err := s.LLMSlugExists(llmSlug)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("LLM with name '%s' already exists", req.Name)
	}

	// Encrypt API key if provided
	encryptedKey := ""
	if req.APIKey != "" {
		encryptedKey, err = s.crypto.Encrypt(req.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
	}

	// Marshal metadata
	var metadata []byte
	if req.Metadata != nil {
		metadata, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Create LLM model
	llm := &database.LLM{
		Name:            req.Name,
		Slug:            llmSlug,
		Vendor:          req.Vendor,
		Endpoint:        req.Endpoint,
		APIKeyEncrypted: encryptedKey,
		DefaultModel:    req.DefaultModel,
		MaxTokens:       s.getOrDefault(req.MaxTokens, 4096),
		TimeoutSeconds:  s.getOrDefault(req.TimeoutSeconds, 30),
		RetryCount:      s.getOrDefault(req.RetryCount, 3),
		IsActive:        req.IsActive,
		MonthlyBudget:   req.MonthlyBudget,
		RateLimitRPM:    req.RateLimitRPM,
		Metadata:        metadata,
	}

	// Save to database
	if err := s.repo.CreateLLM(llm); err != nil {
		return nil, fmt.Errorf("failed to create LLM: %w", err)
	}

	return llm, nil
}

// GetLLM retrieves an LLM by ID
func (s *ManagementService) GetLLM(id uint) (*database.LLM, error) {
	llm, err := s.repo.GetLLM(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("LLM not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get LLM: %w", err)
	}

	return llm, nil
}

// ListLLMs retrieves LLMs with pagination and filtering
func (s *ManagementService) ListLLMs(page, limit int, vendor string, isActive bool) ([]database.LLM, int64, error) {
	llms, total, err := s.repo.ListLLMs(page, limit, vendor, isActive)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list LLMs: %w", err)
	}

	return llms, total, nil
}

// UpdateLLM updates an existing LLM
func (s *ManagementService) UpdateLLM(id uint, req *UpdateLLMRequest) (*database.LLM, error) {
	// Get existing LLM
	llm, err := s.repo.GetLLM(id)
	if err != nil {
		return nil, fmt.Errorf("LLM not found: %w", err)
	}

	// Update fields
	if req.Name != nil {
		llm.Name = *req.Name
		llm.Slug = slug.Make(*req.Name)
	}
	if req.Endpoint != nil {
		llm.Endpoint = *req.Endpoint
	}
	if req.APIKey != nil && *req.APIKey != "" {
		encryptedKey, err := s.crypto.Encrypt(*req.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		llm.APIKeyEncrypted = encryptedKey
	}
	if req.DefaultModel != nil {
		llm.DefaultModel = *req.DefaultModel
	}
	if req.MaxTokens != nil {
		llm.MaxTokens = *req.MaxTokens
	}
	if req.TimeoutSeconds != nil {
		llm.TimeoutSeconds = *req.TimeoutSeconds
	}
	if req.RetryCount != nil {
		llm.RetryCount = *req.RetryCount
	}
	if req.IsActive != nil {
		llm.IsActive = *req.IsActive
	}
	if req.MonthlyBudget != nil {
		llm.MonthlyBudget = *req.MonthlyBudget
	}
	if req.RateLimitRPM != nil {
		llm.RateLimitRPM = *req.RateLimitRPM
	}
	if req.Metadata != nil {
		metadata, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		llm.Metadata = metadata
	}

	// Save changes
	if err := s.repo.UpdateLLM(llm); err != nil {
		return nil, fmt.Errorf("failed to update LLM: %w", err)
	}

	return llm, nil
}

// DeleteLLM soft deletes an LLM
func (s *ManagementService) DeleteLLM(id uint) error {
	if err := s.repo.DeleteLLM(id); err != nil {
		return fmt.Errorf("failed to delete LLM: %w", err)
	}
	return nil
}

// LLMSlugExists checks if an LLM slug already exists
func (s *ManagementService) LLMSlugExists(slug string) (bool, error) {
	_, err := s.repo.GetLLMBySlug(slug)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// App Management Implementation

// CreateApp creates a new application
func (s *ManagementService) CreateApp(req *CreateAppRequest) (*database.App, error) {
	// Validate request
	if req.Name == "" {
		return nil, fmt.Errorf("app name is required")
	}
	if req.OwnerEmail == "" {
		return nil, fmt.Errorf("owner email is required")
	}

	// Prepare allowed IPs
	var allowedIPs []byte
	if len(req.AllowedIPs) > 0 {
		allowedIPs, _ = json.Marshal(req.AllowedIPs)
	}

	// Create app model
	app := &database.App{
		Name:           req.Name,
		Description:    req.Description,
		OwnerEmail:     req.OwnerEmail,
		IsActive:       true,
		MonthlyBudget:  req.MonthlyBudget,
		BudgetResetDay: s.getOrDefault(req.BudgetResetDay, 1),
		RateLimitRPM:   req.RateLimitRPM,
		AllowedIPs:     allowedIPs,
	}

	// Save to database
	if err := s.repo.CreateApp(app); err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}

	// Associate with LLMs if provided
	if len(req.LLMIDs) > 0 {
		if err := s.UpdateAppLLMs(app.ID, req.LLMIDs); err != nil {
			return nil, fmt.Errorf("failed to associate LLMs: %w", err)
		}
	}

	return app, nil
}

// GetApp retrieves an app by ID
func (s *ManagementService) GetApp(id uint) (*database.App, error) {
	app, err := s.repo.GetApp(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("app not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	return app, nil
}

// ListApps retrieves apps with pagination
func (s *ManagementService) ListApps(page, limit int, isActive bool) ([]database.App, int64, error) {
	apps, total, err := s.repo.ListApps(page, limit, isActive)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list apps: %w", err)
	}

	return apps, total, nil
}

// UpdateApp updates an existing app
func (s *ManagementService) UpdateApp(id uint, req *UpdateAppRequest) (*database.App, error) {
	// Get existing app
	app, err := s.repo.GetApp(id)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	// Update fields
	if req.Name != nil {
		app.Name = *req.Name
	}
	if req.Description != nil {
		app.Description = *req.Description
	}
	if req.OwnerEmail != nil {
		app.OwnerEmail = *req.OwnerEmail
	}
	if req.IsActive != nil {
		app.IsActive = *req.IsActive
	}
	if req.MonthlyBudget != nil {
		app.MonthlyBudget = *req.MonthlyBudget
	}
	if req.BudgetResetDay != nil {
		app.BudgetResetDay = *req.BudgetResetDay
	}
	if req.RateLimitRPM != nil {
		app.RateLimitRPM = *req.RateLimitRPM
	}
	if req.AllowedIPs != nil {
		allowedIPs, _ := json.Marshal(req.AllowedIPs)
		app.AllowedIPs = allowedIPs
	}

	// Save changes
	if err := s.repo.UpdateApp(app); err != nil {
		return nil, fmt.Errorf("failed to update app: %w", err)
	}

	return app, nil
}

// DeleteApp soft deletes an app
func (s *ManagementService) DeleteApp(id uint) error {
	if err := s.repo.DeleteApp(id); err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	return nil
}

// Credential Management Implementation

// CreateCredential creates a new credential for an app
func (s *ManagementService) CreateCredential(appID uint, req *CreateCredentialRequest) (*database.Credential, error) {
	// Verify app exists
	_, err := s.repo.GetApp(appID)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	// Generate key pair
	keyID, secret, err := s.crypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Hash the secret
	secretHash := s.crypto.HashSecret(secret)

	// Create credential
	cred := &database.Credential{
		AppID:      appID,
		KeyID:      keyID,
		SecretHash: secretHash,
		Name:       req.Name,
		IsActive:   true,
		ExpiresAt:  req.ExpiresAt,
	}

	if err := s.repo.CreateCredential(cred); err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// For security, we set the plain secret in a temporary field for response
	// This is the only time the client will see the secret
	cred.SecretHash = secret // Temporarily store for response

	return cred, nil
}

// ListCredentials lists all credentials for an app
func (s *ManagementService) ListCredentials(appID uint) ([]database.Credential, error) {
	creds, err := s.repo.ListCredentials(appID)
	if err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}

	return creds, nil
}

// DeleteCredential deletes a credential
func (s *ManagementService) DeleteCredential(credID uint) error {
	if err := s.repo.DeleteCredential(credID); err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}
	return nil
}

// App-LLM Association Management

// GetAppLLMs returns all LLMs associated with an app
func (s *ManagementService) GetAppLLMs(appID uint) ([]database.LLM, error) {
	app, err := s.repo.GetApp(appID)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	return app.LLMs, nil
}

// UpdateAppLLMs updates LLM associations for an app
func (s *ManagementService) UpdateAppLLMs(appID uint, llmIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Remove existing associations
		if err := tx.Where("app_id = ?", appID).Delete(&database.AppLLM{}).Error; err != nil {
			return fmt.Errorf("failed to remove existing associations: %w", err)
		}

		// Add new associations
		for _, llmID := range llmIDs {
			appLLM := database.AppLLM{
				AppID:     appID,
				LLMID:     llmID,
				IsActive:  true,
				CreatedAt: time.Now(),
			}
			if err := tx.Create(&appLLM).Error; err != nil {
				return fmt.Errorf("failed to create app-LLM association: %w", err)
			}
		}

		return nil
	})
}

// Helper functions

// validateCreateLLMRequest validates the create LLM request
func (s *ManagementService) validateCreateLLMRequest(req *CreateLLMRequest) error {
	if req.Name == "" {
		return fmt.Errorf("LLM name is required")
	}
	if req.Vendor == "" {
		return fmt.Errorf("LLM vendor is required")
	}
	if req.DefaultModel == "" {
		return fmt.Errorf("default model is required")
	}

	// Validate vendor-specific requirements
	switch req.Vendor {
	case "openai":
		if req.APIKey == "" {
			return fmt.Errorf("API key is required for OpenAI")
		}
	case "ollama":
		if req.Endpoint == "" {
			return fmt.Errorf("endpoint is required for Ollama")
		}
	case "anthropic":
		if req.APIKey == "" {
			return fmt.Errorf("API key is required for Anthropic")
		}
	case "google", "vertex":
		// May require different validation
	default:
		return fmt.Errorf("unsupported vendor: %s", req.Vendor)
	}

	return nil
}

// getOrDefault returns value if non-zero, otherwise returns default
func (s *ManagementService) getOrDefault(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

// GetLLMWithDecryptedKey returns an LLM with decrypted API key (for admin use)
func (s *ManagementService) GetLLMWithDecryptedKey(id uint) (*database.LLM, error) {
	llm, err := s.GetLLM(id)
	if err != nil {
		return nil, err
	}

	// Decrypt API key if present
	if llm.APIKeyEncrypted != "" {
		decryptedKey, err := s.crypto.Decrypt(llm.APIKeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt API key: %w", err)
		}
		// Store in a temporary field for response (don't modify the original)
		tempLLM := *llm
		tempLLM.APIKeyEncrypted = decryptedKey
		return &tempLLM, nil
	}

	return llm, nil
}

// ToggleLLMStatus toggles the active status of an LLM
func (s *ManagementService) ToggleLLMStatus(id uint) (*database.LLM, error) {
	llm, err := s.repo.GetLLM(id)
	if err != nil {
		return nil, fmt.Errorf("LLM not found: %w", err)
	}

	// Toggle status
	llm.IsActive = !llm.IsActive

	if err := s.repo.UpdateLLM(llm); err != nil {
		return nil, fmt.Errorf("failed to update LLM status: %w", err)
	}

	return llm, nil
}

// GetAppStats returns statistics for a specific app
func (s *ManagementService) GetAppStats(appID uint) (map[string]interface{}, error) {
	// Get request count
	var requestCount int64
	err := s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ?", appID).
		Count(&requestCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get request count: %w", err)
	}

	// Get total cost
	var totalCost float64
	err = s.db.Model(&database.AnalyticsEvent{}).
		Where("app_id = ?", appID).
		Select("COALESCE(SUM(cost), 0)").
		Scan(&totalCost).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get total cost: %w", err)
	}

	// Get credential count
	credCount, err := s.repo.ListCredentials(appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get credential count: %w", err)
	}

	// Get token count
	var tokenCount int64
	err = s.db.Model(&database.APIToken{}).
		Where("app_id = ? AND is_active = ?", appID, true).
		Count(&tokenCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get token count: %w", err)
	}

	return map[string]interface{}{
		"request_count":    requestCount,
		"total_cost":       totalCost,
		"credential_count": len(credCount),
		"token_count":      tokenCount,
	}, nil
}

// Model Pricing Management Implementation

// GetModelPrice retrieves pricing for a specific model and vendor
func (s *ManagementService) GetModelPrice(modelName, vendor string) (*database.ModelPrice, error) {
	var price database.ModelPrice
	err := s.db.Where("model_name = ? AND vendor = ?", modelName, vendor).
		Order("created_at DESC").
		First(&price).Error
	
	if err != nil {
		return nil, fmt.Errorf("pricing not found for model %s from vendor %s: %w", modelName, vendor, err)
	}
	
	return &price, nil
}

// CreateModelPrice creates a new model pricing entry
func (s *ManagementService) CreateModelPrice(req *CreateModelPriceRequest) (*database.ModelPrice, error) {
	// Set defaults
	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}

	price := &database.ModelPrice{
		Vendor:       req.Vendor,
		ModelName:    req.ModelName,
		CPT:          req.CPT,          // Cost per token (completion/output)
		CPIT:         req.CPIT,         // Cost per input token (prompt)  
		CacheWritePT: req.CacheWritePT, // Cost per cache write token
		CacheReadPT:  req.CacheReadPT,  // Cost per cache read token
		Currency:     currency,
	}

	if err := s.db.Create(price).Error; err != nil {
		return nil, fmt.Errorf("failed to create model price: %w", err)
	}

	return price, nil
}

// ListModelPrices lists all model prices, optionally filtered by vendor
func (s *ManagementService) ListModelPrices(vendor string) ([]database.ModelPrice, error) {
	var prices []database.ModelPrice
	query := s.db.Order("vendor, model_name, created_at DESC")  // Use created_at instead of effective_date
	
	if vendor != "" {
		query = query.Where("vendor = ?", vendor)
	}
	
	if err := query.Find(&prices).Error; err != nil {
		return nil, fmt.Errorf("failed to list model prices: %w", err)
	}
	
	return prices, nil
}

// UpdateModelPrice updates an existing model price
func (s *ManagementService) UpdateModelPrice(id uint, req *UpdateModelPriceRequest) (*database.ModelPrice, error) {
	var price database.ModelPrice
	if err := s.db.First(&price, id).Error; err != nil {
		return nil, fmt.Errorf("model price not found: %w", err)
	}

	// Update fields
	if req.CPT != nil {
		price.CPT = *req.CPT
	}
	if req.CPIT != nil {
		price.CPIT = *req.CPIT
	}
	if req.CacheWritePT != nil {
		price.CacheWritePT = *req.CacheWritePT
	}
	if req.CacheReadPT != nil {
		price.CacheReadPT = *req.CacheReadPT
	}
	if req.Currency != nil {
		price.Currency = *req.Currency
	}

	if err := s.db.Save(&price).Error; err != nil {
		return nil, fmt.Errorf("failed to update model price: %w", err)
	}

	return &price, nil
}

// DeleteModelPrice deletes a model price entry
func (s *ManagementService) DeleteModelPrice(id uint) error {
	result := s.db.Delete(&database.ModelPrice{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete model price: %w", result.Error)
	}
	
	if result.RowsAffected == 0 {
		return fmt.Errorf("model price not found")
	}
	
	return nil
}