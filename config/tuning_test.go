package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The tuning settings that used to be read at their point of use are on
// AppConf, with the same environment names and defaults.
func TestLoadFrom_TuningSettings(t *testing.T) {
	defaults := LoadFrom(func(string) string { return "" })
	assert.Equal(t, 1000, defaults.AnalyticsBufferSize)
	assert.Equal(t, 30*time.Second, defaults.BudgetSyncInterval)
	assert.False(t, defaults.DebugHTTPProxy)
	assert.False(t, defaults.MetricsNoLegacyNames, "legacy metric names stay on by default")
	assert.Empty(t, defaults.CORSAllowedOrigins)
	assert.False(t, defaults.SkipFilterDefaults)
	assert.Empty(t, defaults.FilterLimits)

	env := map[string]string{
		"ANALYTICS_BUFFER_SIZE":  "250",
		"BUDGET_SYNC_INTERVAL":   "5s",
		"DEBUG_HTTP_PROXY":       "true",
		"METRICS_LEGACY_NAMES":   "false",
		"CORS_ALLOWED_ORIGINS":   "https://a.example.com, https://b.example.com",
		"SKIP_FILTER_DEFAULTS":   "true",
		"FILTER_SCRIPT_TIMEOUT":  "7s",
		"FILTER_SCRIPT_ALLOW_OS": "true", // security switch: not a tuning limit
	}
	c := LoadFrom(func(k string) string { return env[k] })
	assert.Equal(t, 250, c.AnalyticsBufferSize)
	assert.Equal(t, 5*time.Second, c.BudgetSyncInterval)
	assert.True(t, c.DebugHTTPProxy)
	assert.True(t, c.MetricsNoLegacyNames)
	assert.Equal(t, "https://a.example.com, https://b.example.com", c.CORSAllowedOrigins)
	assert.True(t, c.SkipFilterDefaults)
	assert.Equal(t, map[string]string{"FILTER_SCRIPT_TIMEOUT": "7s"}, c.FilterLimits)

	bad := LoadFrom(func(k string) string {
		return map[string]string{"ANALYTICS_BUFFER_SIZE": "lots", "BUDGET_SYNC_INTERVAL": "-1s"}[k]
	})
	assert.Equal(t, 1000, bad.AnalyticsBufferSize, "invalid values keep the default")
	assert.Equal(t, 30*time.Second, bad.BudgetSyncInterval)
}

func TestInstalledNeverLoads(t *testing.T) {
	ResetGlobalConfig()
	t.Cleanup(ResetGlobalConfig)
	assert.Nil(t, Installed())
	conf := &AppConf{SiteURL: "x"}
	Set(conf)
	assert.Same(t, conf, Installed())
}

// MIGRATION_LOCK_TIMEOUT bounds a starting Studio's wait for another
// instance's migrations (default 15m).
func TestLoadFrom_MigrationLockTimeout(t *testing.T) {
	assert.Equal(t, 15*time.Minute, LoadFrom(func(string) string { return "" }).MigrationLockTimeout)
	set := LoadFrom(func(k string) string { return map[string]string{"MIGRATION_LOCK_TIMEOUT": "90s"}[k] })
	assert.Equal(t, 90*time.Second, set.MigrationLockTimeout)
	bad := LoadFrom(func(k string) string { return map[string]string{"MIGRATION_LOCK_TIMEOUT": "0"}[k] })
	assert.Equal(t, 15*time.Minute, bad.MigrationLockTimeout, "invalid values keep the default")
}
