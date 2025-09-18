// internal/database/repository.go
package database

import (
	"time"

	"gorm.io/gorm"
)

// Repository provides database operations
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new repository instance
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// LLM repository methods

// CreateLLM creates a new LLM
func (r *Repository) CreateLLM(llm *LLM) error {
	return r.db.Create(llm).Error
}

// GetLLM retrieves an LLM by ID
func (r *Repository) GetLLM(id uint) (*LLM, error) {
	var llm LLM
	err := r.db.Preload("Apps").Preload("Filters").First(&llm, id).Error
	if err != nil {
		return nil, err
	}
	return &llm, nil
}

// GetLLMBySlug retrieves an LLM by slug
func (r *Repository) GetLLMBySlug(slug string) (*LLM, error) {
	var llm LLM
	err := r.db.Preload("Apps").Preload("Filters").Where("slug = ? AND is_active = ?", slug, true).First(&llm).Error
	if err != nil {
		return nil, err
	}
	return &llm, nil
}

// ListLLMs retrieves LLMs with pagination and filtering
func (r *Repository) ListLLMs(page, limit int, vendor string, isActive bool) ([]LLM, int64, error) {
	var llms []LLM
	var total int64

	query := r.db.Model(&LLM{})
	
	if vendor != "" {
		query = query.Where("vendor = ?", vendor)
	}
	query = query.Where("is_active = ?", isActive)
	
	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Preload("Apps").Preload("Filters").Find(&llms).Error
	
	return llms, total, err
}

// UpdateLLM updates an existing LLM
func (r *Repository) UpdateLLM(llm *LLM) error {
	return r.db.Save(llm).Error
}

// DeleteLLM soft deletes an LLM
func (r *Repository) DeleteLLM(id uint) error {
	return r.db.Delete(&LLM{}, id).Error
}

// GetActiveLLMs retrieves all active LLMs
func (r *Repository) GetActiveLLMs() ([]LLM, error) {
	var llms []LLM
	err := r.db.Where("is_active = ?", true).Preload("Filters").Find(&llms).Error
	return llms, err
}

// App repository methods

// CreateApp creates a new app
func (r *Repository) CreateApp(app *App) error {
	return r.db.Create(app).Error
}

