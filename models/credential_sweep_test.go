package models

import (
	"crypto/fips140"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawColumn(t *testing.T, db *gorm.DB, table, column string, id any) string {
	t.Helper()
	var v string
	require.NoError(t, db.Session(&gorm.Session{NewDB: true}).Table(table).
		Select("CAST("+column+" AS TEXT)").Where("id = ?", id).Row().Scan(&v))
	return v
}

func encVersion(s string) string {
	switch {
	case strings.HasPrefix(s, "$ENC/v3:"):
		return "v3"
	case strings.HasPrefix(s, "$ENC/v2:"):
		return "v2"
	case strings.HasPrefix(s, "$ENC/"):
		return "legacy"
	}
	return "plain"
}

// In FIPS mode the sweep moves the $ENC/ values of other tables (here an
// LLM's api_key and credential metadata, and a webhook's signing secret) to
// v3; reads still return the plaintext, and a second run changes nothing.
func TestReencryptStoredCredentials_FIPSMode(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	testReencryptStoredCredentials(t, setupTestDB(t))
}

// The same on Postgres (DATABASE_URL), where metadata is a json column.
func TestReencryptStoredCredentials_FIPSMode_PostgreSQL(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	db := setupPostgreSQLTestDB(t)
	// Rows other tests left behind were written under other keys; start clean.
	require.NoError(t, db.Exec("DELETE FROM llms").Error)
	require.NoError(t, db.Exec("DELETE FROM webhook_targets").Error)
	testReencryptStoredCredentials(t, db)
}

func testReencryptStoredCredentials(t *testing.T, db *gorm.DB) {
	if fips140.Enabled() {
		t.Skip("writes v2 first, as a standard build")
	}

	l := bedrockLLM("Bedrock")
	require.NoError(t, l.Create(db))
	w := &WebhookTarget{Name: "hook", URL: "https://example.com/hook", SigningSecret: "whsec-unit-test-signing"}
	require.NoError(t, db.Create(w).Error)

	require.Equal(t, "v2", encVersion(rawColumn(t, db, "llms", "api_key", l.ID)))
	require.Equal(t, "v2", encVersion(rawColumn(t, db, "webhook_targets", "signing_secret", w.ID)))

	// User text that merely contains "$ENC/" is never touched: a free-text
	// metadata value, and a scalar column holding a token inside longer text.
	const note = "note: $ENC/AAAAAAAAAAAAAAAAAAAAAAAAAAAA end"
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(rawColumn(t, db, "llms", "metadata", l.ID)), &meta))
	meta["note"] = note
	metaJSON, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE llms SET metadata = ? WHERE id = ?", string(metaJSON), l.ID).Error)
	const header = "X-Note: $ENC/AAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	require.NoError(t, db.Exec("UPDATE webhook_targets SET headers = ? WHERE id = ?", header, w.ID).Error)

	n, err := ReencryptStoredCredentials(db)
	require.NoError(t, err)
	assert.Zero(t, n, "standard builds do not sweep")

	secrets.ForceFIPSModeForTest(t)
	n, err = ReencryptStoredCredentials(db)
	require.NoError(t, err)
	assert.Equal(t, 5, n, "api_key, two metadata credentials, the LLM's resource signing secret, the webhook signing secret")
	assert.Equal(t, "v3", encVersion(rawColumn(t, db, "llms", "resource_signing_secret", l.ID)))

	assert.Equal(t, "v3", encVersion(rawColumn(t, db, "llms", "api_key", l.ID)))
	assert.Equal(t, "v3", encVersion(rawColumn(t, db, "webhook_targets", "signing_secret", w.ID)))
	raw := rawLLM(t, db, l.ID)
	assert.Equal(t, "v3", encVersion(raw.Metadata["aws_secret_access_key"].(string)))
	assert.Equal(t, "$SECRET/AWS_SESSION", raw.Metadata["aws_session_token"], "references are left alone")
	assert.Equal(t, note, raw.Metadata["note"], "free text containing a token is left alone")
	assert.Equal(t, header, rawColumn(t, db, "webhook_targets", "headers", w.ID), "a token inside longer text is left alone")

	var got LLM
	require.NoError(t, db.First(&got, l.ID).Error)
	assert.Equal(t, fakeAPIKey, got.APIKey)
	assert.Equal(t, fakeSecretKey, got.Metadata["aws_secret_access_key"])
	var gw WebhookTarget
	require.NoError(t, db.First(&gw, w.ID).Error)
	assert.Equal(t, "whsec-unit-test-signing", gw.SigningSecret)

	n, err = ReencryptStoredCredentials(db)
	require.NoError(t, err)
	assert.Zero(t, n, "idempotent")
}

// A value changed between the sweep's read and its write keeps the change.
func TestReencryptStoredCredentials_DoesNotRevertAConcurrentChange(t *testing.T) {
	if fips140.Enabled() {
		t.Skip("writes v2 first, as a standard build")
	}
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	db := setupTestDB(t)
	l := bedrockLLM("Bedrock")
	require.NoError(t, l.Create(db))
	stale := rawColumn(t, db, "llms", "api_key", l.ID)

	changed := secrets.EncryptValue("sk-rotated-meanwhile")
	require.NoError(t, db.Session(&gorm.Session{NewDB: true}).Table("llms").Where("id = ?", l.ID).Update("api_key", changed).Error)

	secrets.ForceFIPSModeForTest(t)
	updated, ok := secrets.ReencryptEncValue(stale)
	require.True(t, ok)
	res := db.Exec("UPDATE llms SET api_key = ? WHERE id = ? AND CAST(api_key AS TEXT) = ?", updated, l.ID, stale)
	require.NoError(t, res.Error)
	assert.Zero(t, res.RowsAffected, "the compare-and-swap the sweep uses refuses a stale row")
	assert.Equal(t, changed, rawColumn(t, db, "llms", "api_key", l.ID))
}
