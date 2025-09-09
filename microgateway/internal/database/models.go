// internal/database/models.go
package database

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// APIToken represents an API token for gateway access
type APIToken struct {
	ID         uint           `gorm:"primaryKey"`
	Token      string         `gorm:"uniqueIndex;not null"`
	Name       string         `gorm:"not null"`
	AppID      uint           `gorm:"not null"`
	App        *App           `gorm:"foreignKey:AppID"`
	Scopes     datatypes.JSON `gorm:"type:json"`
	IsActive   bool           `gorm:"default:true;index:idx_token_active"`
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// TokenCache for persistent cache backing
type TokenCache struct {
	Token     string         `gorm:"primaryKey"`
	CacheData datatypes.JSON `gorm:"not null;type:json"`
	ExpiresAt time.Time      `gorm:"not null;index"`
	CreatedAt time.Time
}

// LLM represents an LLM configuration
type LLM struct {
	gorm.Model
	Name            string         `gorm:"not null"`
	Slug            string         `gorm:"uniqueIndex;not null"`
	Vendor          string         `gorm:"not null"`
	Endpoint        string
	APIKeyEncrypted string
	DefaultModel    string
	MaxTokens       int    `gorm:"default:4096"`
	TimeoutSeconds  int    `gorm:"default:30"`
	RetryCount      int    `gorm:"default:3"`
	IsActive        bool   `gorm:"default:true;index:idx_llm_active"`
	MonthlyBudget   float64
	RateLimitRPM    int
	Metadata        datatypes.JSON `gorm:"type:json"`
	
	// Authentication configuration for pluggable auth mechanisms
	AuthMechanism   string         `gorm:"default:'token'"` // "token", "oauth", "api-key", "custom"
	AuthConfig      datatypes.JSON `gorm:"type:json"`       // Provider-specific configuration

	// Relationships
	Apps    []App           `gorm:"many2many:app_llms;"`
	Filters []Filter        `gorm:"many2many:llm_filters;"`
	Usage   []BudgetUsage   `gorm:"foreignKey:LLMID"`
	Events  []AnalyticsEvent `gorm:"foreignKey:LLMID"`
}

// App represents an application
type App struct {
	gorm.Model
	Name            string         `gorm:"not null"`
	Description     string
	OwnerEmail      string         `gorm:"index"`
	IsActive        bool           `gorm:"default:true;index"`
	MonthlyBudget   float64
	BudgetStartDate *time.Time
	BudgetResetDay  int            `gorm:"default:1"`
	RateLimitRPM    int
	AllowedIPs      datatypes.JSON `gorm:"type:json"`
	Metadata        datatypes.JSON `gorm:"type:json"`

	// Relationships
	Credentials []Credential     `gorm:"foreignKey:AppID"`
	Tokens      []APIToken       `gorm:"foreignKey:AppID"`
	LLMs        []LLM            `gorm:"many2many:app_llms;"`
	BudgetUsage []BudgetUsage    `gorm:"foreignKey:AppID"`
	Events      []AnalyticsEvent `gorm:"foreignKey:AppID"`
}

// Credential represents app credentials
type Credential struct {
	gorm.Model
	AppID      uint       `gorm:"not null"`
	App        *App       `gorm:"foreignKey:AppID"`
	KeyID      string     `gorm:"uniqueIndex;not null"`
	SecretHash string     `gorm:"not null"`
	Name       string
	IsActive   bool       `gorm:"default:true"`
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
}

// AppLLM represents the many-to-many relationship between apps and LLMs
type AppLLM struct {
	AppID        uint      `gorm:"primaryKey"`
	LLMID        uint      `gorm:"primaryKey"`
	IsActive     bool      `gorm:"default:true"`
	CustomBudget float64
	CreatedAt    time.Time
}

// ModelPrice represents LLM model pricing information
type ModelPrice struct {
	gorm.Model
	Vendor          string    `gorm:"not null"`
	ModelName       string    `gorm:"not null"`
	PromptPrice     float64   `gorm:"not null"`
	CompletionPrice float64   `gorm:"not null"`
	Currency        string    `gorm:"default:USD"`
	PerTokens       int       `gorm:"default:1000"`
	EffectiveDate   time.Time `gorm:"not null"`
}

// BudgetUsage tracks budget consumption
type BudgetUsage struct {
	ID               uint      `gorm:"primaryKey"`
	AppID            uint      `gorm:"not null;uniqueIndex:idx_budget_period"`
	App              *App      `gorm:"foreignKey:AppID"`
	LLMID            *uint     `gorm:"uniqueIndex:idx_budget_period"`
	LLM              *LLM      `gorm:"foreignKey:LLMID"`
	PeriodStart      time.Time `gorm:"not null;uniqueIndex:idx_budget_period"`
	PeriodEnd        time.Time `gorm:"not null;uniqueIndex:idx_budget_period"`
	TokensUsed       int64     `gorm:"default:0"`
	RequestsCount    int       `gorm:"default:0"`
	TotalCost        float64   `gorm:"default:0"`
	PromptTokens     int64     `gorm:"default:0"`
	CompletionTokens int64     `gorm:"default:0"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AnalyticsEvent for request/response tracking
type AnalyticsEvent struct {
	ID             uint           `gorm:"primaryKey"`
	RequestID      string         `gorm:"uniqueIndex;not null"`
	AppID          uint           `gorm:"not null;index:idx_analytics_app"`
	App            *App           `gorm:"foreignKey:AppID"`
	LLMID          *uint
	LLM            *LLM           `gorm:"foreignKey:LLMID"`
	CredentialID   *uint
	Credential     *Credential    `gorm:"foreignKey:CredentialID"`
	Endpoint       string
	Method         string
	StatusCode     int
	RequestTokens  int
	ResponseTokens int
	TotalTokens    int
	Cost           float64
	LatencyMs      int
	ErrorMessage   string
	Metadata       datatypes.JSON `gorm:"type:json"`
	CreatedAt      time.Time      `gorm:"index:idx_analytics_app"`
}

// Filter represents a filter script
type Filter struct {
	gorm.Model
	Name       string `gorm:"not null"`
	Type       string `gorm:"not null"` // 'request', 'response', 'both'
	Script     string `gorm:"not null;type:text"`
	IsActive   bool   `gorm:"default:true"`
	OrderIndex int    `gorm:"default:0"`

	// Relationships
	LLMs []LLM `gorm:"many2many:llm_filters;"`
}

// LLMFilter represents the many-to-many relationship between LLMs and filters
type LLMFilter struct {
	LLMID      uint      `gorm:"primaryKey"`
	FilterID   uint      `gorm:"primaryKey"`
	IsActive   bool      `gorm:"default:true"`
	OrderIndex int       `gorm:"default:0"`
	CreatedAt  time.Time
}

// TableName methods to ensure consistent table naming
func (APIToken) TableName() string     { return "api_tokens" }
func (TokenCache) TableName() string   { return "token_cache" }
func (LLM) TableName() string          { return "llms" }
func (App) TableName() string          { return "apps" }
func (Credential) TableName() string   { return "credentials" }
func (AppLLM) TableName() string       { return "app_llms" }
func (ModelPrice) TableName() string   { return "model_prices" }
func (BudgetUsage) TableName() string  { return "budget_usage" }
func (AnalyticsEvent) TableName() string { return "analytics_events" }
func (Filter) TableName() string       { return "filters" }
func (LLMFilter) TableName() string    { return "llm_filters" }