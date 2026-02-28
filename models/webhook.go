package models

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

// WebhookTransportConfig holds per-subscription HTTP transport settings.
// Zero/empty values mean "use service defaults".
type WebhookTransportConfig struct {
	// ProxyURL is an optional HTTP/HTTPS/SOCKS5 proxy for outbound requests.
	ProxyURL string `json:"proxy_url,omitempty"`
	// InsecureSkipVerify disables TLS certificate validation for this endpoint.
	// Use only when the target uses a self-signed cert you trust.
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
	// TLSCACert is an optional PEM-encoded CA certificate to trust when
	// validating the server's TLS certificate (e.g. private CA).
	TLSCACert string `json:"tls_ca_cert,omitempty"`
	// TLSClientCert and TLSClientKey are PEM-encoded client cert/key for mTLS.
	TLSClientCert string `json:"tls_client_cert,omitempty"`
	TLSClientKey  string `json:"tls_client_key,omitempty"`
}

const (
	DeliveryStatusPending = "pending"
	DeliveryStatusSuccess = "success"
	DeliveryStatusFailed  = "failed"
)

const WebhookConfigSingletonID = 1

// WebhookRetryPolicy is a value type shared by WebhookConfig (global defaults) and
// WebhookSubscription (per-subscription overrides).
// Zero values mean "use the next level's default".
type WebhookRetryPolicy struct {
	MaxAttempts    int   `json:"max_attempts"`
	BackoffSeconds []int `json:"backoff_seconds"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}

// WebhookTopic is a row in the subscription–topic join table.
type WebhookTopic struct {
	ID             uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	SubscriptionID uint   `json:"subscription_id" gorm:"not null;index;uniqueIndex:idx_sub_topic"`
	Topic          string `json:"topic" gorm:"not null;index;uniqueIndex:idx_sub_topic"`
}

// WebhookSubscription defines an outbound webhook destination.
type WebhookSubscription struct {
	gorm.Model
	Name            string                 `json:"name"`
	URL             string                 `json:"url"`
	Secret          string                 `json:"secret"`
	Enabled         bool                   `json:"enabled" gorm:"default:true"`
	Description     string                 `json:"description"`
	RetryPolicy     WebhookRetryPolicy     `json:"retry_policy" gorm:"serializer:json"`
	TransportConfig WebhookTransportConfig `json:"transport_config" gorm:"serializer:json"`
	Topics          []WebhookTopic         `json:"topics" gorm:"foreignKey:SubscriptionID;constraint:OnDelete:CASCADE"`
}

// BeforeSave encrypts sensitive fields before writing to the database.
// Uses the same AES-256 key (TYK_AI_SECRET_KEY) and $ENC/ prefix convention
// as the rest of the platform. A no-op if no key is configured.
func (s *WebhookSubscription) BeforeSave(tx *gorm.DB) error {
	s.Secret = secrets.EncryptValue(s.Secret)
	s.TransportConfig.ProxyURL = secrets.EncryptValue(s.TransportConfig.ProxyURL)
	s.TransportConfig.TLSCACert = secrets.EncryptValue(s.TransportConfig.TLSCACert)
	s.TransportConfig.TLSClientCert = secrets.EncryptValue(s.TransportConfig.TLSClientCert)
	s.TransportConfig.TLSClientKey = secrets.EncryptValue(s.TransportConfig.TLSClientKey)
	return nil
}

// AfterFind decrypts sensitive fields after loading from the database.
func (s *WebhookSubscription) AfterFind(tx *gorm.DB) error {
	s.Secret = secrets.DecryptValue(s.Secret)
	s.TransportConfig.ProxyURL = secrets.DecryptValue(s.TransportConfig.ProxyURL)
	s.TransportConfig.TLSCACert = secrets.DecryptValue(s.TransportConfig.TLSCACert)
	s.TransportConfig.TLSClientCert = secrets.DecryptValue(s.TransportConfig.TLSClientCert)
	s.TransportConfig.TLSClientKey = secrets.DecryptValue(s.TransportConfig.TLSClientKey)
	return nil
}

// WebhookEvent is the persistent delivery queue.
type WebhookEvent struct {
	gorm.Model
	SubscriptionID uint      `json:"subscription_id" gorm:"not null;index"`
	EventTopic     string    `json:"event_topic"`
	EventID        string    `json:"event_id"`
	Payload        string    `json:"payload" gorm:"type:text"`
	AttemptNumber  int       `json:"attempt_number" gorm:"default:1"`
	Status         string    `json:"status" gorm:"index:idx_webhook_events_status_next_run"`
	NextRunAt      time.Time `json:"next_run_at" gorm:"index:idx_webhook_events_status_next_run"`
	// TriggeredBy is non-zero for manually-triggered retries; holds the actor's user ID.
	TriggeredBy uint `json:"triggered_by" gorm:"default:0"`
}

// WebhookDeliveryLog records each HTTP delivery attempt for audit.
// CreatedAt is indexed to support efficient retention-based pruning.
type WebhookDeliveryLog struct {
	gorm.Model
	SubscriptionID uint       `json:"subscription_id" gorm:"not null;index"`
	EventTopic     string     `json:"event_topic"`
	EventID        string     `json:"event_id"`
	Payload        string     `json:"payload" gorm:"type:text"`
	AttemptNumber  int        `json:"attempt_number"`
	Status         string     `json:"status"`
	HTTPStatusCode int        `json:"http_status_code"`
	ResponseBody   string     `json:"response_body" gorm:"type:text"`
	ErrorMessage   string     `json:"error_message" gorm:"type:text"`
	AttemptedAt    time.Time  `json:"attempted_at"`
	NextRetryAt    *time.Time `json:"next_retry_at"`
}

// WebhookConfig is a DB singleton (ID=1) that holds dynamic global defaults.
type WebhookConfig struct {
	gorm.Model
	ID                   uint               `json:"id" gorm:"primaryKey"`
	Workers              int                `json:"workers"`
	QueueSize            int                `json:"queue_size"`
	DefaultRetryPolicy   WebhookRetryPolicy `json:"default_retry_policy" gorm:"serializer:json"`
	MaxResponseBodyBytes int                `json:"max_response_body_bytes"`
	// LogRetentionDays is how many days of delivery logs to keep. 0 = keep forever.
	LogRetentionDays int `json:"log_retention_days"`
}

func NewDefaultWebhookConfig() *WebhookConfig {
	return &WebhookConfig{ID: WebhookConfigSingletonID}
}

func GetWebhookConfig(db *gorm.DB) (*WebhookConfig, error) {
	cfg := NewDefaultWebhookConfig()
	// FirstOrCreate is atomic: safe under concurrent calls at startup.
	if err := db.Where(WebhookConfig{ID: WebhookConfigSingletonID}).
		Attrs(cfg).
		FirstOrCreate(cfg).Error; err != nil {
		return nil, err
	}
	return cfg, nil
}

func UpdateWebhookConfig(db *gorm.DB, cfg *WebhookConfig) error {
	cfg.ID = WebhookConfigSingletonID
	return db.Save(cfg).Error
}
