package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	_ "github.com/TykTechnologies/midsommar/v2/secrets/local" // Register local KEK provider
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEncryptionDB(t *testing.T) (*gorm.DB, *secrets.Store) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	store, err := secrets.New(db, "test-encryption-key")
	require.NoError(t, err)
	return db, store
}

func newEncryptionService(t *testing.T) *Service {
	t.Helper()
	db, store := setupEncryptionDB(t)
	svc := NewService(db)
	svc.Secrets = store
	return svc
}

// isEncrypted checks that a string has the $ENC/ prefix (envelope encryption)
func isEncrypted(s string) bool {
	return len(s) > 5 && s[:5] == "$ENC/"
}

func TestCreateSubmission_EncryptsCredentials(t *testing.T) {
	svc := newEncryptionService(t)

	sub, err := svc.CreateSubmission(
		1, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"name":            "Test DS",
			"auth_key":        "super-secret-key",
			"db_conn_api_key": "db-api-key",
			"embed_api_key":   "embed-key",
			"db_conn_string":  "postgres://user:pass@host/db",
		},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Returned submission should have decrypted values (for the caller)
	assert.Equal(t, "super-secret-key", sub.ResourcePayload["auth_key"])
	assert.Equal(t, "db-api-key", sub.ResourcePayload["db_conn_api_key"])
	assert.Equal(t, "embed-key", sub.ResourcePayload["embed_api_key"])
	assert.Equal(t, "postgres://user:pass@host/db", sub.ResourcePayload["db_conn_string"])
	assert.Equal(t, "Test DS", sub.ResourcePayload["name"])

	// Read directly from DB — credentials should be encrypted
	var raw models.Submission
	require.NoError(t, svc.DB.First(&raw, sub.ID).Error)
	assert.True(t, isEncrypted(raw.ResourcePayload["auth_key"].(string)), "auth_key should be encrypted in DB")
	assert.True(t, isEncrypted(raw.ResourcePayload["db_conn_api_key"].(string)), "db_conn_api_key should be encrypted in DB")
	assert.True(t, isEncrypted(raw.ResourcePayload["embed_api_key"].(string)), "embed_api_key should be encrypted in DB")
	assert.True(t, isEncrypted(raw.ResourcePayload["db_conn_string"].(string)), "db_conn_string should be encrypted in DB")
	assert.Equal(t, "Test DS", raw.ResourcePayload["name"], "non-credential fields should not be encrypted")
}

func TestGetSubmissionByID_DecryptsCredentials(t *testing.T) {
	svc := newEncryptionService(t)

	sub, err := svc.CreateSubmission(
		1, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"auth_key": "my-secret",
			"name":     "DS",
		},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Fetch by ID — should decrypt
	fetched, err := svc.GetSubmissionByID(sub.ID)
	require.NoError(t, err)
	assert.Equal(t, "my-secret", fetched.ResourcePayload["auth_key"])
	assert.Equal(t, "DS", fetched.ResourcePayload["name"])
}

func TestUpdateSubmission_EncryptsAndDecrypts(t *testing.T) {
	svc := newEncryptionService(t)

	sub, err := svc.CreateSubmission(
		1, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"auth_key": "original-key",
			"name":     "DS",
		},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Update with new credential
	updated, err := svc.UpdateSubmission(
		sub.ID, 1,
		models.JSONMap{
			"auth_key": "new-secret-key",
			"name":     "DS Updated",
		},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)
	assert.Equal(t, "new-secret-key", updated.ResourcePayload["auth_key"])

	// Verify encrypted in DB
	var raw models.Submission
	require.NoError(t, svc.DB.First(&raw, sub.ID).Error)
	assert.True(t, isEncrypted(raw.ResourcePayload["auth_key"].(string)), "updated credential should be encrypted in DB")
}

