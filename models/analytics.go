package models

import (
	"strconv"
	"time"

	"gorm.io/gorm"
)

type InteractionType string

const (
	ChatInteraction  InteractionType = "chat"
	ProxyInteraction InteractionType = "proxy"
)

// LLMChatRecord logs usage for cost and analytics
type LLMChatRecord struct {
	gorm.Model
	ID     uint `gorm:"primaryKey"`
	Name   string
	Vendor string
	// Add LLMID so we can track usage for that specific LLM object:
	LLMID                  uint      `gorm:"index:idx_llm_chat_records_llm_time,priority:1"`
	TotalTimeMS            int
	PromptTokens           int
	ResponseTokens         int
	TotalTokens            int
	TimeStamp              time.Time `gorm:"index:idx_llm_chat_records_time;index:idx_llm_chat_records_app_time,priority:2;index:idx_llm_chat_records_llm_time,priority:2"`
	UserID                 uint      `gorm:"index"`
	Choices                int
	ToolCalls              int
	ChatID                 string
	AppID                  uint    `gorm:"index:idx_llm_chat_records_app_time,priority:1"`
	Cost                   float64
	Currency               string
	InteractionType        InteractionType `gorm:"type:string;default:'chat'"`
	CacheWritePromptTokens int
	CacheReadPromptTokens  int
}

// LLMChatLogEntry for storing extra logs
type LLMChatLogEntry struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	Name      string
	Vendor    string
	TimeStamp time.Time
	Prompt    string
	Response  string
	Tokens    int
	UserID    uint
	ChatID    string
	SessionID string
}

// records tool usage
type ToolCallRecord struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	ToolID    uint
	Name      string
	ExecTime  int
	TimeStamp time.Time
}

// ChartData represents data for charts
type ChartData struct {
	Labels []string  `json:"labels"`
	Data   []float64 `json:"data"`
}

// MultiAxisChartData represents data for a chart with multiple y-axes
type MultiAxisChartData struct {
	Labels   []string  `json:"labels"`
	Datasets []Dataset `json:"datasets"`
}

// Dataset represents a single dataset in a multi-axis chart
type Dataset struct {
	Label string    `json:"label"`
	Data  []float64 `json:"data"`
	Yaxis string    `json:"yAxisID"`
}

// VendorModelCost represents the total cost for a specific vendor and model
type VendorModelCost struct {
	Vendor    string  `json:"vendor"`
	Model     string  `json:"model"`
	TotalCost float64 `json:"totalCost"`
	Currency  string  `json:"currency"`
}

// AppBudgetUsageResponse represents the budget usage for a specific app
type AppBudgetUsageResponse struct {
	CurrentUsage  float64   `json:"current_usage"`
	MonthlyBudget *float64  `json:"monthly_budget"`
	Percentage    *float64  `json:"percentage"`
	StartDate     time.Time `json:"start_date"`
}

// ProxyLogResponse represents a proxy log response in JSON API format
type ProxyLogResponse struct {
	Type       string             `json:"type"`
	ID         string             `json:"id"`
	Attributes ProxyLogAttributes `json:"attributes"`
}

// ProxyLogAttributes is the serialised form of a ProxyLog row. The failover
// marker is carried so a fallback row can be told from a primary one:
// FailoverAttempt is the 1-based rung index (0 = primary) and
// FailoverFromLLMID the primary the request failed over from, absent for a
// primary attempt.
type ProxyLogAttributes struct {
	AppID             uint      `json:"app_id"`
	UserID            uint      `json:"user_id"`
	LLMID             uint      `json:"llm_id"`
	ModelName         string    `json:"model_name"`
	TimeStamp         time.Time `json:"time_stamp"`
	Vendor            string    `json:"vendor"`
	RequestBody       string    `json:"request_body"`
	ResponseBody      string    `json:"response_body"`
	ResponseCode      int       `json:"response_code"`
	FailoverAttempt   int       `json:"failover_attempt"`
	FailoverFromLLMID *uint     `json:"failover_from_llm_id,omitempty"`
	RouterKind        string    `json:"router_kind,omitempty"`
	RouterSlug        string    `json:"router_slug,omitempty"`
	RouterPool        string    `json:"router_pool,omitempty"`
	Route             string    `json:"route,omitempty"`
	RouteReason       string    `json:"route_reason,omitempty"`
	RouteSourceModel  string    `json:"route_source_model,omitempty"`
	RouteTargetModel  string    `json:"route_target_model,omitempty"`
	RouteSelection    string    `json:"route_selection,omitempty"`
}

