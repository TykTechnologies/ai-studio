package models

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

// Webhook target lifecycle. A target is created pending and never receives a
// delivery until an administrator approves its URL. Changing the URL or the
// custom headers of an approved target sends it back to pending.
const (
	WebhookTargetPending  = "pending"
	WebhookTargetApproved = "approved"
	WebhookTargetRejected = "rejected"
	WebhookTargetRevoked  = "revoked"
)

// Delivery states. queued/retrying are claimable; in_flight is leased by a
// worker; the remaining three are terminal (dead_lettered can be replayed).
const (
	WebhookDeliveryQueued       = "queued"
	WebhookDeliveryInFlight     = "in_flight"
	WebhookDeliverySucceeded    = "succeeded"
	WebhookDeliveryRetrying     = "retrying"
	WebhookDeliveryDeadLettered = "dead_lettered"
	WebhookDeliveryCancelled    = "cancelled"
)

// Delivery kinds. Only event deliveries are deduplicated on (event, target);
// tests and replays always create a new row.
const (
	WebhookDeliveryKindEvent  = "event"
	WebhookDeliveryKindTest   = "test"
	WebhookDeliveryKindReplay = "replay"
)

// Attempt outcomes recorded per HTTP attempt (or per non-HTTP terminal step).
const (
	WebhookAttemptSuccess      = "success"
	WebhookAttemptRetry        = "retry"
	WebhookAttemptDeadLetter   = "dead_letter"
	WebhookAttemptCancelled    = "cancelled"
	WebhookAttemptLeaseExpired = "lease_expired"
	WebhookAttemptRenderError  = "render_error"
)

// Payload template presets. "custom" uses TemplateBody.
const (
	WebhookTemplateStandard = "standard"
	WebhookTemplateSlack    = "slack"
	WebhookTemplateMinimal  = "minimal"
	WebhookTemplateCustom   = "custom"
)

