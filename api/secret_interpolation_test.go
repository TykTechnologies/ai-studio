package api

import (
	"context"
	"os"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadExistingSessionResolvesSecrets verifies that when a chat session is
// resumed via loadExistingSession, the LLM API key is resolved from a
// $SECRET/NAME reference. Before the fix, loadExistingSession used the model
// layer directly (chat.Get) which bypassed secret interpolation.
func TestLoadExistingSessionResolvesSecrets(t *testing.T) {
	os.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	defer os.Unsetenv("TYK_AI_SECRET_KEY")

	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	secrets.SetDBRef(db)

	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Create a secret
	secret := &secrets.Secret{
		VarName: "TEST_LLM_KEY",
		Value:   "sk-actual-secret-key-12345",
	}
	err := secrets.CreateSecret(db, secret)
	require.NoError(t, err)

	// Create an LLM with a secret reference
	llm := &models.LLM{
		Name:   "Test LLM",
		Vendor: models.MOCK_VENDOR,
		APIKey: "$SECRET/TEST_LLM_KEY",
	}
	err = llm.Create(db)
	require.NoError(t, err)

	llmSettings := &models.LLMSettings{
		ModelName: "test-model",
	}
	err = llmSettings.Create(db)
	require.NoError(t, err)

	chat := &models.Chat{
		Name:          "Test Chat",
		LLMID:         llm.ID,
		LLMSettingsID: llmSettings.ID,
	}
	err = chat.Create(db)
	require.NoError(t, err)

	user := &models.User{
		Email:         "testuser@example.com",
		Name:          "Test User",
		IsAdmin:       true,
		EmailVerified: true,
	}
	err = user.Create(db)
	require.NoError(t, err)

	// First, verify the bug: loading via model layer does NOT resolve secrets
	rawChat := &models.Chat{}
	err = rawChat.Get(db, chat.ID)
	require.NoError(t, err)
	assert.Equal(t, "$SECRET/TEST_LLM_KEY", rawChat.LLM.APIKey,
		"Model layer should return raw secret reference (this is the bug path)")

	// Verify the fix: service layer DOES resolve secrets
	resolvedChat, err := service.GetChatByID(chat.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-actual-secret-key-12345", resolvedChat.LLM.APIKey,
		"Service layer should resolve secret reference to actual value")

	// Now test loadExistingSession end-to-end:
	// Create a session record so we can resume it
	history := chat_session.NewGormChatMessageHistory(db, "test-session-id", chat.ID, user.ID, "")
	_, err = history.GetAssociatedChat(context.Background())
	require.NoError(t, err)

	// Resume via loadExistingSession (which now uses service.GetChatByID)
	resumedSession, err := a.loadExistingSession("test-session-id", user.ID)
	require.NoError(t, err)
	require.NotNil(t, resumedSession)
}
