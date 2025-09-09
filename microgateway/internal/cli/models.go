// internal/cli/models.go
package cli

import (
	"time"
)

// Reuse the request/response structures from the services package
// but redefine them here for CLI-specific usage and help text

// CreateLLMRequest for CLI llm create command
type CreateLLMRequest struct {
	Name           string                 `json:"name" yaml:"name"`
	Vendor         string                 `json:"vendor" yaml:"vendor"`
	Endpoint       string                 `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	APIKey         string                 `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	DefaultModel   string                 `json:"default_model" yaml:"default_model"`
	MaxTokens      int                    `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	TimeoutSeconds int                    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	RetryCount     int                    `json:"retry_count,omitempty" yaml:"retry_count,omitempty"`
	IsActive       bool                   `json:"is_active" yaml:"is_active"`
	MonthlyBudget  float64                `json:"monthly_budget,omitempty" yaml:"monthly_budget,omitempty"`
	RateLimitRPM   int                    `json:"rate_limit_rpm,omitempty" yaml:"rate_limit_rpm,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// UpdateLLMRequest for CLI llm update command
type UpdateLLMRequest struct {
	Name           *string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Endpoint       *string                 `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	APIKey         *string                 `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	DefaultModel   *string                 `json:"default_model,omitempty" yaml:"default_model,omitempty"`
	MaxTokens      *int                    `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	TimeoutSeconds *int                    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	RetryCount     *int                    `json:"retry_count,omitempty" yaml:"retry_count,omitempty"`
	IsActive       *bool                   `json:"is_active,omitempty" yaml:"is_active,omitempty"`
	MonthlyBudget  *float64                `json:"monthly_budget,omitempty" yaml:"monthly_budget,omitempty"`
	RateLimitRPM   *int                    `json:"rate_limit_rpm,omitempty" yaml:"rate_limit_rpm,omitempty"`
	Metadata       map[string]interface{}  `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// CreateAppRequest for CLI app create command
type CreateAppRequest struct {
	Name           string   `json:"name" yaml:"name"`
	Description    string   `json:"description,omitempty" yaml:"description,omitempty"`
	OwnerEmail     string   `json:"owner_email" yaml:"owner_email"`
	MonthlyBudget  float64  `json:"monthly_budget,omitempty" yaml:"monthly_budget,omitempty"`
	BudgetResetDay int      `json:"budget_reset_day,omitempty" yaml:"budget_reset_day,omitempty"`
	RateLimitRPM   int      `json:"rate_limit_rpm,omitempty" yaml:"rate_limit_rpm,omitempty"`
	AllowedIPs     []string `json:"allowed_ips,omitempty" yaml:"allowed_ips,omitempty"`
	LLMIDs         []uint   `json:"llm_ids,omitempty" yaml:"llm_ids,omitempty"`
}

// UpdateAppRequest for CLI app update command
type UpdateAppRequest struct {
	Name           *string  `json:"name,omitempty" yaml:"name,omitempty"`
	Description    *string  `json:"description,omitempty" yaml:"description,omitempty"`
	OwnerEmail     *string  `json:"owner_email,omitempty" yaml:"owner_email,omitempty"`
	IsActive       *bool    `json:"is_active,omitempty" yaml:"is_active,omitempty"`
	MonthlyBudget  *float64 `json:"monthly_budget,omitempty" yaml:"monthly_budget,omitempty"`
	BudgetResetDay *int     `json:"budget_reset_day,omitempty" yaml:"budget_reset_day,omitempty"`
	RateLimitRPM   *int     `json:"rate_limit_rpm,omitempty" yaml:"rate_limit_rpm,omitempty"`
	AllowedIPs     []string `json:"allowed_ips,omitempty" yaml:"allowed_ips,omitempty"`
}

// CreateCredentialRequest for CLI credential create command
type CreateCredentialRequest struct {
	Name      string     `json:"name,omitempty" yaml:"name,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// GenerateTokenRequest for CLI token create command
type GenerateTokenRequest struct {
	AppID     uint          `json:"app_id" yaml:"app_id"`
	Name      string        `json:"name" yaml:"name"`
	Scopes    []string      `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	ExpiresIn time.Duration `json:"expires_in,omitempty" yaml:"expires_in,omitempty"`
}

// UpdateBudgetRequest for CLI budget update command
type UpdateBudgetRequest struct {
	MonthlyBudget  float64 `json:"monthly_budget" yaml:"monthly_budget"`
	BudgetResetDay int     `json:"budget_reset_day,omitempty" yaml:"budget_reset_day,omitempty"`
}

// UpdateAppLLMsRequest for CLI app llms command
type UpdateAppLLMsRequest struct {
	LLMIDs []uint `json:"llm_ids" yaml:"llm_ids"`
}

// LLM represents an LLM configuration (for display)
type LLM struct {
	ID             uint                   `json:"id" yaml:"id"`
	Name           string                 `json:"name" yaml:"name"`
	Slug           string                 `json:"slug" yaml:"slug"`
	Vendor         string                 `json:"vendor" yaml:"vendor"`
	Endpoint       string                 `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	DefaultModel   string                 `json:"default_model" yaml:"default_model"`
	MaxTokens      int                    `json:"max_tokens" yaml:"max_tokens"`
	TimeoutSeconds int                    `json:"timeout_seconds" yaml:"timeout_seconds"`
	RetryCount     int                    `json:"retry_count" yaml:"retry_count"`
	IsActive       bool                   `json:"is_active" yaml:"is_active"`
	MonthlyBudget  float64                `json:"monthly_budget" yaml:"monthly_budget"`
	RateLimitRPM   int                    `json:"rate_limit_rpm" yaml:"rate_limit_rpm"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" yaml:"updated_at"`
}

// App represents an application configuration (for display)
type App struct {
	ID             uint      `json:"id" yaml:"id"`
	Name           string    `json:"name" yaml:"name"`
	Description    string    `json:"description,omitempty" yaml:"description,omitempty"`
	OwnerEmail     string    `json:"owner_email" yaml:"owner_email"`
	IsActive       bool      `json:"is_active" yaml:"is_active"`
	MonthlyBudget  float64   `json:"monthly_budget" yaml:"monthly_budget"`
	BudgetResetDay int       `json:"budget_reset_day" yaml:"budget_reset_day"`
	RateLimitRPM   int       `json:"rate_limit_rpm" yaml:"rate_limit_rpm"`
	AllowedIPs     []string  `json:"allowed_ips,omitempty" yaml:"allowed_ips,omitempty"`
	CreatedAt      time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" yaml:"updated_at"`
}

// Credential represents a credential (for display)
type Credential struct {
	ID         uint       `json:"id" yaml:"id"`
	AppID      uint       `json:"app_id" yaml:"app_id"`
	KeyID      string     `json:"key_id" yaml:"key_id"`
	Name       string     `json:"name,omitempty" yaml:"name,omitempty"`
	IsActive   bool       `json:"is_active" yaml:"is_active"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" yaml:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at" yaml:"created_at"`
}

// Token represents an API token (for display)
type Token struct {
	ID         uint       `json:"id" yaml:"id"`
	Token      string     `json:"token" yaml:"token"`
	Name       string     `json:"name" yaml:"name"`
	AppID      uint       `json:"app_id" yaml:"app_id"`
	Scopes     []string   `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	IsActive   bool       `json:"is_active" yaml:"is_active"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" yaml:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at" yaml:"created_at"`
}

// BudgetStatus represents budget usage information
type BudgetStatus struct {
	AppID           uint      `json:"app_id" yaml:"app_id"`
	LLMID           *uint     `json:"llm_id,omitempty" yaml:"llm_id,omitempty"`
	MonthlyBudget   float64   `json:"monthly_budget" yaml:"monthly_budget"`
	CurrentUsage    float64   `json:"current_usage" yaml:"current_usage"`
	RemainingBudget float64   `json:"remaining_budget" yaml:"remaining_budget"`
	TokensUsed      int64     `json:"tokens_used" yaml:"tokens_used"`
	RequestsCount   int       `json:"requests_count" yaml:"requests_count"`
	PeriodStart     time.Time `json:"period_start" yaml:"period_start"`
	PeriodEnd       time.Time `json:"period_end" yaml:"period_end"`
	IsOverBudget    bool      `json:"is_over_budget" yaml:"is_over_budget"`
	PercentageUsed  float64   `json:"percentage_used" yaml:"percentage_used"`
}

// AnalyticsEvent represents an analytics event
type AnalyticsEvent struct {
	ID             uint      `json:"id" yaml:"id"`
	RequestID      string    `json:"request_id" yaml:"request_id"`
	AppID          uint      `json:"app_id" yaml:"app_id"`
	LLMID          *uint     `json:"llm_id,omitempty" yaml:"llm_id,omitempty"`
	CredentialID   *uint     `json:"credential_id,omitempty" yaml:"credential_id,omitempty"`
	Endpoint       string    `json:"endpoint" yaml:"endpoint"`
	Method         string    `json:"method" yaml:"method"`
	StatusCode     int       `json:"status_code" yaml:"status_code"`
	RequestTokens  int       `json:"request_tokens" yaml:"request_tokens"`
	ResponseTokens int       `json:"response_tokens" yaml:"response_tokens"`
	TotalTokens    int       `json:"total_tokens" yaml:"total_tokens"`
	Cost           float64   `json:"cost" yaml:"cost"`
	LatencyMs      int       `json:"latency_ms" yaml:"latency_ms"`
	ErrorMessage   string    `json:"error_message,omitempty" yaml:"error_message,omitempty"`
	CreatedAt      time.Time `json:"created_at" yaml:"created_at"`
}

// CreateModelPriceRequest for CLI pricing create command (matches AI Gateway interface)
type CreateModelPriceRequest struct {
	Vendor       string  `json:"vendor" yaml:"vendor"`
	ModelName    string  `json:"model_name" yaml:"model_name"`
	CPT          float64 `json:"cpt" yaml:"cpt"`                    // Cost per token (completion/output)
	CPIT         float64 `json:"cpit" yaml:"cpit"`                  // Cost per input token (prompt)  
	CacheWritePT float64 `json:"cache_write_pt" yaml:"cache_write_pt"` // Cost per cache write token
	CacheReadPT  float64 `json:"cache_read_pt" yaml:"cache_read_pt"`   // Cost per cache read token
	Currency     string  `json:"currency,omitempty" yaml:"currency,omitempty"`
}

// UpdateModelPriceRequest for CLI pricing update command
type UpdateModelPriceRequest struct {
	CPT          *float64 `json:"cpt,omitempty" yaml:"cpt,omitempty"`                    // Cost per token (completion/output)
	CPIT         *float64 `json:"cpit,omitempty" yaml:"cpit,omitempty"`                  // Cost per input token (prompt)
	CacheWritePT *float64 `json:"cache_write_pt,omitempty" yaml:"cache_write_pt,omitempty"` // Cost per cache write token
	CacheReadPT  *float64 `json:"cache_read_pt,omitempty" yaml:"cache_read_pt,omitempty"`   // Cost per cache read token
	Currency     *string  `json:"currency,omitempty" yaml:"currency,omitempty"`
}

// ModelPrice represents a model price configuration (for display)
type ModelPrice struct {
	ID           uint      `json:"id" yaml:"id"`
	Vendor       string    `json:"vendor" yaml:"vendor"`
	ModelName    string    `json:"model_name" yaml:"model_name"`
	CPT          float64   `json:"cpt" yaml:"cpt"`                    // Cost per token (completion/output)
	CPIT         float64   `json:"cpit" yaml:"cpit"`                  // Cost per input token (prompt)  
	CacheWritePT float64   `json:"cache_write_pt" yaml:"cache_write_pt"` // Cost per cache write token
	CacheReadPT  float64   `json:"cache_read_pt" yaml:"cache_read_pt"`   // Cost per cache read token
	Currency     string    `json:"currency" yaml:"currency"`
	CreatedAt    time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" yaml:"updated_at"`
}