package secrets_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	// Import all store backends
	_ "github.com/TykTechnologies/midsommar/v2/secrets/all"
)

func setupCompatTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}, &secrets.EncryptionKey{}))
	return db
}

func TestCompat_SetDBRefAndCRUD(t *testing.T) {
	db := setupCompatTestDB(t)
	t.Setenv("TYK_AI_SECRET_KEY", "compat-test-key")

	secrets.SetStore(nil)
	secrets.SetDBRef(db)

	assert.NotNil(t, secrets.Store())

	// Create
	secret := &secrets.Secret{VarName: "COMPAT_KEY", Value: "compat-value"}
	require.NoError(t, secrets.CreateSecret(db, secret))

	// GetByID
	got, err := secrets.GetSecretByID(db, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "compat-value", got.Value)

	// GetByVarName
	got, err = secrets.GetSecretByVarName(db, "COMPAT_KEY", false)
	require.NoError(t, err)
	assert.Equal(t, "compat-value", got.Value)

	// Update
	got.Value = "updated-value"
	require.NoError(t, secrets.UpdateSecret(db, got))

	got2, err := secrets.GetSecretByID(db, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "updated-value", got2.Value)

	// List
	items, total, _, err := secrets.ListSecrets(db, 10, 1, true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, items, 1)

	// Delete
	require.NoError(t, secrets.DeleteSecretByID(db, secret.ID))
	_, err = secrets.GetSecretByID(db, secret.ID, false)
	assert.Error(t, err)
}

func TestCompat_EncryptDecryptValue(t *testing.T) {
	db := setupCompatTestDB(t)
	t.Setenv("TYK_AI_SECRET_KEY", "encrypt-test-key")

	secrets.SetStore(nil)
	secrets.SetDBRef(db)

	encrypted := secrets.EncryptValue("hello-world")
	assert.NotEqual(t, "hello-world", encrypted)
	assert.Contains(t, encrypted, "$ENC/")

	decrypted := secrets.DecryptValue(encrypted)
	assert.Equal(t, "hello-world", decrypted)

	assert.Equal(t, "plain", secrets.DecryptValue("plain"))
	assert.Equal(t, "", secrets.EncryptValue(""))
	assert.Equal(t, "[redacted]", secrets.EncryptValue("[redacted]"))
}

func TestCompat_GetValue(t *testing.T) {
	db := setupCompatTestDB(t)
	t.Setenv("TYK_AI_SECRET_KEY", "getvalue-test-key")

	secrets.SetStore(nil)
	secrets.SetDBRef(db)

	secret := &secrets.Secret{VarName: "MY_API_KEY", Value: "sk-12345"}
	require.NoError(t, secrets.CreateSecret(db, secret))

	val := secrets.GetValue("$SECRET/MY_API_KEY", false)
	assert.Equal(t, "sk-12345", val)

	val = secrets.GetValue("$SECRET/MY_API_KEY", true)
	assert.Equal(t, "$SECRET/MY_API_KEY", val)

	t.Setenv("MY_ENV", "env-val")
	val = secrets.GetValue("$ENV/MY_ENV", false)
	assert.Equal(t, "env-val", val)

	val = secrets.GetValue("not-a-reference", false)
	assert.Equal(t, "not-a-reference", val)
}

func TestCompat_GetOrCreateDefaultSecrets(t *testing.T) {
	db := setupCompatTestDB(t)
	t.Setenv("TYK_AI_SECRET_KEY", "defaults-test-key")

	secrets.SetStore(nil)
	secrets.SetDBRef(db)

	require.NoError(t, secrets.GetOrCreateDefaultSecrets(db))

	_, err := secrets.GetSecretByVarName(db, "OPENAI_KEY", false)
	require.NoError(t, err)

	_, err = secrets.GetSecretByVarName(db, "ANTHROPIC_KEY", false)
	require.NoError(t, err)
}

func TestCompat_NoStoreInitialized(t *testing.T) {
	secrets.SetStore(nil)

	err := secrets.CreateSecret(nil, &secrets.Secret{VarName: "X", Value: "Y"})
	assert.Error(t, err)

	_, err = secrets.GetSecretByID(nil, 1, false)
	assert.Error(t, err)

	_, err = secrets.GetSecretByVarName(nil, "X", false)
	assert.Error(t, err)

	err = secrets.UpdateSecret(nil, &secrets.Secret{})
	assert.Error(t, err)

	err = secrets.DeleteSecretByID(nil, 1)
	assert.Error(t, err)

	_, _, _, err = secrets.ListSecrets(nil, 10, 1, false)
	assert.Error(t, err)

	err = secrets.GetOrCreateDefaultSecrets(nil)
	assert.Error(t, err)
}
