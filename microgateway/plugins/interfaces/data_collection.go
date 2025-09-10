// plugins/interfaces/data_collection.go
package interfaces

import (
	"context"
	"time"
)

// Data collection plugins use HookTypeDataCollection from base.go

// DataCollectionPlugin handles storage/processing of collected data
// This plugin type allows users to intercept and redirect data that would
// normally be stored in the database to external systems like Elasticsearch,
// ClickHouse, data lakes, or custom analytics platforms.
type DataCollectionPlugin interface {
	BasePlugin
	
	// HandleProxyLog processes proxy request/response logs
	// This is called for every LLM request/response that goes through the gateway
	HandleProxyLog(ctx context.Context, req *ProxyLogData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
	
	// HandleAnalytics processes token usage and cost data
	// This is called when recording LLM usage for billing and analytics
	HandleAnalytics(ctx context.Context, req *AnalyticsData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
	
	// HandleBudgetUsage processes budget usage tracking data
	// This is called when updating budget usage for apps and LLMs
	HandleBudgetUsage(ctx context.Context, req *BudgetUsageData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
}

// ProxyLogData contains the request/response data for LLM proxy calls
type ProxyLogData struct {
	AppID        uint      `json:"app_id"`
	UserID       uint      `json:"user_id"`
	Vendor       string    `json:"vendor"`        // e.g., "openai", "anthropic"
	RequestBody  []byte    `json:"request_body"`  // Full request JSON
	ResponseBody []byte    `json:"response_body"` // Full response JSON
	ResponseCode int       `json:"response_code"` // HTTP status code
	Timestamp    time.Time `json:"timestamp"`
	RequestID    string    `json:"request_id"`    // Unique request identifier
}

// AnalyticsData contains token usage and cost information
type AnalyticsData struct {
	LLMID                   uint      `json:"llm_id"`
	ModelName              string    `json:"model_name"`              // e.g., "gpt-4", "claude-3-sonnet"
	Vendor                 string    `json:"vendor"`                 // e.g., "openai", "anthropic"
	PromptTokens           int       `json:"prompt_tokens"`
	ResponseTokens         int       `json:"response_tokens"`
	CacheWritePromptTokens int       `json:"cache_write_prompt_tokens"` // Anthropic cache writes
	CacheReadPromptTokens  int       `json:"cache_read_prompt_tokens"`  // Anthropic cache reads
	TotalTokens            int       `json:"total_tokens"`
	Cost                   float64   `json:"cost"`                   // Cost in USD
	Currency               string    `json:"currency"`               // Usually "USD"
	AppID                  uint      `json:"app_id"`
	UserID                 uint      `json:"user_id"`
	Timestamp              time.Time `json:"timestamp"`
	ToolCalls              int       `json:"tool_calls"`             // Number of tool calls in request
	Choices                int       `json:"choices"`                // Number of response choices
	RequestID              string    `json:"request_id"`             // Unique request identifier
}

// BudgetUsageData contains budget tracking information
type BudgetUsageData struct {
	AppID            uint      `json:"app_id"`
	LLMID            uint      `json:"llm_id"`
	TokensUsed       int64     `json:"tokens_used"`        // Total tokens consumed
	Cost             float64   `json:"cost"`               // Cost in USD
	RequestsCount    int       `json:"requests_count"`     // Number of requests
	PromptTokens     int64     `json:"prompt_tokens"`      // Input tokens
	CompletionTokens int64     `json:"completion_tokens"`  // Output tokens  
	PeriodStart      time.Time `json:"period_start"`       // Budget period start
	PeriodEnd        time.Time `json:"period_end"`         // Budget period end
	Timestamp        time.Time `json:"timestamp"`          // When this usage occurred
	RequestID        string    `json:"request_id"`         // Unique request identifier
}

// DataCollectionResponse indicates how the plugin handled the data
type DataCollectionResponse struct {
	// Success indicates if the plugin processed the data successfully
	Success bool `json:"success"`
	
	// Handled indicates if the plugin processed the data
	// If true and ReplaceDatabase is configured, database storage may be skipped
	Handled bool `json:"handled"`
	
	// ErrorMessage provides details if Success is false
	ErrorMessage string `json:"error_message"`
	
	// Metadata can contain plugin-specific response data
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DataCollectionHookType represents the type of data collection hook
type DataCollectionHookType string

const (
	// ProxyLogHook for proxy request/response data
	ProxyLogHook DataCollectionHookType = "proxy_log"
	
	// AnalyticsHook for token usage and cost data
	AnalyticsHook DataCollectionHookType = "analytics"
	
	// BudgetHook for budget usage tracking data
	BudgetHook DataCollectionHookType = "budget"
)

// IsValidDataCollectionHookType checks if the hook type is valid
func IsValidDataCollectionHookType(hookType string) bool {
	switch DataCollectionHookType(hookType) {
	case ProxyLogHook, AnalyticsHook, BudgetHook:
		return true
	default:
		return false
	}
}