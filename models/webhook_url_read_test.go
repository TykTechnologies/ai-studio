package models

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Part A of storing webhook target URLs encrypted (o4a.21): this release
// reads a plain or an encrypted url back as the real URL, and still stores
// it in clear, so a later release can turn encryption on without breaking
// readers on this one.
func TestWebhookTargetURL_ReadsPlainAndEncrypted(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "webhook-url-read-test-key")
	db := setupTestDB(t)
	const hook = "https://hooks.slack.com/services/T0123/B0456/xYz789secretTokenValue"

	plain := &WebhookTarget{ID: "t-plain", Name: "plain", URL: hook}
	require.NoError(t, db.Create(plain).Error)
	var raw string
	require.NoError(t, db.Table("webhook_targets").Select("url").Where("id = ?", "t-plain").Row().Scan(&raw))
	assert.True(t, strings.HasPrefix(raw, "$ENC/"), "saving stores the url encrypted")
	assert.NotContains(t, raw, "secretTokenValue")

	enc := &WebhookTarget{ID: "t-enc", Name: "enc", URL: hook}
	require.NoError(t, db.Create(enc).Error)
	ciphertext := secrets.EncryptValue(hook)
	require.True(t, strings.HasPrefix(ciphertext, "$ENC/"))
	require.NoError(t, db.Exec("UPDATE webhook_targets SET url = ? WHERE id = ?", ciphertext, "t-enc").Error)
	// A row written by 2.3.0, in clear.
	require.NoError(t, db.Exec("UPDATE webhook_targets SET url = ? WHERE id = ?", hook, "t-plain").Error)

	for _, id := range []string{"t-plain", "t-enc"} {
		var got WebhookTarget
		require.NoError(t, db.Where("id = ?", id).Take(&got).Error)
		assert.Equal(t, hook, got.URL, id)
	}

	// A url longer than the old varchar(2048) limit, as its ciphertext is.
	long := "https://receiver.example.com/" + strings.Repeat("a", 3000)
	require.NoError(t, db.Create(&WebhookTarget{ID: "t-long", Name: "long", URL: long}).Error)
}
