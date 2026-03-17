package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	return db
}

func insertSubmission(t *testing.T, db *gorm.DB, submitterID uint, payload models.JSONMap) *models.Submission {
	t.Helper()
	sub := &models.Submission{
		ResourceType:    models.SubmissionResourceTypeDatasource,
		Status:          models.SubmissionStatusDraft,
		SubmitterID:     submitterID,
		ResourcePayload: payload,
	}
	require.NoError(t, db.Create(sub).Error)
	return sub
}

func TestMigrateLegacySubmissions_ContextCancelledBetweenBatches(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	// Create enough submissions to span multiple batches
	for i := 0; i < 5; i++ {
		insertSubmission(t, db, 1, models.JSONMap{
			"auth_key": "plaintext-key",
		})
	}

	// Use batch size 2 and cancel after processing
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately to trigger shutdown on next batch boundary
	cancel()

	migrated, err := svc.MigrateLegacySubmissions(ctx, 2)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, migrated, "should return 0 since context was cancelled before first batch")
}

func TestMigrateLegacySubmissions_AllAlreadyPrefixed(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	insertSubmission(t, db, 1, models.JSONMap{
		"auth_key":        "$PLAIN/my-key",
		"db_conn_api_key": "$ENC/v2/1/some-encrypted",
	})
	insertSubmission(t, db, 1, models.JSONMap{
		"embed_api_key": "$PLAIN/another",
	})

	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated, "nothing to migrate when all fields are prefixed")
}

func TestMigrateLegacySubmissions_MixedFields(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	sub := insertSubmission(t, db, 1, models.JSONMap{
		"auth_key":        "unprefixed-key",         // should get $PLAIN/
		"db_conn_api_key": "$PLAIN/already-tagged",   // should be skipped
		"embed_api_key":   "$ENC/v2/1/encrypted",     // should be skipped
		"db_conn_string":  "postgres://localhost/db",  // should get $PLAIN/
		"name":            "My Datasource",            // not a credential field
	})

	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 1, migrated)

	// Verify the updated payload
	var updated models.Submission
	require.NoError(t, db.First(&updated, sub.ID).Error)
	payload := updated.ResourcePayload
	assert.Equal(t, "$PLAIN/unprefixed-key", payload["auth_key"])
	assert.Equal(t, "$PLAIN/already-tagged", payload["db_conn_api_key"])
	assert.Equal(t, "$ENC/v2/1/encrypted", payload["embed_api_key"])
	assert.Equal(t, "$PLAIN/postgres://localhost/db", payload["db_conn_string"])
	assert.Equal(t, "My Datasource", payload["name"], "non-credential fields should be unchanged")
}

func TestMigrateLegacySubmissions_EmptyCredentialFields(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	insertSubmission(t, db, 1, models.JSONMap{
		"auth_key":       "",
		"db_conn_string": "",
	})

	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated, "empty strings should not be tagged")
}

func TestMigrateLegacySubmissions_NilPayload(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	// Create submission with nil payload directly
	sub := &models.Submission{
		ResourceType: models.SubmissionResourceTypeDatasource,
		Status:       models.SubmissionStatusDraft,
		SubmitterID:  1,
	}
	require.NoError(t, db.Create(sub).Error)

	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated)
}

func TestMigrateLegacySubmissions_EmptyPayload(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	insertSubmission(t, db, 1, models.JSONMap{})

	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated)
}

func TestMigrateLegacySubmissions_BatchSizeOne(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	for i := 0; i < 3; i++ {
		insertSubmission(t, db, 1, models.JSONMap{
			"auth_key": "key-to-tag",
		})
	}

	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 3, migrated)

	// Verify all were tagged
	var subs []models.Submission
	require.NoError(t, db.Find(&subs).Error)
	for _, sub := range subs {
		assert.Equal(t, "$PLAIN/key-to-tag", sub.ResourcePayload["auth_key"])
	}
}

func TestMigrateLegacySubmissions_DefaultBatchSize(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	insertSubmission(t, db, 1, models.JSONMap{"auth_key": "key"})

	// batchSize <= 0 should default to 100
	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, 1, migrated)

	// Second run should be no-op (already prefixed)
	migrated, err = svc.MigrateLegacySubmissions(context.Background(), -5)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated, "already prefixed should be idempotent")
}

func TestMigrateLegacySubmissions_Idempotent(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	insertSubmission(t, db, 1, models.JSONMap{
		"auth_key":     "my-secret",
		"embed_api_key": "embed-key",
	})

	// First run
	migrated1, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 1, migrated1)

	// Second run — should be a no-op
	migrated2, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated2, "second run should be idempotent")
}

func TestMigrateLegacySubmissions_SequentialIdempotent_SimulatesReplicas(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	for i := 0; i < 10; i++ {
		insertSubmission(t, db, 1, models.JSONMap{
			"auth_key": "replica-key",
		})
	}

	// Simulate two replicas running migration sequentially
	migrated1, err := svc.MigrateLegacySubmissions(context.Background(), 3)
	require.NoError(t, err)
	assert.Equal(t, 10, migrated1)

	// Second "replica" runs — should be a no-op (all already $PLAIN/ prefixed)
	migrated2, err := svc.MigrateLegacySubmissions(context.Background(), 3)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated2, "second run should be no-op (idempotent)")

	// Verify no double-tagging
	var subs []models.Submission
	require.NoError(t, db.Find(&subs).Error)
	for _, sub := range subs {
		val := sub.ResourcePayload["auth_key"].(string)
		assert.Equal(t, "$PLAIN/replica-key", val, "should be tagged exactly once")
	}
}

func TestMigrateLegacySubmissions_NonStringCredentialField(t *testing.T) {
	db := setupMigrationDB(t)
	svc := NewService(db)

	// Credential field with non-string value (e.g., number or bool)
	insertSubmission(t, db, 1, models.JSONMap{
		"auth_key":     12345,
		"embed_api_key": true,
	})

	migrated, err := svc.MigrateLegacySubmissions(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated, "non-string credential values should be skipped")
}
