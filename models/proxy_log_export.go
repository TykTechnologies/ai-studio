package models

import (
	"time"

	"gorm.io/gorm"
)

// ExportStatus represents the current state of an export job
type ExportStatus string

const (
	ExportStatusPending    ExportStatus = "pending"
	ExportStatusProcessing ExportStatus = "processing"
	ExportStatusCompleted  ExportStatus = "completed"
	ExportStatusFailed     ExportStatus = "failed"
	ExportStatusExpired    ExportStatus = "expired"
)

// ExportSourceType indicates whether the export is for an App, LLM, or User chat history
type ExportSourceType string

const (
	ExportSourceApp  ExportSourceType = "app"
	ExportSourceLLM  ExportSourceType = "llm"
	ExportSourceUser ExportSourceType = "user"
)

// ProxyLogExport tracks export job status and file references for proxy log exports.
// This is an Enterprise Edition feature.
type ProxyLogExport struct {
	gorm.Model

	// Export identification - UUID used as download token
	ExportID string `gorm:"uniqueIndex;not null" json:"export_id"`

	// Source information
	SourceType ExportSourceType `gorm:"not null" json:"source_type"` // "app" or "llm"
	SourceID   uint             `gorm:"not null" json:"source_id"`   // AppID or LLMID

	// Filter criteria (stored for reference)
	StartDate    time.Time `gorm:"not null" json:"start_date"`
	EndDate      time.Time `gorm:"not null" json:"end_date"`
	SearchFilter string    `json:"search_filter,omitempty"` // Optional search term

	// Job status
	Status       ExportStatus `gorm:"default:'pending'" json:"status"`
	TotalRecords int64        `json:"total_records"` // Total count after query

	// File information
	FilePath string `json:"file_path,omitempty"` // Path to generated JSON file
	FileSize int64  `json:"file_size"`           // Size in bytes

	// Timing
	RequestedAt time.Time  `gorm:"not null" json:"requested_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt   time.Time  `gorm:"not null;index" json:"expires_at"` // 24 hours after completion

	// User tracking
	RequestedBy uint `gorm:"not null" json:"requested_by"` // Admin user ID

	// Error handling
	ErrorMessage string `json:"error_message,omitempty"`
}

// TableName returns the table name for the ProxyLogExport model
func (ProxyLogExport) TableName() string {
	return "proxy_log_exports"
}

// ProxyLogExportResponse is the JSON:API response format for a proxy log export
type ProxyLogExportResponse struct {
	Type       string                       `json:"type"`
	ID         string                       `json:"id"`
	Attributes ProxyLogExportAttributes     `json:"attributes"`
}

// ProxyLogExportAttributes contains the attributes for the JSON:API response
type ProxyLogExportAttributes struct {
	ExportID     string           `json:"export_id"`
	SourceType   ExportSourceType `json:"source_type"`
	SourceID     uint             `json:"source_id"`
	StartDate    time.Time        `json:"start_date"`
	EndDate      time.Time        `json:"end_date"`
	SearchFilter string           `json:"search_filter,omitempty"`
	Status       ExportStatus     `json:"status"`
	TotalRecords int64            `json:"total_records"`
	FileSize     int64            `json:"file_size"`
	RequestedAt  time.Time        `json:"requested_at"`
	CompletedAt  *time.Time       `json:"completed_at,omitempty"`
	ExpiresAt    time.Time        `json:"expires_at"`
	RequestedBy  uint             `json:"requested_by"`
	ErrorMessage string           `json:"error_message,omitempty"`
}

// ToResponse converts a ProxyLogExport to its JSON:API response format
func (p *ProxyLogExport) ToResponse() ProxyLogExportResponse {
	return ProxyLogExportResponse{
		Type: "proxy_log_export",
		ID:   p.ExportID,
		Attributes: ProxyLogExportAttributes{
			ExportID:     p.ExportID,
			SourceType:   p.SourceType,
			SourceID:     p.SourceID,
			StartDate:    p.StartDate,
			EndDate:      p.EndDate,
			SearchFilter: p.SearchFilter,
			Status:       p.Status,
			TotalRecords: p.TotalRecords,
			FileSize:     p.FileSize,
			RequestedAt:  p.RequestedAt,
			CompletedAt:  p.CompletedAt,
			ExpiresAt:    p.ExpiresAt,
			RequestedBy:  p.RequestedBy,
			ErrorMessage: p.ErrorMessage,
		},
	}
}
