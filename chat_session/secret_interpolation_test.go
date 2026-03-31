package chat_session

import (
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddDatasourceResolvesSecrets verifies that when a datasource is added to
// a chat session, the EmbedAPIKey and DBConnAPIKey fields are resolved from
// $SECRET/NAME references to their actual decrypted values.
func TestAddDatasourceResolvesSecrets(t *testing.T) {
	os.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	defer os.Unsetenv("TYK_AI_SECRET_KEY")

	db := setupTestDB(t)
	secrets.SetDBRef(db)
	service := services.NewService(db)

	// Create secrets for embed and DB connection keys
	embedSecret := &secrets.Secret{
		VarName: "TEST_EMBED_KEY",
		Value:   "sk-embed-actual-key-12345",
	}
	err := secrets.CreateSecret(db, embedSecret)
	require.NoError(t, err)

	dbConnSecret := &secrets.Secret{
		VarName: "TEST_DBCONN_KEY",
		Value:   "dbconn-actual-key-67890",
	}
	err = secrets.CreateSecret(db, dbConnSecret)
	require.NoError(t, err)

	// Create a datasource with secret references
	ds := &models.Datasource{
		Name:        "Test Datasource",
		EmbedAPIKey: "$SECRET/TEST_EMBED_KEY",
		DBConnAPIKey: "$SECRET/TEST_DBCONN_KEY",
	}
	err = db.Create(ds).Error
	require.NoError(t, err)

	// Create an admin user (bypasses entitlements check)
	adminUser := &models.User{
		Email:         "admin@test.com",
		Name:          "Admin User",
		IsAdmin:       true,
		EmailVerified: true,
	}
	err = adminUser.Create(db)
	require.NoError(t, err)
	adminUID := adminUser.ID

	// Create a chat session
	chat := &models.Chat{
		LLM:         &models.LLM{Name: "Dummy LLM", Vendor: models.MOCK_VENDOR},
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
	}
	cs, err := NewChatSession(chat, ChatMessage, db, service, nil, &adminUID, &sid)
	require.NoError(t, err)

	// Add the datasource — this should resolve secrets
	err = cs.AddDatasource(ds.ID)
	require.NoError(t, err)

	// Verify the datasource was added with resolved secrets
	addedDS, ok := cs.datasources[ds.ID]
	require.True(t, ok, "Datasource should be in the session")

	assert.Equal(t, "sk-embed-actual-key-12345", addedDS.EmbedAPIKey,
		"EmbedAPIKey should be resolved from secret reference to actual value")
	assert.Equal(t, "dbconn-actual-key-67890", addedDS.DBConnAPIKey,
		"DBConnAPIKey should be resolved from secret reference to actual value")
}

// TestAddDatasourcePlainKeysUnchanged verifies that plain (non-secret-reference)
// API keys are passed through unchanged.
func TestAddDatasourcePlainKeysUnchanged(t *testing.T) {
	db := setupTestDB(t)
	secrets.SetDBRef(db)
	service := services.NewService(db)

	ds := &models.Datasource{
		Name:         "Plain Key Datasource",
		EmbedAPIKey:  "sk-plain-embed-key",
		DBConnAPIKey: "plain-dbconn-key",
	}
	err := db.Create(ds).Error
	require.NoError(t, err)

	adminUser := &models.User{
		Email:         "admin@test.com",
		Name:          "Admin User",
		IsAdmin:       true,
		EmailVerified: true,
	}
	err = adminUser.Create(db)
	require.NoError(t, err)
	adminUID := adminUser.ID

	chat := &models.Chat{
		LLM:         &models.LLM{Name: "Dummy LLM", Vendor: models.MOCK_VENDOR},
		LLMSettings: &models.LLMSettings{ModelName: "dummy"},
	}
	cs, err := NewChatSession(chat, ChatMessage, db, service, nil, &adminUID, &sid)
	require.NoError(t, err)

	err = cs.AddDatasource(ds.ID)
	require.NoError(t, err)

	addedDS := cs.datasources[ds.ID]
	assert.Equal(t, "sk-plain-embed-key", addedDS.EmbedAPIKey,
		"Plain EmbedAPIKey should be passed through unchanged")
	assert.Equal(t, "plain-dbconn-key", addedDS.DBConnAPIKey,
		"Plain DBConnAPIKey should be passed through unchanged")
}
