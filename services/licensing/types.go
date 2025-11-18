package licensing

import (
	"time"
)

// LicenseInfo contains parsed license information
type LicenseInfo struct {
	// JWT claims
	ExpiresAt time.Time
	IssuedAt  time.Time
	NotBefore time.Time
	Version   string

	// Feature entitlements from scope claim
	Features map[string]Feature

	// Raw token
	Token string
}

// Feature represents a license feature entitlement
type Feature struct {
	Name  string
	Value interface{}
}

// Bool returns the feature value as a boolean
func (f Feature) Bool() bool {
	if v, ok := f.Value.(bool); ok {
		return v
	}
	return false
}

// String returns the feature value as a string
func (f Feature) String() string {
	if v, ok := f.Value.(string); ok {
		return v
	}
	return ""
}

// Int returns the feature value as an integer
func (f Feature) Int() int {
	switch v := f.Value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// Config holds licensing configuration
type Config struct {
	// License key (JWT token)
	LicenseKey string

	// Telemetry configuration
	TelemetryURL            string
	TelemetryPeriod         time.Duration
	TelemetryDisabled       bool
	ValidityCheckPeriod     time.Duration
	TelemetryConcurrency    int
}

// TelemetryEvent represents a telemetry event to be sent
type TelemetryEvent struct {
	Identity   string                 `json:"identity"`   // Hashed license key
	Event      string                 `json:"event"`      // Event type
	Timestamp  int64                  `json:"timestamp"`  // Unix timestamp
	Properties map[string]interface{} `json:"properties"` // Event data
}

// Common event types
const (
	EventLLMReport        = "llm_report"
	EventAppReport        = "app_report"
	EventUserReport       = "user_report"
	EventChatReport       = "chat_report"
	EventHTTPInteraction  = "http_interaction"
)

// Common feature names
const (
	FeaturePortal              = "feature_portal"
	FeatureChat                = "feature_chat"
	FeatureGateway             = "feature_gateway"
	FeatureTrack               = "track"                     // Telemetry enabled
	FeatureHubSpokeMultiTenant = "hub_spoke_multi_tenant"    // Multi-tenant namespace support (ENT)
	FeatureGroups              = "feature_groups"            // Group-based access control (ENT)
)
