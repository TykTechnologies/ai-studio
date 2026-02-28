package models

import (
	"time"

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

// WebhookEvent is the persistent delivery queue.
type WebhookEvent struct {
	gorm.Model
	SubscriptionID uint      `json:"subscription_id" gorm:"not null;index"`
	EventTopic     string    `json:"event_topic"`
	EventID        string    `json:"event_id"`
	Payload        string    `json:"payload" gorm:"type:text"`
	AttemptNumber  int       `json:"attempt_number" gorm:"default:1"`
	Status         string    `json:"status" gorm:"index"`
	NextRunAt      time.Time `json:"next_run_at" gorm:"index"`
}

// WebhookDeliveryLog records each HTTP delivery attempt for audit.
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
}

func NewDefaultWebhookConfig() *WebhookConfig {
	return &WebhookConfig{ID: WebhookConfigSingletonID}
}

func GetWebhookConfig(db *gorm.DB) (*WebhookConfig, error) {
	var cfg WebhookConfig
	err := db.First(&cfg, WebhookConfigSingletonID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			cfg = *NewDefaultWebhookConfig()
			if err2 := db.Create(&cfg).Error; err2 != nil {
				return nil, err2
			}
			return &cfg, nil
		}
		return nil, err
	}
	return &cfg, nil
}

func UpdateWebhookConfig(db *gorm.DB, cfg *WebhookConfig) error {
	cfg.ID = WebhookConfigSingletonID
	return db.Save(cfg).Error
}
