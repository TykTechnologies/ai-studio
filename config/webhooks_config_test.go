package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func clearWebhooksEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"WEBHOOKS_ENABLED", "WEBHOOKS_WORKER_ENABLED", "WEBHOOKS_WORKER_COUNT",
		"WEBHOOKS_MAX_ATTEMPTS", "WEBHOOKS_BASE_BACKOFF", "WEBHOOKS_MAX_BACKOFF",
		"WEBHOOKS_REQUEST_TIMEOUT", "WEBHOOKS_ALLOW_INTERNAL_TARGETS",
		"WEBHOOKS_ALLOWED_HOSTS", "WEBHOOKS_DENIED_HOSTS", "WEBHOOKS_RETENTION_DAYS",
		"WEBHOOKS_DEAD_LETTER_RETENTION_DAYS", "WEBHOOKS_MAX_RESPONSE_SNIPPET_BYTES",
		"WEBHOOKS_REQUIRE_DIFFERENT_APPROVER", "WEBHOOKS_SECRET_ROTATION_GRACE",
		"WEBHOOKS_REDACT_KEYS", "WEBHOOKS_AUDIT_DELIVERIES", "WEBHOOKS_SHUTDOWN_DRAIN_TIMEOUT",
	} {
		t.Setenv(k, "")
	}
}

func TestGetWebhooksConfig_Defaults(t *testing.T) {
	clearWebhooksEnv(t)
	cfg := getWebhooksConfig()

	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.WorkerEnabled)
	assert.Equal(t, 4, cfg.WorkerCount)
	assert.Equal(t, 10, cfg.MaxAttempts)
	assert.Equal(t, 5*time.Second, cfg.BaseBackoff)
	assert.Equal(t, time.Hour, cfg.MaxBackoff)
	assert.Equal(t, 10*time.Second, cfg.RequestTimeout)
	assert.False(t, cfg.AllowInternalTargets, "internal targets must be blocked by default")
	assert.Empty(t, cfg.AllowedHosts)
	assert.Empty(t, cfg.DeniedHosts)
	assert.Equal(t, 14, cfg.RetentionDays)
	assert.Equal(t, 30, cfg.DeadLetterRetentionDays)
	assert.Equal(t, 4096, cfg.MaxResponseSnippetBytes)
	assert.False(t, cfg.RequireDifferentApprover)
	assert.Equal(t, 24*time.Hour, cfg.SecretRotationGrace)
	assert.Empty(t, cfg.RedactKeys)
	assert.False(t, cfg.AuditDeliveries)
	assert.Equal(t, 15*time.Second, cfg.ShutdownDrainTimeout)
}

func TestGetWebhooksConfig_Overrides(t *testing.T) {
	clearWebhooksEnv(t)
	t.Setenv("WEBHOOKS_ENABLED", "false")
	t.Setenv("WEBHOOKS_WORKER_ENABLED", "false")
	t.Setenv("WEBHOOKS_WORKER_COUNT", "8")
	t.Setenv("WEBHOOKS_MAX_ATTEMPTS", "3")
	t.Setenv("WEBHOOKS_BASE_BACKOFF", "1s")
	t.Setenv("WEBHOOKS_MAX_BACKOFF", "2m")
	t.Setenv("WEBHOOKS_REQUEST_TIMEOUT", "30s")
	t.Setenv("WEBHOOKS_ALLOW_INTERNAL_TARGETS", "true")
	t.Setenv("WEBHOOKS_ALLOWED_HOSTS", "Hooks.Example.com, .internal.example.com")
	t.Setenv("WEBHOOKS_DENIED_HOSTS", "evil.example.com")
	t.Setenv("WEBHOOKS_RETENTION_DAYS", "0")
	t.Setenv("WEBHOOKS_DEAD_LETTER_RETENTION_DAYS", "7")
	t.Setenv("WEBHOOKS_MAX_RESPONSE_SNIPPET_BYTES", "1024")
	t.Setenv("WEBHOOKS_REQUIRE_DIFFERENT_APPROVER", "true")
	t.Setenv("WEBHOOKS_SECRET_ROTATION_GRACE", "1h")
	t.Setenv("WEBHOOKS_REDACT_KEYS", "SSN,Iban")
	t.Setenv("WEBHOOKS_AUDIT_DELIVERIES", "true")
	t.Setenv("WEBHOOKS_SHUTDOWN_DRAIN_TIMEOUT", "3s")

	cfg := getWebhooksConfig()

	assert.False(t, cfg.Enabled)
	assert.False(t, cfg.WorkerEnabled)
	assert.Equal(t, 8, cfg.WorkerCount)
	assert.Equal(t, 3, cfg.MaxAttempts)
	assert.Equal(t, time.Second, cfg.BaseBackoff)
	assert.Equal(t, 2*time.Minute, cfg.MaxBackoff)
	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.True(t, cfg.AllowInternalTargets)
	assert.Equal(t, []string{"hooks.example.com", ".internal.example.com"}, cfg.AllowedHosts)
	assert.Equal(t, []string{"evil.example.com"}, cfg.DeniedHosts)
	assert.Equal(t, 0, cfg.RetentionDays)
	assert.Equal(t, 7, cfg.DeadLetterRetentionDays)
	assert.Equal(t, 1024, cfg.MaxResponseSnippetBytes)
	assert.True(t, cfg.RequireDifferentApprover)
	assert.Equal(t, time.Hour, cfg.SecretRotationGrace)
	assert.Equal(t, []string{"ssn", "iban"}, cfg.RedactKeys)
	assert.True(t, cfg.AuditDeliveries)
	assert.Equal(t, 3*time.Second, cfg.ShutdownDrainTimeout)
}

func TestGetWebhooksConfig_InvalidValuesFallBack(t *testing.T) {
	clearWebhooksEnv(t)
	t.Setenv("WEBHOOKS_WORKER_COUNT", "0")
	t.Setenv("WEBHOOKS_MAX_ATTEMPTS", "abc")
	t.Setenv("WEBHOOKS_REQUEST_TIMEOUT", "5m") // above the 60s cap
	t.Setenv("WEBHOOKS_BASE_BACKOFF", "10m")
	t.Setenv("WEBHOOKS_MAX_BACKOFF", "1m") // below base
	t.Setenv("WEBHOOKS_MAX_RESPONSE_SNIPPET_BYTES", "-1")
	t.Setenv("WEBHOOKS_ENABLED", "maybe")

	cfg := getWebhooksConfig()

	assert.Equal(t, 4, cfg.WorkerCount)
	assert.Equal(t, 10, cfg.MaxAttempts)
	assert.Equal(t, 10*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 10*time.Minute, cfg.BaseBackoff)
	assert.Equal(t, 10*time.Minute, cfg.MaxBackoff, "max backoff is raised to the base value")
	assert.Equal(t, 4096, cfg.MaxResponseSnippetBytes)
	assert.True(t, cfg.Enabled)
}
