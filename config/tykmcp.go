package config

import (
	"os"
	"strconv"
	"time"
)

// TykMCPConfig controls the Tyk Dashboard MCP integration (Enterprise
// feature): MCP proxies defined in a Tyk Dashboard are imported into the AI
// Portal, registered from Studio, and access keys are brokered against Tyk
// policies. Every switch here can only disable or tighten the feature.
type TykMCPConfig struct {
	// Enabled turns the feature on. Enterprise builds default to true.
	Enabled bool
	// SyncMinInterval is the floor for a connection's sync interval.
	SyncMinInterval time.Duration
	// RequestTimeout is the per-call HTTP timeout for Dashboard reads;
	// writes get three times this value.
	RequestTimeout time.Duration
	// AllowedHosts, when non-empty, restricts Dashboard and MDCB URLs to
	// these hosts (exact or ".suffix"). DeniedHosts always rejects and wins.
	AllowedHosts []string
	DeniedHosts  []string
	// RequireDifferentActivator rejects activation of a connection by the
	// administrator who created it.
	RequireDifferentActivator bool
	// SyncRunRetention deletes sync run records older than this.
	SyncRunRetention time.Duration
	// RateLimitPerSecond caps outbound Dashboard calls per connection.
	RateLimitPerSecond int
}

const (
	tykMCPMaxRequestTimeout = 60 * time.Second
	tykMCPMinSyncInterval   = 10 * time.Second
)

func getTykMCPConfig() TykMCPConfig {
	cfg := TykMCPConfig{
		Enabled:            true,
		SyncMinInterval:    60 * time.Second,
		RequestTimeout:     10 * time.Second,
		SyncRunRetention:   30 * 24 * time.Hour,
		RateLimitPerSecond: 10,
	}
	if v := os.Getenv("TYK_MCP_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Enabled = b
		} else {
			cfgLog.Warn().Msgf("Invalid TYK_MCP_ENABLED value: %s. Using default: %t", v, cfg.Enabled)
		}
	}
	cfg.SyncMinInterval = parseDurationWithDefault("TYK_MCP_SYNC_MIN_INTERVAL", cfg.SyncMinInterval)
	if cfg.SyncMinInterval < tykMCPMinSyncInterval {
		cfgLog.Warn().Msgf("TYK_MCP_SYNC_MIN_INTERVAL (%s) is below %s; using the minimum", cfg.SyncMinInterval, tykMCPMinSyncInterval)
		cfg.SyncMinInterval = tykMCPMinSyncInterval
	}
	cfg.RequestTimeout = parseDurationWithDefault("TYK_MCP_REQUEST_TIMEOUT", cfg.RequestTimeout)
	if cfg.RequestTimeout <= 0 || cfg.RequestTimeout > tykMCPMaxRequestTimeout {
		cfgLog.Warn().Msgf("TYK_MCP_REQUEST_TIMEOUT (%s) must be between 1s and %s; using 10s", cfg.RequestTimeout, tykMCPMaxRequestTimeout)
		cfg.RequestTimeout = 10 * time.Second
	}
	cfg.AllowedHosts = splitCSVList(os.Getenv("TYK_MCP_ALLOWED_HOSTS"))
	cfg.DeniedHosts = splitCSVList(os.Getenv("TYK_MCP_DENIED_HOSTS"))
	if v := os.Getenv("TYK_MCP_REQUIRE_DIFFERENT_ACTIVATOR"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.RequireDifferentActivator = b
		}
	}
	cfg.SyncRunRetention = parseDurationWithDefault("TYK_MCP_SYNC_RUN_RETENTION", cfg.SyncRunRetention)
	if v := os.Getenv("TYK_MCP_RATE_LIMIT_PER_SECOND"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 1000 {
			cfg.RateLimitPerSecond = n
		} else {
			cfgLog.Warn().Msgf("Invalid TYK_MCP_RATE_LIMIT_PER_SECOND value: %s. Using default: %d", v, cfg.RateLimitPerSecond)
		}
	}
	return cfg
}
