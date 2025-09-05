// internal/services/interfaces.go
package services

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
)

// GatewayServiceInterface defines the interface for gateway operations
type GatewayServiceInterface interface {
	// GetActiveLLMs returns all active LLMs
	GetActiveLLMs() ([]interface{}, error)

	// GetLLMBySlug returns an LLM by its slug
	GetLLMBySlug(slug string) (interface{}, error)

	// GetCredentialBySecret validates and returns credential
	GetCredentialBySecret(secret string) (interface{}, error)

	// GetAppByCredentialID returns app associated with credential
	GetAppByCredentialID(credID uint) (interface{}, error)

	// ValidateAppAccess validates if app can access the specified LLM
	ValidateAppAccess(appID uint, llmSlug string) error

	// Reload reloads the gateway configuration
	Reload() error
}

// BudgetServiceInterface defines the interface for budget operations
type BudgetServiceInterface interface {
	// CheckBudget validates if the request is within budget limits
	CheckBudget(appID uint, llmID *uint, estimatedCost float64) error

	// RecordUsage records usage for budget tracking
	RecordUsage(appID uint, llmID *uint, tokens int64, cost float64, promptTokens, completionTokens int64) error

	// GetBudgetStatus returns current budget status for an app
	GetBudgetStatus(appID uint, llmID *uint) (*BudgetStatus, error)

	// GetBudgetHistory returns budget usage history
	GetBudgetHistory(appID uint, llmID *uint, startTime, endTime time.Time) ([]BudgetUsage, error)

	// UpdateBudget updates budget limits for an app
	UpdateBudget(appID uint, monthlyBudget float64, resetDay int) error
}

// AnalyticsServiceInterface defines the interface for analytics operations
type AnalyticsServiceInterface interface {
	// RecordRequest records an analytics event
	RecordRequest(ctx context.Context, record interface{}) error

	// GetEvents returns analytics events with pagination
	GetEvents(appID uint, page, limit int) ([]AnalyticsEvent, int64, error)

	// GetSummary returns analytics summary for a time period
	GetSummary(appID uint, startTime, endTime time.Time) (*AnalyticsSummary, error)

	// GetCostAnalysis returns cost analysis data
	GetCostAnalysis(appID uint, startTime, endTime time.Time) (*CostAnalysis, error)

	// Flush flushes buffered analytics data
	Flush() error
}

// ManagementServiceInterface defines the interface for management operations
type ManagementServiceInterface interface {
	// LLM Management
	CreateLLM(req *CreateLLMRequest) (*database.LLM, error)
	GetLLM(id uint) (*database.LLM, error)
	ListLLMs(page, limit int, vendor string, isActive bool) ([]database.LLM, int64, error)
	UpdateLLM(id uint, req *UpdateLLMRequest) (*database.LLM, error)
	DeleteLLM(id uint) error
	LLMSlugExists(slug string) (bool, error)

	// App Management
	CreateApp(req *CreateAppRequest) (*database.App, error)
	GetApp(id uint) (*database.App, error)
	ListApps(page, limit int, isActive bool) ([]database.App, int64, error)
	UpdateApp(id uint, req *UpdateAppRequest) (*database.App, error)
	DeleteApp(id uint) error

	// Credential Management
	CreateCredential(appID uint, req *CreateCredentialRequest) (*database.Credential, error)
	ListCredentials(appID uint) ([]database.Credential, error)
	DeleteCredential(credID uint) error

	// App-LLM Association Management
	GetAppLLMs(appID uint) ([]database.LLM, error)
	UpdateAppLLMs(appID uint, llmIDs []uint) error
}

// TokenServiceInterface defines the interface for token operations
type TokenServiceInterface interface {
	// GenerateToken generates a new API token
	GenerateToken(req *GenerateTokenRequest) (*TokenResponse, error)

	// ListTokens lists tokens for an app
	ListTokens(appID uint) ([]TokenInfo, error)

	// RevokeToken revokes an API token
	RevokeToken(token string) error

	// GetTokenInfo gets information about a token
	GetTokenInfo(token string) (*TokenInfo, error)
}

// CryptoServiceInterface defines the interface for cryptographic operations
type CryptoServiceInterface interface {
	// Encrypt encrypts a plaintext string
	Encrypt(plaintext string) (string, error)

	// Decrypt decrypts a ciphertext string
	Decrypt(ciphertext string) (string, error)

	// HashSecret creates a hash of a secret
	HashSecret(secret string) string

	// VerifySecret verifies a secret against its hash
	VerifySecret(secret, hash string) bool

	// GenerateSecureToken generates a cryptographically secure random token
	GenerateSecureToken(length int) (string, error)

	// GenerateKeyPair generates a new encryption key pair for API keys
	GenerateKeyPair() (keyID, secret string, err error)
}

// Data structures used by service interfaces

// CacheStats represents cache statistics
type CacheStats struct {
	TokenCount      int
	CredentialCount int
	MaxSize         int
	TTL             time.Duration
}

// BudgetStatus represents current budget status
type BudgetStatus struct {
	AppID            uint
	LLMID            *uint
	MonthlyBudget    float64
	CurrentUsage     float64
	RemainingBudget  float64
	TokensUsed       int64
	RequestsCount    int
	PeriodStart      time.Time
	PeriodEnd        time.Time
	IsOverBudget     bool
	PercentageUsed   float64
}

