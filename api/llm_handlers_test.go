package api_test

import (
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
)

func TestLLMWithSecretReference(t *testing.T) {
	// Set required environment variable
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")

	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)

	// Create a test user directly in the database
	user := &models.User{
		Email:         "test@example.com",
		Name:          "Test User",
		IsAdmin:       true,
		EmailVerified: true,
		ShowPortal:    true,
		ShowChat:      true,
	}
	err := db.Create(user).Error
	assert.NoError(t, err)

	// Initialize secrets package with DB reference
	secrets.SetDBRef(db)

	// Create a secret directly in the database
	secret := &secrets.Secret{
		VarName: "OPENAI_KEY",
		Value:   "sk-test-key-123",
	}
	err = secrets.CreateSecret(db, secret)
	assert.NoError(t, err)

	// Create an LLM that references this secret directly in the database
	llm := &models.LLM{
		Name:             "OpenAI LLM",
		APIKey:           "$SECRET/OPENAI_KEY",
		APIEndpoint:      "https://api.openai.com/v1",
		PrivacyScore:     75,
		ShortDescription: "OpenAI LLM with secret key",
		LongDescription:  "OpenAI LLM using a secret reference for the API key",
		LogoURL:          "https://openai.com/logo.png",
		Vendor:           models.OPENAI,
		Active:           true,
		DefaultModel:     "gpt-4",
		AllowedModels:    []string{"gpt-4", "gpt-3.5-turbo"},
	}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	// Verify the actual secret value is used when loading into proxy
	activeLLMs, err := service.GetActiveLLMs()
	assert.NoError(t, err)
	assert.Equal(t, "sk-test-key-123", activeLLMs[0].APIKey)

	// But when getting directly via API, we preserve the reference
	llm, err = service.GetLLMByID(uint(1))
	assert.NoError(t, err)
	assert.Equal(t, "$SECRET/OPENAI_KEY", llm.APIKey)
}
