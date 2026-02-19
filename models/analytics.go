package models

import (
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

// ProxyLogResponse represents a proxy log response in JSON API format
type ProxyLogResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		AppID        uint      `json:"app_id"`
		UserID       uint      `json:"user_id"`
		TimeStamp    time.Time `json:"time_stamp"`
		Vendor       string    `json:"vendor"`
		RequestBody  string    `json:"request_body"`
		ResponseBody string    `json:"response_body"`
		ResponseCode int       `json:"response_code"`
	} `json:"attributes"`
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
	TimeStamp    time.Time `gorm:"index:idx_proxy_logs_time;index:idx_proxy_logs_app_time,priority:2;index:idx_proxy_logs_app_code_time,priority:3"`
	Vendor       string
	RequestBody  string
	ResponseBody string
	ResponseCode int `gorm:"index:idx_proxy_logs_code;index:idx_proxy_logs_app_code_time,priority:2"`
}
