// Package audit defines the platform audit trail: an append-only record of
// every action taken through the management API, who took it, from where, and
// what changed. It exists so a security team can walk back an incident and
// reconstruct the history of any object on the platform.
//
// The design mirrors the Tyk Dashboard audit log, which has been in production
// against enterprise requirements for years: one record per request under the
// API routes, with a request id, actor, IP, action name, method, URL, status,
// and a diff of changed fields for updates. Storage is the database (queryable
// from the API and UI), a log file, or both.
//
// Community Edition ships a no-op implementation. Enterprise Edition registers
// the real one via the factory in this package.
package audit

import (
	"context"
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// ErrEnterpriseFeature is returned by Community Edition for every query.
var ErrEnterpriseFeature = errors.New("the audit trail is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

// ErrNotFound is returned when a single record lookup misses.
var ErrNotFound = errors.New("audit record not found")

// ErrDisabled is returned when recording is switched off or records are not
// stored in the database (file-only store type).
var ErrDisabled = errors.New("audit trail database storage is not enabled")

// Query is the filter set for listing, summarising and exporting records.
// Zero values mean "no filter".
type Query struct {
	Start time.Time
	End   time.Time

	UserID       uint
	UserEmail    string // case-insensitive substring match
	Action       string // exact match
	ResourceType string // exact match
	ResourceID   string // exact match
	Method       string // exact match, upper-cased by the caller
	Status       int    // exact match
	StatusClass  int    // 2, 4 or 5 → 2xx, 4xx, 5xx
	IP           string // exact match
	RequestID    string // exact match
	Search       string // case-insensitive substring over action, url, user, resource name, ip

	Page     int // 1-based
	PageSize int
	SortAsc  bool // default newest first
}

// Page is one page of records plus the total count for the filter.
type Page struct {
	Records  []models.AuditRecord `json:"records"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

// DayCount is a per-day bucket for the timeline.
type DayCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// NamedCount is a count keyed by a label (action, user, resource type).
type NamedCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// Summary aggregates the records matching a query for filter facets and the
// overview strip on the audit page.
type Summary struct {
	Total          int64        `json:"total"`
	Failed         int64        `json:"failed"` // status >= 400
	DistinctUsers  int64        `json:"distinct_users"`
	ByAction       []NamedCount `json:"by_action"`
	ByUser         []NamedCount `json:"by_user"`
	ByResourceType []NamedCount `json:"by_resource_type"`
	ByStatusClass  []NamedCount `json:"by_status_class"`
	Timeline       []DayCount   `json:"timeline"`
}

// Status describes the running configuration so the UI can explain itself.
type Status struct {
	Available         bool   `json:"available"`
	Enabled           bool   `json:"enabled"`
	StoreType         string `json:"store_type"`
	DetailedRecording bool   `json:"detailed_recording"`
	RecordReads       bool   `json:"record_reads"`
	RetentionDays     int    `json:"retention_days"`
	QueueDepth        int    `json:"queue_depth"`
	Dropped           int64  `json:"dropped"`
}

// Export formats.
const (
	FormatCSV  = "csv"
	FormatJSON = "json"
)

// Service is the audit trail contract shared by both editions.
type Service interface {
	// Middleware returns the Gin handler that records requests. It must be
	// registered on the engine before routes are added. CE returns a pass-through.
	Middleware() gin.HandlerFunc

	// Record stores a record produced outside the HTTP path (background jobs,
	// gRPC control plane operations). Non-blocking in ENT; no-op in CE.
	Record(ctx context.Context, rec *models.AuditRecord) error

	// List returns a page of records for the filter.
	List(ctx context.Context, q Query) (*Page, error)

	// Get returns one record including any stored dumps.
	Get(ctx context.Context, id uint) (*models.AuditRecord, error)

	// Summary aggregates records for the filter.
	Summary(ctx context.Context, q Query) (*Summary, error)

	// Export renders matching records as CSV or JSON. Returns the bytes and
	// the content type.
	Export(ctx context.Context, q Query, format string) ([]byte, string, error)

	// Cleanup deletes records past the retention window. Returns rows removed.
	Cleanup(ctx context.Context) (int64, error)

	// Status reports the effective configuration and queue health.
	Status() Status

	// Stop flushes queued records and stops background workers.
	Stop()
}
