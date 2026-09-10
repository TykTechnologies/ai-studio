package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// RawJSON is a string column that already holds JSON. It marshals to the API
// as the JSON value itself (not a quoted string) so clients get an object.
// An empty value marshals as null.
type RawJSON string

// MarshalJSON emits the stored JSON verbatim, or null when empty or invalid.
func (r RawJSON) MarshalJSON() ([]byte, error) {
	if r == "" || !json.Valid([]byte(r)) {
		return []byte("null"), nil
	}
	return []byte(r), nil
}

// UnmarshalJSON accepts any JSON value and stores its raw bytes.
func (r *RawJSON) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*r = ""
		return nil
	}
	*r = RawJSON(b)
	return nil
}

// AuditRecord is one entry in the platform audit trail: a single request made
// to the management API, who made it, from where, what it touched, and what
// changed. The field set mirrors the Tyk Dashboard audit log (req_id, ip,
// user, action, method, url, status, diff, request_dump, response_dump) with
// resource identification added because AI Studio objects are typed.
//
// Records are append-only: there is no update path and retention is the only
// delete path. Deliberately not gorm.Model so there is no soft-delete column.
type AuditRecord struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RequestID string    `gorm:"size:64;index:idx_audit_req_id" json:"req_id"`
	Timestamp time.Time `gorm:"index:idx_audit_ts" json:"timestamp"`

	// Actor
	IP        string `gorm:"size:64;index:idx_audit_ip" json:"ip"`
	UserID    uint   `gorm:"index:idx_audit_user_id" json:"user_id"`
	UserEmail string `gorm:"size:255;index:idx_audit_user_email" json:"user"`
	UserName  string `gorm:"size:255" json:"user_name"`
	UserAgent string `gorm:"size:512" json:"user_agent,omitempty"`

	// What happened
	Action string `gorm:"size:128;index:idx_audit_action" json:"action"`
	Method string `gorm:"size:16;index:idx_audit_method" json:"method"`
	URL    string `gorm:"size:2048" json:"url"`
	Route  string `gorm:"size:255;index:idx_audit_route" json:"route"`
	Status int    `gorm:"index:idx_audit_status" json:"status"`

	// What it touched
	ResourceType string `gorm:"size:64;index:idx_audit_resource,priority:1" json:"resource_type"`
	ResourceID   string `gorm:"size:255;index:idx_audit_resource,priority:2" json:"resource_id"`
	ResourceName string `gorm:"size:255" json:"resource_name"`

	// Diff of changed fields for updates, and the final state for deletes.
	// Shape: {"field": {"old": <v>, "new": <v>}}. Sensitive columns are
	// redacted before storage.
	Diff RawJSON `gorm:"type:text" json:"diff"`

	// Populated only when detailed recording is enabled.
	RequestDump  RawJSON `gorm:"type:text" json:"request_dump"`
	ResponseDump RawJSON `gorm:"type:text" json:"response_dump"`

	Error      string `gorm:"size:1024" json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// TableName matches the Tyk Dashboard audit collection name.
func (AuditRecord) TableName() string {
	return "audit_records"
}

// Create inserts a single audit record.
func (r *AuditRecord) Create(db *gorm.DB) error {
	return db.Create(r).Error
}

// AuditRecords is a slice helper for batch inserts.
type AuditRecords []AuditRecord

// CreateBatch inserts records in chunks.
func (rs AuditRecords) CreateBatch(db *gorm.DB, batchSize int) error {
	if len(rs) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	return db.CreateInBatches(rs, batchSize).Error
}