// WebhookTarget is an approved (or not yet approved) outbound HTTP endpoint
// that receives event-bus events matching its topic filters.
//
// Custom headers and signing secrets are encrypted at rest through the
// BeforeSave/AfterFind hooks (same scheme as Submission credentials) and are
// never serialised to the API: see ToResponse.
type WebhookTarget struct {
	ID          string `gorm:"primaryKey;size:36" json:"id"`
	Name        string `gorm:"size:200;not null" json:"name"`
	Description string `gorm:"size:1024" json:"description"`
	URL         string `gorm:"size:2048;not null" json:"url"`
	Status      string `gorm:"size:16;index:idx_webhook_targets_status;not null" json:"status"`
	Paused      bool   `json:"paused"`

	// TopicFilters are path.Match globs, e.g. "system.llm.*". Empty matches nothing.
	// Stored as JSON text in TopicFiltersJSON (a plain string column, so the
	// audit trail can snapshot the row into a map) and synced by the hooks.
	TopicFilters     []string `gorm:"-" json:"topic_filters"`
	TopicFiltersJSON string   `gorm:"column:topic_filters;type:text" json:"-"`

	TemplatePreset string `gorm:"size:32" json:"template_preset"`
	TemplateBody   string `gorm:"type:text" json:"template_body"`

	// Headers is a JSON-encoded map[string]string of extra request headers.
	// Encrypted at rest because it commonly carries an Authorization value.
	Headers string `gorm:"type:text" json:"-"`

	// SigningSecret signs every delivery (HMAC-SHA256). PrevSigningSecret is
	// honoured until PrevSecretExpiresAt after a rotation so receivers can roll.
	SigningSecret       string     `gorm:"type:text" json:"-"`
	PrevSigningSecret   string     `gorm:"type:text" json:"-"`
	PrevSecretExpiresAt *time.Time `json:"-"`

	// MaxConcurrency caps in-flight deliveries to this target; 0 = engine default.
	MaxConcurrency int `json:"max_concurrency"`

	CreatedByUserID  uint       `json:"created_by_user_id"`
	CreatedByEmail   string     `gorm:"size:255" json:"created_by_email"`
	ApprovedByUserID uint       `json:"approved_by_user_id"`
	ApprovedByEmail  string     `gorm:"size:255" json:"approved_by_email"`
	ApprovedAt       *time.Time `json:"approved_at"`
	RejectedReason   string     `gorm:"size:1024" json:"rejected_reason"`
	RevokedByUserID  uint       `json:"revoked_by_user_id"`
	RevokedByEmail   string     `gorm:"size:255" json:"revoked_by_email"`
	RevokedAt        *time.Time `json:"revoked_at"`
	RevokedReason    string     `gorm:"size:1024" json:"revoked_reason"`

	LastDeliveryAt      *time.Time `json:"last_delivery_at"`
	LastSuccessAt       *time.Time `json:"last_success_at"`
	LastFailureAt       *time.Time `json:"last_failure_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`

	// LockVersion is bumped on every save; clients send it back on update so
	// two admins editing the same target cannot silently overwrite each other.
	LockVersion int       `gorm:"not null;default:0" json:"lock_version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (WebhookTarget) TableName() string { return "webhook_targets" }

// encryptedPrefix is what secrets.EncryptValue prepends. BeforeSave checks it
// so a value that is already ciphertext is never encrypted twice.
const encryptedPrefix = "$ENC/"

func encryptIfPlain(v string) string {
	if v == "" || strings.HasPrefix(v, encryptedPrefix) {
		return v
	}
	return secrets.EncryptValue(v)
}

// BeforeSave encrypts the header map and signing secrets before they hit the
// DB and serialises the topic filters.
func (t *WebhookTarget) BeforeSave(tx *gorm.DB) error {
	t.Headers = encryptIfPlain(t.Headers)
	t.SigningSecret = encryptIfPlain(t.SigningSecret)
	t.PrevSigningSecret = encryptIfPlain(t.PrevSigningSecret)
	if t.TopicFilters == nil {
		t.TopicFilters = []string{}
	}
	b, err := json.Marshal(t.TopicFilters)
	if err != nil {
		return err
	}
	t.TopicFiltersJSON = string(b)
	return nil
}

// AfterSave restores plaintext on the in-memory struct so callers that keep
// using the object after Create/Save do not see ciphertext.
func (t *WebhookTarget) AfterSave(tx *gorm.DB) error {
	return t.AfterFind(tx)
}

// AfterFind decrypts the header map and signing secrets after a read and
// decodes the topic filters.
func (t *WebhookTarget) AfterFind(tx *gorm.DB) error {
	t.Headers = secrets.DecryptValue(t.Headers)
	t.SigningSecret = secrets.DecryptValue(t.SigningSecret)
	t.PrevSigningSecret = secrets.DecryptValue(t.PrevSigningSecret)
	filters := []string{}
	if strings.TrimSpace(t.TopicFiltersJSON) != "" {
		_ = json.Unmarshal([]byte(t.TopicFiltersJSON), &filters)
	}
	t.TopicFilters = filters
	return nil
}

// HeaderMap decodes the custom headers. Invalid or empty JSON yields an empty map.
func (t *WebhookTarget) HeaderMap() map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(t.Headers) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(t.Headers), &out)
	return out
}

// SetHeaderMap encodes the custom headers. A nil or empty map clears them.
func (t *WebhookTarget) SetHeaderMap(h map[string]string) {
	if len(h) == 0 {
		t.Headers = ""
		return
	}
	b, _ := json.Marshal(h)
	t.Headers = string(b)
}

// HeaderNames lists the custom header names, sorted, without values.
func (t *WebhookTarget) HeaderNames() []string {
	h := t.HeaderMap()
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// IsDeliverable reports whether deliveries to this target may be sent now.
func (t *WebhookTarget) IsDeliverable() bool {
	return t.Status == WebhookTargetApproved && !t.Paused
}

// WebhookTargetResponse is the API shape of a target. Secret values are never
// included: custom headers appear as names only and the signing secret as a flag.
type WebhookTargetResponse struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	URL                 string     `json:"url"`
	Status              string     `json:"status"`
	Paused              bool       `json:"paused"`
	TopicFilters        []string   `json:"topic_filters"`
	TemplatePreset      string     `json:"template_preset"`
	TemplateBody        string     `json:"template_body"`
	HeaderNames         []string   `json:"header_names"`
	HasSigningSecret    bool       `json:"has_signing_secret"`
	MaxConcurrency      int        `json:"max_concurrency"`
	CreatedByUserID     uint       `json:"created_by_user_id"`
	CreatedByEmail      string     `json:"created_by_email"`
	ApprovedByUserID    uint       `json:"approved_by_user_id"`
	ApprovedByEmail     string     `json:"approved_by_email"`
	ApprovedAt          *time.Time `json:"approved_at"`
	RejectedReason      string     `json:"rejected_reason"`
	RevokedByUserID     uint       `json:"revoked_by_user_id"`
	RevokedByEmail      string     `json:"revoked_by_email"`
	RevokedAt           *time.Time `json:"revoked_at"`
	RevokedReason       string     `json:"revoked_reason"`
	LastDeliveryAt      *time.Time `json:"last_delivery_at"`
	LastSuccessAt       *time.Time `json:"last_success_at"`
	LastFailureAt       *time.Time `json:"last_failure_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LockVersion         int        `json:"lock_version"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// ToResponse converts the target to its API shape, dropping every secret.
func (t *WebhookTarget) ToResponse() WebhookTargetResponse {
	filters := t.TopicFilters
	if filters == nil {
		filters = []string{}
	}
	return WebhookTargetResponse{
		ID: t.ID, Name: t.Name, Description: t.Description, URL: t.URL,
		Status: t.Status, Paused: t.Paused, TopicFilters: filters,
		TemplatePreset: t.TemplatePreset, TemplateBody: t.TemplateBody,
		HeaderNames: t.HeaderNames(), HasSigningSecret: t.SigningSecret != "",
		MaxConcurrency:  t.MaxConcurrency,
		CreatedByUserID: t.CreatedByUserID, CreatedByEmail: t.CreatedByEmail,
		ApprovedByUserID: t.ApprovedByUserID, ApprovedByEmail: t.ApprovedByEmail, ApprovedAt: t.ApprovedAt,
		RejectedReason:  t.RejectedReason,
		RevokedByUserID: t.RevokedByUserID, RevokedByEmail: t.RevokedByEmail, RevokedAt: t.RevokedAt, RevokedReason: t.RevokedReason,
		LastDeliveryAt: t.LastDeliveryAt, LastSuccessAt: t.LastSuccessAt, LastFailureAt: t.LastFailureAt,
		ConsecutiveFailures: t.ConsecutiveFailures, LockVersion: t.LockVersion,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

// WebhookEvent is a bus event persisted for fan-out and replay. The primary
// key is the bus event ID, which is what makes ingestion idempotent. Payload
// is stored after redaction; the unredacted object is never written.
type WebhookEvent struct {
	ID          string    `gorm:"primaryKey;size:128" json:"id"`
	Topic       string    `gorm:"size:200;index:idx_webhook_events_topic_received,priority:1" json:"topic"`
	Origin      string    `gorm:"size:128" json:"origin"`
	Payload     RawJSON   `gorm:"type:text" json:"payload"`
	ObjectType  string    `gorm:"size:64" json:"object_type"`
	Action      string    `gorm:"size:32" json:"action"`
	ObjectID    uint      `json:"object_id"`
	ActorUserID uint      `json:"actor_user_id"`
	ReceivedAt  time.Time `gorm:"index:idx_webhook_events_topic_received,priority:2;index:idx_webhook_events_received" json:"received_at"`
	FanoutCount int       `json:"fanout_count"`
	// Synthetic marks events created by "send test", not received from the bus.
	Synthetic bool `json:"synthetic"`
}

func (WebhookEvent) TableName() string { return "webhook_events" }

// WebhookDelivery is one (event, target) outbox row. Workers claim due rows
// with a lease and an optimistic lock, so any number of Studio nodes can run
// workers against a shared database without double delivery.
type WebhookDelivery struct {
	ID       string `gorm:"primaryKey;size:36" json:"id"`
	EventID  string `gorm:"size:128;index:idx_webhook_deliveries_event" json:"event_id"`
	TargetID string `gorm:"size:36;index:idx_webhook_deliveries_target_created,priority:1" json:"target_id"`
	Topic    string `gorm:"size:200;index:idx_webhook_deliveries_topic_created,priority:1" json:"topic"`
	// TargetURLSnapshot is the URL at enqueue time, kept so the log stays
	// meaningful after the target is edited or deleted.
	TargetURLSnapshot string `gorm:"size:2048" json:"target_url"`

	Status         string     `gorm:"size:16;index:idx_webhook_deliveries_status_next,priority:1" json:"status"`
	AttemptCount   int        `json:"attempt_count"`
	MaxAttempts    int        `json:"max_attempts"`
	NextAttemptAt  time.Time  `gorm:"index:idx_webhook_deliveries_status_next,priority:2" json:"next_attempt_at"`
	LeaseOwner     string     `gorm:"size:128" json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`

	// RenderedPayload is produced once on the first attempt and reused by
	// every retry so receivers see byte-identical bodies.
	RenderedPayload     string `gorm:"type:text" json:"rendered_payload,omitempty"`
	RenderError         string `gorm:"size:1024" json:"render_error,omitempty"`
	LastStatusCode      int    `json:"last_status_code"`
	LastError           string `gorm:"size:1024" json:"last_error"`
	LastResponseSnippet string `gorm:"size:4096" json:"last_response_snippet,omitempty"`

	ReplayOfID string `gorm:"size:36;index:idx_webhook_deliveries_replay_of" json:"replay_of_id,omitempty"`
	Kind       string `gorm:"size:16;index:idx_webhook_deliveries_kind_created,priority:1" json:"kind"`
	// DedupeKey is "<event_id>:<target_id>" for event deliveries so a bus
	// event that arrives twice fans out once. Test and replay rows use their
	// own ID so they are never deduplicated.
	DedupeKey string `gorm:"size:200;uniqueIndex:uq_webhook_deliveries_dedupe" json:"-"`

	LockVersion int        `gorm:"not null;default:0" json:"-"`
	CreatedAt   time.Time  `gorm:"index:idx_webhook_deliveries_target_created,priority:2;index:idx_webhook_deliveries_topic_created,priority:2;index:idx_webhook_deliveries_kind_created,priority:2;index:idx_webhook_deliveries_created" json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (WebhookDelivery) TableName() string { return "webhook_deliveries" }

// IsTerminal reports whether the delivery will not be attempted again.
func (d *WebhookDelivery) IsTerminal() bool {
	switch d.Status {
	case WebhookDeliverySucceeded, WebhookDeliveryDeadLettered, WebhookDeliveryCancelled:
		return true
	}
	return false
}

// WebhookDeliveryAttempt records one HTTP attempt (or one non-HTTP terminal
// step such as a render error). RequestHeaders holds only the X-Webhook-*,
// Content-Type and User-Agent headers, never the target's custom headers.
type WebhookDeliveryAttempt struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	DeliveryID      string    `gorm:"size:36;index:idx_webhook_attempts_delivery" json:"delivery_id"`
	Attempt         int       `json:"attempt"`
	StartedAt       time.Time `json:"started_at"`
	DurationMs      int64     `json:"duration_ms"`
	StatusCode      int       `json:"status_code"`
	RequestHeaders  RawJSON   `gorm:"type:text" json:"request_headers"`
	ResponseHeaders RawJSON   `gorm:"type:text" json:"response_headers"`
	ResponseSnippet string    `gorm:"size:4096" json:"response_snippet"`
	Error           string    `gorm:"size:1024" json:"error"`
	Outcome         string    `gorm:"size:16" json:"outcome"`
}

func (WebhookDeliveryAttempt) TableName() string { return "webhook_delivery_attempts" }