// NewProxyLogResponse serialises one ProxyLog row for the proxy-log
// endpoints.
func NewProxyLogResponse(log ProxyLog) ProxyLogResponse {
	return ProxyLogResponse{
		Type: "proxy_log",
		ID:   strconv.FormatUint(uint64(log.ID), 10),
		Attributes: ProxyLogAttributes{
			AppID:             log.AppID,
			UserID:            log.UserID,
			LLMID:             log.LLMID,
			ModelName:         log.ModelName,
			TimeStamp:         log.TimeStamp,
			Vendor:            log.Vendor,
			RequestBody:       log.RequestBody,
			ResponseBody:      log.ResponseBody,
			ResponseCode:      log.ResponseCode,
			FailoverAttempt:   log.FailoverAttempt,
			FailoverFromLLMID: log.FailoverFromLLMID,
			RouterKind:        log.RouterKind,
			RouterSlug:        log.RouterSlug,
			RouterPool:        log.RouterPool,
			Route:             log.Route,
			RouteReason:       log.RouteReason,
			RouteSourceModel:  log.RouteSourceModel,
			RouteTargetModel:  log.RouteTargetModel,
			RouteSelection:    log.RouteSelection,
		},
	}
}

// PaginatedProxyLogs represents a paginated list of proxy logs
type PaginatedProxyLogs struct {
	Data []ProxyLogResponse `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Errors []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

type ProxyLog struct {
	gorm.Model
	ID           uint      `gorm:"primaryKey"`
	AppID        uint      `gorm:"index:idx_proxy_logs_app_time,priority:1;index:idx_proxy_logs_app_code_time,priority:1"`
	UserID       uint      `gorm:"index"`
	TimeStamp    time.Time `gorm:"index:idx_proxy_logs_time;index:idx_proxy_logs_app_time,priority:2;index:idx_proxy_logs_app_code_time,priority:3;index:idx_proxy_logs_llm_time,priority:2"`
	// LLMID is the specific LLM vendor entry that handled the request.
	// Required to disambiguate when several LLM entries share a Vendor type
	// (e.g. two Anthropic entries with different API keys).
	LLMID        uint `gorm:"index:idx_proxy_logs_llm_time,priority:1"`
	Vendor       string
	ModelName    string
	RequestBody  string
	ResponseBody string
	ResponseCode int `gorm:"index:idx_proxy_logs_code;index:idx_proxy_logs_app_code_time,priority:2"`
	// FailoverFromLLMID is set when this attempt was a rung of that LLM's
	// failover waterfall; nil for a primary attempt. FailoverAttempt is the
	// 1-based rung index (0 = primary). A request that failed over leaves one
	// row per attempt, so request counts should filter failover_attempt = 0.
	FailoverFromLLMID *uint `gorm:"index:idx_proxy_logs_failover_from"`
	FailoverAttempt   int   `gorm:"default:0"`
	// Router fields are set when the request was addressed to a router: its
	// kind ("model_router"), slug, the pool or route that matched, and why
	// (RouteReason, a small fixed set).
	RouterKind  string `gorm:"size:32"`
	RouterSlug  string `gorm:"index:idx_proxy_logs_router"`
	RouterPool  string
	Route       string
	RouteReason string `gorm:"size:64"`
	// The model the caller asked the router for, the model the chosen LLM
	// was asked for (after any mapping), and how the target was selected
	// ("round_robin", "weighted").
	RouteSourceModel string
	RouteTargetModel string
	RouteSelection   string `gorm:"size:32"`
}
