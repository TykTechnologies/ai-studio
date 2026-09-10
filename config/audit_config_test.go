package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func clearAuditEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AUDIT_ENABLED", "AUDIT_STORE_TYPE", "AUDIT_FILE_PATH", "AUDIT_FILE_FORMAT",
		"AUDIT_DETAILED_RECORDING", "AUDIT_RECORD_READS", "AUDIT_RETENTION_DAYS",
		"AUDIT_MAX_BODY_BYTES", "AUDIT_QUEUE_SIZE", "AUDIT_REDACT_KEYS", "AUDIT_REDACT_HEADERS",
	} {
		t.Setenv(k, "")
	}
}

func TestGetAuditConfig_Defaults(t *testing.T) {
	clearAuditEnv(t)
	cfg := getAuditConfig()

	assert.True(t, cfg.Enabled)
	assert.Equal(t, AuditStoreDB, cfg.StoreType)
	assert.Equal(t, "./data/audit/audit.log", cfg.FilePath)
	assert.Equal(t, "json", cfg.FileFormat)
	assert.False(t, cfg.DetailedRecording)
	assert.False(t, cfg.RecordReads)
	assert.Equal(t, 90, cfg.RetentionDays)
	assert.Equal(t, 64*1024, cfg.MaxBodyBytes)
	assert.Equal(t, 4096, cfg.QueueSize)
	assert.Empty(t, cfg.RedactKeys)
	assert.Empty(t, cfg.RedactHeaders)
	assert.True(t, cfg.StoresToDB())
	assert.False(t, cfg.StoresToFile())
}

func TestGetAuditConfig_Overrides(t *testing.T) {
	clearAuditEnv(t)
	t.Setenv("AUDIT_ENABLED", "false")
	t.Setenv("AUDIT_STORE_TYPE", "BOTH")
	t.Setenv("AUDIT_FILE_PATH", "/var/log/studio/audit.log")
	t.Setenv("AUDIT_FILE_FORMAT", "text")
	t.Setenv("AUDIT_DETAILED_RECORDING", "true")
	t.Setenv("AUDIT_RECORD_READS", "1")
	t.Setenv("AUDIT_RETENTION_DAYS", "0")
	t.Setenv("AUDIT_MAX_BODY_BYTES", "1024")
	t.Setenv("AUDIT_QUEUE_SIZE", "10")
	t.Setenv("AUDIT_REDACT_KEYS", " SSN, customer_ref ,,")
	t.Setenv("AUDIT_REDACT_HEADERS", "X-Tenant-Key")

	cfg := getAuditConfig()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, AuditStoreBoth, cfg.StoreType)
	assert.True(t, cfg.StoresToDB())
	assert.True(t, cfg.StoresToFile())
	assert.Equal(t, "/var/log/studio/audit.log", cfg.FilePath)
	assert.Equal(t, "text", cfg.FileFormat)
	assert.True(t, cfg.DetailedRecording)
	assert.True(t, cfg.RecordReads)
	assert.Equal(t, 0, cfg.RetentionDays, "0 means keep forever and must be accepted")
	assert.Equal(t, 1024, cfg.MaxBodyBytes)
	assert.Equal(t, 10, cfg.QueueSize)
	assert.Equal(t, []string{"ssn", "customer_ref"}, cfg.RedactKeys, "trimmed, lower-cased, empties dropped")
	assert.Equal(t, []string{"x-tenant-key"}, cfg.RedactHeaders)
}

func TestGetAuditConfig_InvalidValuesFallBack(t *testing.T) {
	clearAuditEnv(t)
	t.Setenv("AUDIT_ENABLED", "maybe")
	t.Setenv("AUDIT_STORE_TYPE", "s3")
	t.Setenv("AUDIT_FILE_FORMAT", "xml")
	t.Setenv("AUDIT_RETENTION_DAYS", "-5")
	t.Setenv("AUDIT_MAX_BODY_BYTES", "0")
	t.Setenv("AUDIT_QUEUE_SIZE", "lots")

	cfg := getAuditConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, AuditStoreDB, cfg.StoreType)
	assert.Equal(t, "json", cfg.FileFormat)
	assert.Equal(t, 90, cfg.RetentionDays)
	assert.Equal(t, 64*1024, cfg.MaxBodyBytes)
	assert.Equal(t, 4096, cfg.QueueSize)
}

func TestSplitCSVList(t *testing.T) {
	assert.Nil(t, splitCSVList(""))
	assert.Nil(t, splitCSVList(" , ,"))
	assert.Equal(t, []string{"a", "b c", "d"}, splitCSVList("A, b c ,D"))
}
