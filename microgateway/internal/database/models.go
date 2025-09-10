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
	
	// Hub-and-Spoke Configuration
	Namespace  string         `gorm:"default:'';index:idx_token_namespace"` // Empty = global, specific = filtered to edge
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
	Name            string         `gorm:"not null" json:"name"`
	Slug            string         `gorm:"uniqueIndex;not null" json:"slug"`
	Vendor          string         `gorm:"not null" json:"vendor"`
	Endpoint        string         `json:"endpoint"`
	APIKeyEncrypted string         `json:"api_key_encrypted"`
	DefaultModel    string         `json:"default_model"`
	MaxTokens       int            `gorm:"default:4096" json:"max_tokens"`
	TimeoutSeconds  int            `gorm:"default:30" json:"timeout_seconds"`
	RetryCount      int            `gorm:"default:3" json:"retry_count"`
	IsActive        bool           `gorm:"default:true;index:idx_llm_active" json:"is_active"`
	MonthlyBudget   float64        `json:"monthly_budget"`
	RateLimitRPM    int            `json:"rate_limit_rpm"`
	Metadata        datatypes.JSON `gorm:"type:json" json:"metadata"`
	AllowedModels   datatypes.JSON `gorm:"type:json" json:"allowed_models"` // JSON array of regex patterns for allowed models
	
	// Authentication configuration for pluggable auth mechanisms
	AuthMechanism   string         `gorm:"default:'token'" json:"auth_mechanism"` // "token", "oauth", "api-key", "custom"
	AuthConfig      datatypes.JSON `gorm:"type:json" json:"auth_config"`          // Provider-specific configuration
	
	// Hub-and-Spoke Configuration
	Namespace       string         `gorm:"default:'';index:idx_llm_namespace" json:"namespace"` // Empty = global, specific = filtered to edge

	// Relationships
	Apps    []App           `gorm:"many2many:app_llms;"`
	Filters []Filter        `gorm:"many2many:llm_filters;"`
	Plugins []Plugin        `gorm:"many2many:llm_plugins;"`
	Usage   []BudgetUsage   `gorm:"foreignKey:LLMID"`
	Events  []AnalyticsEvent `gorm:"foreignKey:LLMID"`
}

