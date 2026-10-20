package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
)

// Agent tracing settings stored on Apps and on the resources they reach
// (spec §11). The columns exist in every edition; they only take effect when
// the Enterprise agent tracing feature is running.

// AppTracingSettings are an App's overrides of the global agent tracing
// settings. An empty field inherits the global value.
//
// Stored via Scanner/Valuer like LLMPriceMultipliers; all-empty is NULL.
type AppTracingSettings struct {
	// Enabled turns tracing on or off for the App; nil inherits.
	Enabled *bool `json:"enabled,omitempty"`
	// ContentMode is the App's capture mode (off|metadata|hashed|redacted|full).
	ContentMode string `json:"content_mode,omitempty"`
	// MaxRequestOverride is the highest mode a request header may raise
	// capture to for this App.
	MaxRequestOverride string `json:"max_request_override,omitempty"`
	// SessionIdleTimeout closes a session after this long without activity
	// (a Go duration, e.g. "2h").
	SessionIdleTimeout string `json:"session_idle_timeout,omitempty"`
	// PayloadDays is how long captured content is kept; nil inherits.
	PayloadDays *int `json:"payload_days,omitempty"`
	// OwnerMayViewContent lets the App's Portal owner read captured content
	// (Q4). Only administrators or holders of traces write may set it, and
	// it is hub-only: never synced to edges.
	OwnerMayViewContent bool `json:"owner_may_view_content,omitempty"`
}

// TracingOn reports whether the App's traffic is traced: on unless the App
// switched tracing off (nil inherits the install, which is on wherever
// capture runs at all).
func (s AppTracingSettings) TracingOn() bool { return s.Enabled == nil || *s.Enabled }

// Bounds of the App settings.
const (
	MinTracingSessionIdleTimeout = time.Minute
	MaxTracingSessionIdleTimeout = 7 * 24 * time.Hour
	MaxTracingPayloadDays        = 365
)

// IsZero reports whether every setting inherits.
func (s AppTracingSettings) IsZero() bool {
	return s == AppTracingSettings{}
}

// Validate checks the modes, the timeout and the retention.
func (s AppTracingSettings) Validate() error {
	if err := ValidateTracingContentMode("tracing.content_mode", s.ContentMode); err != nil {
		return err
	}
	if err := ValidateTracingContentMode("tracing.max_request_override", s.MaxRequestOverride); err != nil {
		return err
	}
	if s.SessionIdleTimeout != "" {
		d, err := time.ParseDuration(s.SessionIdleTimeout)
		if err != nil {
			return fmt.Errorf("tracing.session_idle_timeout: %q is not a duration such as 30m or 2h", s.SessionIdleTimeout)
		}
		if d < MinTracingSessionIdleTimeout || d > MaxTracingSessionIdleTimeout {
			return fmt.Errorf("tracing.session_idle_timeout: must be between %s and %s", MinTracingSessionIdleTimeout, MaxTracingSessionIdleTimeout)
		}
	}
	if s.PayloadDays != nil && (*s.PayloadDays < 0 || *s.PayloadDays > MaxTracingPayloadDays) {
		return fmt.Errorf("tracing.payload_days: must be between 0 and %d", MaxTracingPayloadDays)
	}
	return nil
}

// EdgeSettings is the part of the settings edges receive: everything but
// OwnerMayViewContent, which only the hub's Portal reads.
func (s AppTracingSettings) EdgeSettings() AppTracingSettings {
	s.OwnerMayViewContent = false
	return s
}

// EdgeJSON is the edge settings as the JSON the config snapshot carries, or
// "" when they all inherit.
func (s AppTracingSettings) EdgeJSON() string {
	e := s.EdgeSettings()
	if e.IsZero() {
		return ""
	}
	b, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(b)
}

// Scan implements sql.Scanner.
func (s *AppTracingSettings) Scan(value interface{}) error {
	*s = AppTracingSettings{}
	if value == nil {
		return nil
	}
	return JSONScan(value, s)
}

// Value implements driver.Valuer. No overrides is stored as NULL.
func (s AppTracingSettings) Value() (driver.Value, error) {
	if s.IsZero() {
		return nil, nil
	}
	return JSONValue(s)
}

// ValidateTracingContentMode accepts "" (inherit or no cap) or a known mode.
func ValidateTracingContentMode(field, mode string) error {
	if mode == "" || config.AgentTracingContentModeRank(mode) >= 0 {
		return nil
	}
	return fmt.Errorf("%s: %q is not one of off, metadata, hashed, redacted, full", field, mode)
}