// BudgetUsage represents budget usage data
type BudgetUsage struct {
	ID               uint
	AppID            uint
	LLMID            *uint
	PeriodStart      time.Time
	PeriodEnd        time.Time
	TokensUsed       int64
	RequestsCount    int
	TotalCost        float64
	PromptTokens     int64
	CompletionTokens int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AnalyticsEvent represents an analytics event
type AnalyticsEvent struct {
	ID             uint
	RequestID      string
	AppID          uint
	LLMID          *uint
	CredentialID   *uint
	Endpoint       string
	Method         string
	StatusCode     int
	RequestTokens  int
	ResponseTokens int
	TotalTokens    int
	Cost           float64
	LatencyMs      int
	ErrorMessage   string
	Metadata       map[string]interface{}
	CreatedAt      time.Time
}

// AnalyticsSummary represents analytics summary data
type AnalyticsSummary struct {
	TotalRequests    int64
	SuccessfulRequests int64
	FailedRequests   int64
	TotalTokens      int64
	TotalCost        float64
	AverageLatency   float64
	RequestsPerHour  float64
	TopEndpoints     []EndpointStats
	ErrorStats       []ErrorStats
}

// CostAnalysis represents cost analysis data
type CostAnalysis struct {
	TotalCost        float64
	CostByLLM        map[string]float64
	CostByDay        map[string]float64
	PromptTokensCost float64
	CompletionTokensCost float64
	AverageCostPerRequest float64
}

// EndpointStats represents endpoint usage statistics
type EndpointStats struct {
	Endpoint     string
	RequestCount int64
	ErrorCount   int64
	AverageLatency float64
}

// ErrorStats represents error statistics
type ErrorStats struct {
	StatusCode   int
	ErrorMessage string
	Count        int64
	Percentage   float64
}

// Request/Response structures

// CreateLLMRequest for creating a new LLM
type CreateLLMRequest struct {
	Name           string                 `json:"name" binding:"required"`
	Vendor         string                 `json:"vendor" binding:"required"`
	Endpoint       string                 `json:"endpoint"`
	APIKey         string                 `json:"api_key"`
	DefaultModel   string                 `json:"default_model" binding:"required"`
	MaxTokens      int                    `json:"max_tokens"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
	RetryCount     int                    `json:"retry_count"`
	IsActive       bool                   `json:"is_active"`
	MonthlyBudget  float64                `json:"monthly_budget"`
	RateLimitRPM   int                    `json:"rate_limit_rpm"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// UpdateLLMRequest for updating an LLM
type UpdateLLMRequest struct {
	Name           *string                 `json:"name"`
	Endpoint       *string                 `json:"endpoint"`
	APIKey         *string                 `json:"api_key"`
	DefaultModel   *string                 `json:"default_model"`
	MaxTokens      *int                    `json:"max_tokens"`
	TimeoutSeconds *int                    `json:"timeout_seconds"`
	RetryCount     *int                    `json:"retry_count"`
	IsActive       *bool                   `json:"is_active"`
	MonthlyBudget  *float64                `json:"monthly_budget"`
	RateLimitRPM   *int                    `json:"rate_limit_rpm"`
	Metadata       map[string]interface{}  `json:"metadata"`
}

// CreateAppRequest for creating a new app
type CreateAppRequest struct {
	Name           string   `json:"name" binding:"required"`
	Description    string   `json:"description"`
	OwnerEmail     string   `json:"owner_email" binding:"required,email"`
	MonthlyBudget  float64  `json:"monthly_budget"`
	BudgetResetDay int      `json:"budget_reset_day"`
	RateLimitRPM   int      `json:"rate_limit_rpm"`
	AllowedIPs     []string `json:"allowed_ips"`
	LLMIDs         []uint   `json:"llm_ids"`
}

// UpdateAppRequest for updating an app
type UpdateAppRequest struct {
	Name           *string  `json:"name"`
	Description    *string  `json:"description"`
	OwnerEmail     *string  `json:"owner_email"`
	IsActive       *bool    `json:"is_active"`
	MonthlyBudget  *float64 `json:"monthly_budget"`
	BudgetResetDay *int     `json:"budget_reset_day"`
	RateLimitRPM   *int     `json:"rate_limit_rpm"`
	AllowedIPs     []string `json:"allowed_ips"`
}

// CreateCredentialRequest for creating a new credential
type CreateCredentialRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// GenerateTokenRequest for generating an API token
type GenerateTokenRequest struct {
	AppID     uint          `json:"app_id" binding:"required"`
	Name      string        `json:"name" binding:"required"`
	Scopes    []string      `json:"scopes"`
	ExpiresIn time.Duration `json:"expires_in"`
}

// TokenResponse for token generation response
type TokenResponse struct {
	Token     string     `json:"token"`
	Name      string     `json:"name"`
	AppID     uint       `json:"app_id"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// TokenInfo contains information about an API token
type TokenInfo struct {
	ID        uint       `json:"id"`
	Name      string     `json:"name"`
	AppID     uint       `json:"app_id"`
	Scopes    []string   `json:"scopes"`
	IsActive  bool       `json:"is_active"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
}