// App represents an application
type App struct {
	gorm.Model
	Name            string         `gorm:"not null" json:"name"`
	Description     string         `json:"description"`
	OwnerEmail      string         `gorm:"index" json:"owner_email"`
	IsActive        bool           `gorm:"default:true;index" json:"is_active"`
	MonthlyBudget   float64        `json:"monthly_budget"`
	BudgetStartDate *time.Time     `json:"budget_start_date"`
	BudgetResetDay  int            `gorm:"default:1" json:"budget_reset_day"`
	RateLimitRPM    int            `json:"rate_limit_rpm"`
	AllowedIPs      datatypes.JSON `gorm:"type:json" json:"allowed_ips"`
	Metadata        datatypes.JSON `gorm:"type:json" json:"metadata"`
	
	// Hub-and-Spoke Configuration
	Namespace       string         `gorm:"default:'';index:idx_app_namespace" json:"namespace"` // Empty = global, specific = filtered to edge

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

// ModelPrice represents LLM model pricing information (full AI Gateway interface)
type ModelPrice struct {
	gorm.Model
	Vendor       string  `gorm:"not null"`
	ModelName    string  `gorm:"not null"`
	CPT          float64 `gorm:"not null"`     // Cost per token (completion/output)
	CPIT         float64 `gorm:"not null"`     // Cost per input token (prompt)  
	CacheWritePT float64 `gorm:"default:0"`   // Cost per cache write token
	CacheReadPT  float64 `gorm:"default:0"`   // Cost per cache read token
	Currency     string  `gorm:"default:USD"`
	
	// Hub-and-Spoke Configuration
	Namespace    string  `gorm:"default:'';index:idx_model_price_namespace"` // Empty = global, specific = filtered to edge
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
	
	// Detailed payload storage (configurable)
	RequestBody    string         `gorm:"type:text"` // Store request payload
	ResponseBody   string         `gorm:"type:text"` // Store response payload
	
	CreatedAt      time.Time      `gorm:"index:idx_analytics_app"`
}

// Filter represents a filter script
type Filter struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name        string    `gorm:"not null" json:"name"`
	Description string    `json:"description"`
	Script      string    `gorm:"not null;type:text" json:"script"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	OrderIndex  int       `gorm:"default:0" json:"order_index"`
	
	// Hub-and-Spoke Configuration
	Namespace   string    `gorm:"default:'';index:idx_filter_namespace" json:"namespace"` // Empty = global, specific = filtered to edge

	// Relationships
	LLMs []LLM `gorm:"many2many:llm_filters;" json:"llms,omitempty"`
}

// LLMFilter represents the many-to-many relationship between LLMs and filters
type LLMFilter struct {
	LLMID      uint      `gorm:"primaryKey" json:"llm_id"`
	FilterID   uint      `gorm:"primaryKey" json:"filter_id"`
	IsActive   bool      `gorm:"default:true" json:"is_active"`
	OrderIndex int       `gorm:"default:0" json:"order_index"`
	CreatedAt  time.Time `json:"created_at"`
}

// Plugin represents a plugin configuration
type Plugin struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"not null" json:"name"`
	Slug        string         `gorm:"uniqueIndex;not null" json:"slug"`
	Description string         `json:"description"`
	Command     string         `gorm:"not null;size:500" json:"command"`
	Checksum    string         `gorm:"size:255" json:"checksum"`
	Config      datatypes.JSON `gorm:"type:json" json:"config"`
	HookType    string         `gorm:"not null;size:50;index:idx_plugins_hook_type" json:"hook_type"`
	IsActive    bool           `gorm:"index:idx_plugins_is_active" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	
	// Hub-and-Spoke Configuration
	Namespace   string         `gorm:"default:'';index:idx_plugin_namespace" json:"namespace"` // Empty = global, specific = filtered to edge

	// Relationships
	LLMs []LLM `gorm:"many2many:llm_plugins;" json:"llms,omitempty"`
}

// LLMPlugin represents the many-to-many relationship between LLMs and plugins
type LLMPlugin struct {
	LLMID          uint           `gorm:"primaryKey;index:idx_llm_plugins_llm_id" json:"llm_id"`
	PluginID       uint           `gorm:"primaryKey" json:"plugin_id"`
	OrderIndex     int            `gorm:"default:0;index:idx_llm_plugins_order" json:"order_index"`
	IsActive       bool           `gorm:"default:true" json:"is_active"`
	ConfigOverride datatypes.JSON `gorm:"type:json" json:"config_override"`
	CreatedAt      time.Time      `json:"created_at"`
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
func (Plugin) TableName() string       { return "plugins" }
func (LLMPlugin) TableName() string    { return "llm_plugins" }

// EdgeInstance represents a registered edge instance (for control mode)
type EdgeInstance struct {
	ID            uint           `gorm:"primaryKey"`
	EdgeID        string         `gorm:"uniqueIndex;not null"`
	Namespace     string         `gorm:"index:idx_edge_namespace;not null;default:''"`
	Version       string         `gorm:"size:100"`
	BuildHash     string         `gorm:"size:64"`
	Metadata      datatypes.JSON `gorm:"type:json"`
	LastHeartbeat *time.Time     `gorm:"index:idx_edge_heartbeat"`
	Status        string         `gorm:"size:50;default:'registered';index:idx_edge_namespace"`
	SessionID     string         `gorm:"size:255"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ConfigurationChange represents a configuration change for propagation
type ConfigurationChange struct {
	ID                 uint           `gorm:"primaryKey"`
	ChangeType         string         `gorm:"size:20;not null;index:idx_config_changes_type"` // CREATE, UPDATE, DELETE
	EntityType         string         `gorm:"size:50;not null;index:idx_config_changes_type"` // LLM, APP, TOKEN, etc.
	EntityID           uint           `gorm:"not null;index:idx_config_changes_type"`
	EntityData         datatypes.JSON `gorm:"type:json"` // Complete serialized entity data
	Namespace          string         `gorm:"not null;default:'';index:idx_config_changes_namespace"`
	PropagatedToEdges  datatypes.JSON `gorm:"type:json"` // Array of edge_ids that received this change
	Processed          bool           `gorm:"default:false;index:idx_config_changes_processed"`
	CreatedAt          time.Time      `gorm:"index:idx_config_changes_namespace,idx_config_changes_processed"`
}

// TableName methods for new models
func (EdgeInstance) TableName() string       { return "edge_instances" }
func (ConfigurationChange) TableName() string { return "configuration_changes" }