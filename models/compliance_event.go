package models

import (
	"time"

	"gorm.io/gorm"
)

// ComplianceEvent records a compliance-relevant event reported by a filter script.
// Scripts can flag any type of compliance event (redactions, rewrites, PII detection,
// silent failures, etc.) by setting compliance_events in their output object.
type ComplianceEvent struct {
	gorm.Model
	AppID       uint      `json:"app_id" gorm:"index:idx_ce_app_time,priority:1"`
	UserID      uint      `json:"user_id" gorm:"index"`
	LLMID       uint      `json:"llm_id" gorm:"index"`
	FilterName  string    `json:"filter_name" gorm:"index"`
	FilterScope string    `json:"filter_scope"` // "proxy_request", "proxy_response", "chat_request", "chat_response", "file_reference", "tool_response"
	EventType   string    `json:"event_type" gorm:"index"`   // Free-form: "pii_redacted", "content_rewritten", "silent_failure", etc.
	Severity    string    `json:"severity" gorm:"index"`     // "info", "warning", "critical"
	Description string    `json:"description"`
	Metadata    string    `json:"metadata"`                  // JSON blob for arbitrary key-value data
	Vendor      string    `json:"vendor"`
	ModelName   string    `json:"model_name"`
	TimeStamp   time.Time `json:"timestamp" gorm:"index:idx_ce_time;index:idx_ce_app_time,priority:2"`
}