// GetApp retrieves an app by ID
func (r *Repository) GetApp(id uint) (*App, error) {
	var app App
	err := r.db.Preload("Credentials").Preload("Tokens").Preload("LLMs").First(&app, id).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// ListApps retrieves apps with pagination
func (r *Repository) ListApps(page, limit int, isActive bool) ([]App, int64, error) {
	var apps []App
	var total int64

	query := r.db.Model(&App{}).Where("is_active = ?", isActive)
	
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Preload("Credentials").Preload("LLMs").Find(&apps).Error
	
	return apps, total, err
}

// UpdateApp updates an existing app
func (r *Repository) UpdateApp(app *App) error {
	return r.db.Save(app).Error
}

// DeleteApp soft deletes an app
func (r *Repository) DeleteApp(id uint) error {
	return r.db.Delete(&App{}, id).Error
}

// Credential repository methods

// CreateCredential creates a new credential
func (r *Repository) CreateCredential(cred *Credential) error {
	return r.db.Create(cred).Error
}

// GetCredentialBySecret retrieves credential by secret hash
func (r *Repository) GetCredentialBySecret(secretHash string) (*Credential, error) {
	var cred Credential
	err := r.db.Where("secret_hash = ? AND is_active = ?", secretHash, true).
		Preload("App").First(&cred).Error
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

// GetCredentialByKeyID retrieves credential by key ID
func (r *Repository) GetCredentialByKeyID(keyID string) (*Credential, error) {
	var cred Credential
	err := r.db.Where("key_id = ? AND is_active = ?", keyID, true).
		Preload("App").First(&cred).Error
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

// ListCredentials retrieves credentials for an app
func (r *Repository) ListCredentials(appID uint) ([]Credential, error) {
	var creds []Credential
	err := r.db.Where("app_id = ? AND is_active = ?", appID, true).Find(&creds).Error
	return creds, err
}

// UpdateCredential updates a credential's last used time
func (r *Repository) UpdateCredentialLastUsed(id uint) error {
	return r.db.Model(&Credential{}).Where("id = ?", id).Update("last_used_at", time.Now()).Error
}

// DeleteCredential soft deletes a credential
func (r *Repository) DeleteCredential(id uint) error {
	return r.db.Delete(&Credential{}, id).Error
}

// APIToken repository methods

// CreateAPIToken creates a new API token
func (r *Repository) CreateAPIToken(token *APIToken) error {
	return r.db.Create(token).Error
}

// GetAPIToken retrieves an API token by token value
func (r *Repository) GetAPIToken(token string) (*APIToken, error) {
	var apiToken APIToken
	err := r.db.Where("token = ? AND is_active = ?", token, true).
		Preload("App").First(&apiToken).Error
	if err != nil {
		return nil, err
	}
	return &apiToken, nil
}

// ListAPITokens retrieves API tokens for an app
func (r *Repository) ListAPITokens(appID uint) ([]APIToken, error) {
	var tokens []APIToken
	err := r.db.Where("app_id = ? AND is_active = ?", appID, true).Find(&tokens).Error
	return tokens, err
}

// UpdateAPITokenLastUsed updates token's last used time
func (r *Repository) UpdateAPITokenLastUsed(id uint) error {
	return r.db.Model(&APIToken{}).Where("id = ?", id).Update("last_used_at", time.Now()).Error
}

// RevokeAPIToken deactivates an API token
func (r *Repository) RevokeAPIToken(token string) error {
	return r.db.Model(&APIToken{}).Where("token = ?", token).Update("is_active", false).Error
}

// BudgetUsage repository methods

// GetOrCreateBudgetUsage gets or creates budget usage record for a period
func (r *Repository) GetOrCreateBudgetUsage(appID uint, llmID *uint, periodStart, periodEnd time.Time) (*BudgetUsage, error) {
	var usage BudgetUsage
	
	err := r.db.Where(BudgetUsage{
		AppID:       appID,
		LLMID:       llmID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	}).FirstOrCreate(&usage).Error
	
	return &usage, err
}

// UpdateBudgetUsage updates budget usage statistics
func (r *Repository) UpdateBudgetUsage(id uint, tokensUsed int64, requestsCount int, totalCost float64, promptTokens, completionTokens int64) error {
	return r.db.Model(&BudgetUsage{}).Where("id = ?", id).Updates(map[string]interface{}{
		"tokens_used":       gorm.Expr("tokens_used + ?", tokensUsed),
		"requests_count":    gorm.Expr("requests_count + ?", requestsCount),
		"total_cost":        gorm.Expr("total_cost + ?", totalCost),
		"prompt_tokens":     gorm.Expr("prompt_tokens + ?", promptTokens),
		"completion_tokens": gorm.Expr("completion_tokens + ?", completionTokens),
		"updated_at":        time.Now(),
	}).Error
}

// GetBudgetUsage retrieves budget usage for an app and period
func (r *Repository) GetBudgetUsage(appID uint, llmID *uint, periodStart, periodEnd time.Time) (*BudgetUsage, error) {
	var usage BudgetUsage
	query := r.db.Where("app_id = ? AND period_start = ? AND period_end = ?", appID, periodStart, periodEnd)
	
	if llmID != nil {
		query = query.Where("llm_id = ?", *llmID)
	} else {
		query = query.Where("llm_id IS NULL")
	}
	
	err := query.First(&usage).Error
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

// AnalyticsEvent repository methods

// CreateAnalyticsEvent creates a new analytics event
func (r *Repository) CreateAnalyticsEvent(event *AnalyticsEvent) error {
	return r.db.Create(event).Error
}

// CreateAnalyticsEventsBatch creates analytics events in batch
func (r *Repository) CreateAnalyticsEventsBatch(events []AnalyticsEvent) error {
	return r.db.CreateInBatches(events, 100).Error
}

// GetAnalyticsEvents retrieves analytics events with pagination
func (r *Repository) GetAnalyticsEvents(appID uint, page, limit int) ([]AnalyticsEvent, int64, error) {
	var events []AnalyticsEvent
	var total int64

	query := r.db.Model(&AnalyticsEvent{}).Where("app_id = ?", appID)
	
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).
		Preload("App").Preload("LLM").Preload("Credential").
		Order("created_at DESC").Find(&events).Error
	
	return events, total, err
}

// Transaction support

// WithTransaction executes a function within a database transaction
func (r *Repository) WithTransaction(fn func(*Repository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		txRepo := &Repository{db: tx}
		return fn(txRepo)
	})
}