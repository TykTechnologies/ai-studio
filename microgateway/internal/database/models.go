// internal/database/models.go
package database

import (
	"encoding/json"
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
	UserID          uint           `json:"user_id"` // Owner user ID (synced from control plane for analytics)
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
	ModelName    string  `gorm:"not null;uniqueIndex:idx_model_vendor"` // Model name with composite unique index
	Vendor       string  `gorm:"not null;uniqueIndex:idx_model_vendor"` // Vendor with composite unique index
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
// Aligned with LLMChatRecord from main AI Studio for analytics parity
type AnalyticsEvent struct {
	ID             uint           `gorm:"primaryKey"`
	RequestID      string         `gorm:"uniqueIndex;not null"`
	AppID          uint           `gorm:"not null;index:idx_analytics_app"`
	App            *App           `gorm:"foreignKey:AppID"`
	LLMID          *uint
	LLM            *LLM           `gorm:"foreignKey:LLMID"`
	CredentialID   *uint
	Credential     *Credential    `gorm:"foreignKey:CredentialID"`

	// Fields matching LLMChatRecord for parity
	UserID              uint   `gorm:"index:idx_analytics_user"`      // User who made the request
	Name                string                                        // Model name (e.g., "gpt-4", "claude-3-opus")
	Vendor              string                                        // LLM vendor (e.g., "openai", "anthropic")
	InteractionType     string `gorm:"type:string;default:'proxy'"`  // "chat" or "proxy"
	Choices             int                                           // Number of choices in response
	ToolCalls           int                                           // Number of tool calls made
	ChatID              string                                        // Chat session identifier
	Currency            string `gorm:"default:USD"`                  // Currency for cost

	// Request/Response details
	Endpoint       string
	Method         string
	StatusCode     int

	// Token tracking (matching LLMChatRecord naming)
	PromptTokens           int     // Input tokens (matches LLMChatRecord)
	ResponseTokens         int     // Output tokens (matches LLMChatRecord)
	TotalTokens            int
	CacheWritePromptTokens int     // Cache creation/write tokens (e.g., Anthropic prompt caching)
	CacheReadPromptTokens  int     // Cache read tokens (e.g., Anthropic prompt caching)

	// Cost and timing
	Cost                   float64
	TotalTimeMS            int     // Request latency in milliseconds (matches LLMChatRecord)

	// Error tracking
	ErrorMessage           string
	Metadata               datatypes.JSON `gorm:"type:json"`

	// Detailed payload storage (configurable)
	RequestBody    string         `gorm:"type:text"` // Store request payload
	ResponseBody   string         `gorm:"type:text"` // Store response payload

	// Timestamps
	TimeStamp      time.Time      `gorm:"index:idx_analytics_timestamp"` // Match LLMChatRecord field name
	CreatedAt      time.Time      `gorm:"index:idx_analytics_app"`
}

// Filter represents a filter script
type Filter struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name           string    `gorm:"not null" json:"name"`
	Description    string    `json:"description"`
	Script         string    `gorm:"not null;type:text" json:"script"`
	ResponseFilter bool      `gorm:"default:false" json:"response_filter"` // true = response filter, false = request filter
	IsActive       bool      `gorm:"default:true" json:"is_active"`
	OrderIndex     int       `gorm:"default:0" json:"order_index"`

	// Hub-and-Spoke Configuration
	Namespace      string    `gorm:"default:'';index:idx_filter_namespace" json:"namespace"` // Empty = global, specific = filtered to edge

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
	ID                  uint           `gorm:"primaryKey" json:"id"`
	Name                string         `gorm:"not null" json:"name"`
	Description         string         `json:"description"`
	Command             string         `gorm:"not null;size:500" json:"command"`
	Checksum            string         `gorm:"size:255" json:"checksum"`
	Config              datatypes.JSON `gorm:"type:json" json:"config"`
	HookType            string         `gorm:"not null;size:50;index:idx_plugins_hook_type" json:"hook_type"`
	HookTypes           datatypes.JSON `gorm:"type:json" json:"hook_types"`                         // All hook types this plugin supports
	HookTypesCustomized bool           `gorm:"default:false" json:"hook_types_customized"`          // True if user overrode manifest hooks
	IsActive            bool           `gorm:"index:idx_plugins_is_active" json:"is_active"`
	ServiceScopes       datatypes.JSON `gorm:"type:json" json:"service_scopes"`                     // Service API scopes (e.g., ["llms.read", "apps.read"])
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Hub-and-Spoke Configuration
	Namespace string `gorm:"default:'';index:idx_plugin_namespace" json:"namespace"` // Empty = global, specific = filtered to edge

	// Relationships
	LLMs []LLM `gorm:"many2many:llm_plugins;" json:"llms,omitempty"`
}

// HasServiceAccess checks if the plugin has been authorized for service API access
func (p *Plugin) HasServiceAccess() bool {
	// Plugin has service access if it has any service scopes defined
	if p.ServiceScopes == nil || len(p.ServiceScopes) == 0 {
		return false
	}

	var scopes []string
	if err := json.Unmarshal(p.ServiceScopes, &scopes); err != nil {
		return false
	}

	return len(scopes) > 0
}

// HasServiceScope checks if the plugin has a specific service scope
func (p *Plugin) HasServiceScope(requiredScope string) bool {
	if p.ServiceScopes == nil || len(p.ServiceScopes) == 0 {
		return false
	}

	var scopes []string
	if err := json.Unmarshal(p.ServiceScopes, &scopes); err != nil {
		return false
	}

	for _, scope := range scopes {
		if scope == requiredScope {
			return true
		}
	}

	return false
}

// GetServiceScopes returns the list of service scopes for this plugin
func (p *Plugin) GetServiceScopes() []string {
	if p.ServiceScopes == nil || len(p.ServiceScopes) == 0 {
		return []string{}
	}

	var scopes []string
	if err := json.Unmarshal(p.ServiceScopes, &scopes); err != nil {
		return []string{}
	}

	return scopes
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

// PluginKV represents plugin key-value storage
type PluginKV struct {
	ID        uint       `gorm:"primaryKey"`
	Key       string     `gorm:"uniqueIndex;not null"`
	Value     []byte     `gorm:"type:bytea;not null"`
	PluginID  uint       `gorm:"not null;index"`
	Plugin    *Plugin    `gorm:"foreignKey:PluginID"`
	ExpireAt  *time.Time `gorm:"index:idx_plugin_kv_expire_at"` // Optional expiration timestamp
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsExpired checks if the plugin KV entry has expired
func (pkv *PluginKV) IsExpired() bool {
	if pkv.ExpireAt == nil {
		return false // No expiration set
	}
	return pkv.ExpireAt.Before(time.Now())
}

// ControlPayload represents a queued payload to be sent to the control plane
// This enables plugins on edge instances to send data back to their control-plane counterpart
type ControlPayload struct {
	ID            uint           `gorm:"primaryKey"`
	PluginID      uint           `gorm:"not null;index:idx_control_payload_plugin"`
	Payload       []byte         `gorm:"type:blob;not null"`
	CorrelationID string         `gorm:"size:255;index:idx_control_payload_correlation"`
	Metadata      datatypes.JSON `gorm:"type:json"`
	Sent          bool           `gorm:"default:false;index:idx_control_payload_sent"`
	SentAt        *time.Time
	CreatedAt     time.Time      `gorm:"index:idx_control_payload_created"`
}

// TableName methods for new models
func (EdgeInstance) TableName() string    { return "edge_instances" }
func (PluginKV) TableName() string        { return "plugin_kv" }
func (ControlPayload) TableName() string  { return "control_payloads" }