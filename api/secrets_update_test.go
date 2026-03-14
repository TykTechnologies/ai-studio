package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateSecret_DoesNotOverwriteWithPlaceholder(t *testing.T) {
	// Set the secret key for encryption
	originalKey := os.Getenv("TYK_AI_ENCRYPTION_KEY")
	os.Setenv("TYK_AI_ENCRYPTION_KEY", "test-secret-key-for-unit-test-12345678")
	defer os.Setenv("TYK_AI_ENCRYPTION_KEY", originalKey)

	api, db := setupTestAPI(t)
	ctx := context.Background()

	// Migrate the Secret table
	db.AutoMigrate(&secrets.Secret{})

	// Create secret store
	secretStore, err := secrets.NewFromProvider(db, "test-secret-key-for-unit-test-12345678", "local", nil)
	require.NoError(t, err)
	defer secretStore.Close(ctx)

	// Create a secret with a real value
	secret := &secrets.Secret{
		VarName: "MY_API_KEY",
		Value:   "super-secret-value-123",
	}
	err = secretStore.Create(ctx, secret)
	require.NoError(t, err)

	secretID := secret.ID

	// Verify the secret was stored encrypted
	storedSecret, err := secretStore.GetByID(ctx, secretID, false)
	require.NoError(t, err)
	assert.Equal(t, "super-secret-value-123", storedSecret.Value)

	// Now simulate what the frontend does: PATCH with the placeholder reference
	// (this is what happens when the user edits only the var_name)
	updatePayload := SecretInput{}
	updatePayload.Data.Attributes.VarName = "MY_API_KEY_RENAMED"
	updatePayload.Data.Attributes.Value = "$SECRET/MY_API_KEY" // placeholder from the frontend

	body, _ := json.Marshal(updatePayload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/api/v1/secrets/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	api.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify the secret value was NOT overwritten with the placeholder
	updatedSecret, err := secretStore.GetByID(ctx, secretID, false)
	require.NoError(t, err)
	assert.Equal(t, "MY_API_KEY_RENAMED", updatedSecret.VarName)
	assert.Equal(t, "super-secret-value-123", updatedSecret.Value, "Secret value should be preserved when placeholder is sent")
}

func TestUpdateSecret_UpdatesValueWhenChanged(t *testing.T) {
	originalKey := os.Getenv("TYK_AI_ENCRYPTION_KEY")
	os.Setenv("TYK_AI_ENCRYPTION_KEY", "test-secret-key-for-unit-test-12345678")
	defer os.Setenv("TYK_AI_ENCRYPTION_KEY", originalKey)

	api, db := setupTestAPI(t)
	ctx := context.Background()
	db.AutoMigrate(&secrets.Secret{})

	secretStore, err := secrets.NewFromProvider(db, "test-secret-key-for-unit-test-12345678", "local", nil)
	require.NoError(t, err)
	defer secretStore.Close(ctx)

	// Create a secret
	secret := &secrets.Secret{
		VarName: "MY_API_KEY",
		Value:   "old-secret-value",
	}
	err = secretStore.Create(ctx, secret)
	require.NoError(t, err)

	secretID := secret.ID

	// PATCH with a new real value (not a placeholder)
	updatePayload := SecretInput{}
	updatePayload.Data.Attributes.VarName = "MY_API_KEY"
	updatePayload.Data.Attributes.Value = "new-secret-value"

	body, _ := json.Marshal(updatePayload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/api/v1/secrets/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	api.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify the secret value WAS updated
	updatedSecret, err := secretStore.GetByID(ctx, secretID, false)
	require.NoError(t, err)
	assert.Equal(t, "new-secret-value", updatedSecret.Value, "Secret value should be updated when a real value is sent")
}

func TestUpdateSecret_EmptyValueDoesNotOverwrite(t *testing.T) {
	originalKey := os.Getenv("TYK_AI_ENCRYPTION_KEY")
	os.Setenv("TYK_AI_ENCRYPTION_KEY", "test-secret-key-for-unit-test-12345678")
	defer os.Setenv("TYK_AI_ENCRYPTION_KEY", originalKey)

	api, db := setupTestAPI(t)
	ctx := context.Background()
	db.AutoMigrate(&secrets.Secret{})

	secretStore, err := secrets.NewFromProvider(db, "test-secret-key-for-unit-test-12345678", "local", nil)
	require.NoError(t, err)
	defer secretStore.Close(ctx)

	// Create a secret
	secret := &secrets.Secret{
		VarName: "MY_API_KEY",
		Value:   "real-secret",
	}
	err = secretStore.Create(ctx, secret)
	require.NoError(t, err)

	secretID := secret.ID

	// PATCH with an empty value (frontend omitted the value field)
	updatePayload := SecretInput{}
	updatePayload.Data.Attributes.VarName = "MY_API_KEY"
	updatePayload.Data.Attributes.Value = ""

	body, _ := json.Marshal(updatePayload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/api/v1/secrets/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	api.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify the secret value was NOT overwritten
	updatedSecret, err := secretStore.GetByID(ctx, secretID, false)
	require.NoError(t, err)
	assert.Equal(t, "real-secret", updatedSecret.Value, "Secret value should be preserved when empty value is sent")
}
