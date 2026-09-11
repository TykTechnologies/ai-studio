// Package webhooks defines outbound webhook delivery: event-bus events are
// persisted, fanned out to administrator-approved HTTP targets, delivered
// with retries and a dead-letter queue, and every attempt is logged so an
// operator can search what was sent where, when, and what came back.
//
// Target URLs are an exfiltration vector, so a target never receives a
// delivery until an administrator approves it, and every change to where a
// target points (URL, custom headers) sends it back to pending.
//
// Community Edition ships a no-op implementation that answers every query
// with ErrEnterpriseFeature. Enterprise Edition registers the real one via
// the factory in this package.
package webhooks

import (
	"context"
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ErrEnterpriseFeature is returned by Community Edition for every operation.
var ErrEnterpriseFeature = errors.New("webhooks are an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")

// Errors shared by both editions. Handlers map them onto HTTP statuses.
var (
	ErrNotFound     = errors.New("webhook resource not found")
	ErrConflict     = errors.New("webhook target was modified by someone else; reload and retry")
	ErrInvalidState = errors.New("operation not allowed in the target's current state")
	ErrURLPolicy    = errors.New("target URL is not allowed by the webhook URL policy")
	ErrTemplate     = errors.New("payload template is invalid")
	ErrSameApprover = errors.New("a target must be approved by a different administrator than the one who created it")
	ErrNotApproved  = errors.New("target is not approved")
	ErrValidation   = errors.New("invalid webhook input")
	ErrDisabled     = errors.New("webhooks are disabled by configuration")
)

// Actor is the administrator performing an operation, taken from the
// authenticated request. It is recorded on the target and in the audit trail.
type Actor struct {
	UserID uint
	Email  string
	Name   string
}

// TargetInput creates a target.
type TargetInput struct {
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	URL            string            `json:"url"`
	TopicFilters   []string          `json:"topic_filters"`
	TemplatePreset string            `json:"template_preset"`
	TemplateBody   string            `json:"template_body"`
	Headers        map[string]string `json:"headers"`
	MaxConcurrency int               `json:"max_concurrency"`
	// SigningSecret is optional; a random one is generated when empty.
	SigningSecret string `json:"signing_secret"`
}

// TargetPatch updates a target. Nil fields are left unchanged. Changing URL
// or Headers on an approved target sends it back to pending.
type TargetPatch struct {
	Name           *string            `json:"name"`
	Description    *string            `json:"description"`
	URL            *string            `json:"url"`
	TopicFilters   *[]string          `json:"topic_filters"`
	TemplatePreset *string            `json:"template_preset"`
	TemplateBody   *string            `json:"template_body"`
	Headers        *map[string]string `json:"headers"`
	MaxConcurrency *int               `json:"max_concurrency"`
}

// TargetFilter narrows ListTargets. Zero values mean "no filter".
type TargetFilter struct {
	Status string
	Search string // case-insensitive substring over name, url, description
}

// TargetList is every matching target plus the pending count for the badge.
type TargetList struct {
	Targets      []models.WebhookTargetResponse `json:"targets"`
	PendingCount int64                          `json:"pending_count"`
}

// TargetStats summarises delivery outcomes for one target.
type TargetStats struct {
	TargetID     string  `json:"target_id"`
	Name         string  `json:"name"`
	Queued       int64   `json:"queued"`
	Retrying     int64   `json:"retrying"`
	InFlight     int64   `json:"in_flight"`
	Succeeded    int64   `json:"succeeded"`
	Failed       int64   `json:"failed"` // dead-lettered + cancelled
	DeadLettered int64   `json:"dead_lettered"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
}

// TargetDetail is one target with its delivery statistics.
type TargetDetail struct {
	Target models.WebhookTargetResponse `json:"target"`
	Stats  TargetStats                  `json:"stats"`
}

// RotatedSecret is the result of RotateSecret: the new secret, shown once,
// and how long the previous secret keeps producing a second signature.
type RotatedSecret struct {
	SigningSecret      string    `json:"signing_secret"`
	PreviousValidUntil time.Time `json:"previous_valid_until"`
}

// PreviewInput renders a template against a sample event without saving.
type PreviewInput struct {
	TemplatePreset string `json:"template_preset"`
	TemplateBody   string `json:"template_body"`
	Topic          string `json:"topic"`
}

// PreviewResult is the rendered sample and whether it is valid JSON.
type PreviewResult struct {
	Rendered string `json:"rendered"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
}

// Preset is a built-in payload template.
type Preset struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// TopicList is what the target editor offers: the platform's system topics
// plus every topic actually observed on the bus recently.
type TopicList struct {
	Known []string `json:"known"`
	Seen  []string `json:"seen"`
}

// DeliveryQuery filters the delivery log. Zero values mean "no filter".
type DeliveryQuery struct {
	TargetID string
	Topic    string
	Statuses []string
	EventID  string
	Kind     string
	Start    time.Time
	End      time.Time
	Search   string // case-insensitive substring over target url, topic, last error, response snippet

	Page     int // 1-based
	PageSize int
	SortAsc  bool // default newest first
}

// DeliveryPage is one page of deliveries plus the total for the filter.
// RenderedPayload is omitted from list rows; GetDelivery returns it.
type DeliveryPage struct {
	Deliveries []models.WebhookDelivery `json:"deliveries"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
}

// DeliveryDetail is one delivery with everything an operator needs to
// understand it: the attempts, the event that caused it and the target.
type DeliveryDetail struct {
	Delivery models.WebhookDelivery          `json:"delivery"`
	Attempts []models.WebhookDeliveryAttempt `json:"attempts"`
	Event    *models.WebhookEvent            `json:"event,omitempty"`
	Target   *models.WebhookTargetResponse   `json:"target,omitempty"`
}

// ReplayRequest selects dead letters to replay: explicit IDs, or every dead
// letter for a target (optionally all targets) up to Max.
type ReplayRequest struct {
	TargetID string   `json:"target_id"`
	IDs      []string `json:"ids"`
	Max      int      `json:"max"`
}

// ReplayResult counts what a bulk replay did.
type ReplayResult struct {
	Replayed int `json:"replayed"`
	Skipped  int `json:"skipped"`
}

// Stats aggregates deliveries over a window for the overview strip.
type Stats struct {
	Window                 string           `json:"window"`
	ByStatus               map[string]int64 `json:"by_status"`
	ByTarget               []TargetStats    `json:"by_target"`
	QueueDepth             int64            `json:"queue_depth"`
	OldestQueuedAgeSeconds int64            `json:"oldest_queued_age_seconds"`
}

// Status describes the running feature so the UI can explain itself.
type Status struct {
	Available     bool   `json:"available"`
	Enabled       bool   `json:"enabled"`
	BusConnected  bool   `json:"bus_connected"`
	WorkerEnabled bool   `json:"worker_enabled"`
	Workers       int    `json:"workers"`
	NodeID        string `json:"node_id,omitempty"`
	Dialect       string `json:"dialect,omitempty"`

	QueueDepth     int64 `json:"queue_depth"`
	Retrying       int64 `json:"retrying"`
	InFlight       int64 `json:"in_flight"`
	DeadLettered   int64 `json:"dead_lettered"`
	PendingTargets int64 `json:"pending_targets"`
	// DroppedEvents counts bus events this node could not persist even after
	// spilling to memory. Non-zero means at-least-once was violated.
	DroppedEvents int64      `json:"dropped_events"`
	LastIngestAt  *time.Time `json:"last_ingest_at,omitempty"`
}

// Export formats.
const (
	FormatCSV  = "csv"
	FormatJSON = "json"
)

// Paging bounds, shared by the API and the enterprise implementation.
const (
	DefaultPageSize = 50
	MaxPageSize     = 500
	MaxExportRows   = 50000
	MaxBulkReplay   = 500
)

// StatsWindows are the windows accepted by Stats.
var StatsWindows = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// Service is the webhooks contract shared by both editions.
type Service interface {
	// Targets
	ListTargets(ctx context.Context, f TargetFilter) (*TargetList, error)
	GetTarget(ctx context.Context, id string) (*TargetDetail, error)
	// CreateTarget stores a pending target. The returned secret is shown once.
	CreateTarget(ctx context.Context, actor Actor, in TargetInput) (*models.WebhookTargetResponse, string, error)
	// UpdateTarget applies a patch under optimistic locking. repended reports
	// whether the change sent an approved target back to pending.
	UpdateTarget(ctx context.Context, actor Actor, id string, lockVersion int, p TargetPatch) (view *models.WebhookTargetResponse, repended bool, err error)
	DeleteTarget(ctx context.Context, actor Actor, id string) error
	ApproveTarget(ctx context.Context, actor Actor, id string, note string) (*models.WebhookTargetResponse, error)
	RejectTarget(ctx context.Context, actor Actor, id string, reason string) (*models.WebhookTargetResponse, error)
	RevokeTarget(ctx context.Context, actor Actor, id string, reason string) (*models.WebhookTargetResponse, error)
	PauseTarget(ctx context.Context, actor Actor, id string) (*models.WebhookTargetResponse, error)
	ResumeTarget(ctx context.Context, actor Actor, id string) (*models.WebhookTargetResponse, error)
	RotateSecret(ctx context.Context, actor Actor, id string) (*RotatedSecret, error)
	// SendTest enqueues a synthetic event for one approved target and returns the delivery ID.
	SendTest(ctx context.Context, actor Actor, id string, topic string) (string, error)

	// Templates and topics
	PreviewTemplate(ctx context.Context, in PreviewInput) (*PreviewResult, error)
	ListPresets() []Preset
	ListTopics(ctx context.Context) (*TopicList, error)

	// Deliveries
	ListDeliveries(ctx context.Context, q DeliveryQuery) (*DeliveryPage, error)
	GetDelivery(ctx context.Context, id string) (*DeliveryDetail, error)
	// ReplayDelivery enqueues a new delivery for the same event and target and returns its ID.
	ReplayDelivery(ctx context.Context, actor Actor, id string, reRender bool) (string, error)
	CancelDelivery(ctx context.Context, actor Actor, id string) error
	ReplayDeadLetters(ctx context.Context, actor Actor, req ReplayRequest) (*ReplayResult, error)
	Stats(ctx context.Context, window string) (*Stats, error)
	// Export renders matching deliveries as CSV or JSON. Returns the bytes and the content type.
	Export(ctx context.Context, q DeliveryQuery, format string) ([]byte, string, error)

	// Lifecycle
	// Cleanup deletes rows past the retention windows. Returns rows removed.
	Cleanup(ctx context.Context) (int64, error)
	Status() Status
	// Stop drains in-flight deliveries and stops background workers.
	Stop()
}
