package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	_ "github.com/TykTechnologies/midsommar/v2/secrets/local" // Register local KEK provider
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

	// Initialize secrets store on the service
	secretStore, err := secrets.New(db, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	service.SetSecretStore(secretStore)

	// Create a secret directly in the database
	secret := &secrets.Secret{
		VarName: "OPENAI_KEY",
		Value:   "sk-test-key-123",
	}
	err = service.Secrets.Create(context.Background(), secret)
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

func TestSerializeLLMRedactsAPIKey(t *testing.T) {
	// Set required environment variable
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")

	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	// Initialize secrets store on the service
	secretStore, err := secrets.New(db, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	service.SetSecretStore(secretStore)

	a := NewAPI(service, true, authService, config, nil, apitest.EmptyFile, nil)

	// Test 1: LLM with direct API key
	llmWithDirectKey := &models.LLM{
		Name:             "Direct Key LLM",
		APIKey:           "sk-direct-key-123",
		APIEndpoint:      "https://api.openai.com/v1",
		PrivacyScore:     75,
		ShortDescription: "LLM with direct API key",
		Vendor:           models.OPENAI,
		Active:           true,
		DefaultModel:     "gpt-4",
	}

	// Test 2: LLM with secret reference
	secret := &secrets.Secret{
		VarName: "TEST_KEY",
		Value:   "sk-secret-key-456",
	}
	err = service.Secrets.Create(context.Background(), secret)
	assert.NoError(t, err)

	llmWithSecretRef := &models.LLM{
		Name:             "Secret Ref LLM",
		APIKey:           "$SECRET/TEST_KEY",
		APIEndpoint:      "https://api.anthropic.com/v1",
		PrivacyScore:     80,
		ShortDescription: "LLM with secret reference",
		Vendor:           models.ANTHROPIC,
		Active:           true,
		DefaultModel:     "claude-3-sonnet",
	}

	// Test 3: LLM with empty API key
	llmWithEmptyKey := &models.LLM{
		Name:             "Empty Key LLM",
		APIKey:           "",
		APIEndpoint:      "https://api.ollama.com",
		PrivacyScore:     90,
		ShortDescription: "LLM with no API key",
		Vendor:           models.OLLAMA,
		Active:           true,
		DefaultModel:     "llama2",
	}

	// Store LLMs in database to test via actual API endpoints
	err = db.Create(llmWithDirectKey).Error
	assert.NoError(t, err)
	err = db.Create(llmWithSecretRef).Error
	assert.NoError(t, err)
	err = db.Create(llmWithEmptyKey).Error
	assert.NoError(t, err)

	// Test serialization through actual API endpoints
	t.Run("DirectAPIKeyIsRedacted", func(t *testing.T) {
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms/1", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data LLMResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response.Data.Attributes.APIKey)
		assert.True(t, response.Data.Attributes.HasAPIKey)
	})

	t.Run("SecretReferenceIsRedacted", func(t *testing.T) {
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms/2", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data LLMResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response.Data.Attributes.APIKey)
		assert.True(t, response.Data.Attributes.HasAPIKey)
	})

	t.Run("EmptyAPIKeyShowsCorrectly", func(t *testing.T) {
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms/3", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data LLMResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response.Data.Attributes.APIKey)
		assert.False(t, response.Data.Attributes.HasAPIKey)
	})
}