func TestUpdateSubmission_RedactedPreservesOriginal(t *testing.T) {
	svc := newEncryptionService(t)

	sub, err := svc.CreateSubmission(
		1, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"auth_key": "keep-this-secret",
			"name":     "DS",
		},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Update with [redacted] placeholder — should preserve original
	updated, err := svc.UpdateSubmission(
		sub.ID, 1,
		models.JSONMap{
			"auth_key": "[redacted]",
			"name":     "DS Renamed",
		},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// The returned value should be the original decrypted value
	assert.Equal(t, "keep-this-secret", updated.ResourcePayload["auth_key"])
	assert.Equal(t, "DS Renamed", updated.ResourcePayload["name"])
}

func TestGetSubmissionsBySubmitter_DecryptsAll(t *testing.T) {
	svc := newEncryptionService(t)

	for i := 0; i < 3; i++ {
		_, err := svc.CreateSubmission(
			42, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{"auth_key": "secret-key"},
			nil, 50, "", "", "", "", nil, "", "",
		)
		require.NoError(t, err)
	}

	subs, count, _, err := svc.GetSubmissionsBySubmitter(42, "", 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	for _, sub := range subs {
		assert.Equal(t, "secret-key", sub.ResourcePayload["auth_key"], "all submissions should be decrypted")
	}
}

func TestGetAllSubmissions_DecryptsAll(t *testing.T) {
	svc := newEncryptionService(t)

	for i := 0; i < 3; i++ {
		_, err := svc.CreateSubmission(
			1, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
			models.JSONMap{"db_conn_string": "postgres://secret"},
			nil, 50, "", "", "", "", nil, "", "",
		)
		require.NoError(t, err)
	}

	subs, count, _, err := svc.GetAllSubmissions("", "", 10, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	for _, sub := range subs {
		assert.Equal(t, "postgres://secret", sub.ResourcePayload["db_conn_string"])
	}
}

func TestCreateSubmission_NilSecretsSkipsEncryption(t *testing.T) {
	// Service without secrets configured (Secrets = nil)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	svc := NewService(db)

	sub, err := svc.CreateSubmission(
		1, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{"auth_key": "plaintext-key"},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Should be stored as plaintext since no secrets store
	var raw models.Submission
	require.NoError(t, svc.DB.First(&raw, sub.ID).Error)
	assert.Equal(t, "plaintext-key", raw.ResourcePayload["auth_key"])
}

func TestCreateSubmission_EmptyCredentialNotEncrypted(t *testing.T) {
	svc := newEncryptionService(t)

	sub, err := svc.CreateSubmission(
		1, models.SubmissionResourceTypeDatasource, models.SubmissionStatusDraft,
		models.JSONMap{
			"auth_key":       "",
			"db_conn_string": "",
		},
		nil, 50, "", "", "", "", nil, "", "",
	)
	require.NoError(t, err)

	// Empty strings should not be encrypted
	var raw models.Submission
	require.NoError(t, svc.DB.First(&raw, sub.ID).Error)
	assert.Equal(t, "", raw.ResourcePayload["auth_key"])
	assert.Equal(t, "", raw.ResourcePayload["db_conn_string"])
	_ = sub
}

func TestEncryptDecryptSubmissionPayload_RoundTrip(t *testing.T) {
	svc := newEncryptionService(t)
	ctx := context.Background()

	payload := models.JSONMap{
		"auth_key":        "secret-1",
		"db_conn_api_key": "secret-2",
		"embed_api_key":   "secret-3",
		"db_conn_string":  "secret-4",
		"name":            "not-a-credential",
	}

	svc.encryptSubmissionPayload(ctx, 42, payload)

	// All credential fields should now be encrypted
	assert.True(t, isEncrypted(payload["auth_key"].(string)))
	assert.True(t, isEncrypted(payload["db_conn_api_key"].(string)))
	assert.True(t, isEncrypted(payload["embed_api_key"].(string)))
	assert.True(t, isEncrypted(payload["db_conn_string"].(string)))
	assert.Equal(t, "not-a-credential", payload["name"])

	svc.decryptSubmissionPayload(ctx, payload)

	// All credential fields should be decrypted back
	assert.Equal(t, "secret-1", payload["auth_key"])
	assert.Equal(t, "secret-2", payload["db_conn_api_key"])
	assert.Equal(t, "secret-3", payload["embed_api_key"])
	assert.Equal(t, "secret-4", payload["db_conn_string"])
	assert.Equal(t, "not-a-credential", payload["name"])
}

func TestEncryptSubmissionPayload_NonStringFieldsSkipped(t *testing.T) {
	svc := newEncryptionService(t)
	ctx := context.Background()

	payload := models.JSONMap{
		"auth_key":       12345,
		"embed_api_key":  true,
		"db_conn_string": nil,
	}

	svc.encryptSubmissionPayload(ctx, 0, payload)

	// Non-string values should be unchanged
	assert.Equal(t, 12345, payload["auth_key"])
	assert.Equal(t, true, payload["embed_api_key"])
	assert.Nil(t, payload["db_conn_string"])
}

func TestEncryptSubmissionPayload_NilPayload(t *testing.T) {
	svc := newEncryptionService(t)
	ctx := context.Background()

	// Should not panic
	svc.encryptSubmissionPayload(ctx, 0, nil)
	svc.decryptSubmissionPayload(ctx, nil)
}
