package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
)

// TestAPIKeysNeverExposedInResponses tests that API keys are never exposed in any API responses
// This is a comprehensive end-to-end test for the security fix
func TestAPIKeysNeverExposedInResponses(t *testing.T) {
	// Set required environment variable
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")

	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, nil)

	// Initialize secrets package with DB reference
	secrets.SetDBRef(db)

	// Create test user
	user := &models.User{
		Email:         "test@test.com",
		Name:          "Test User",
		IsAdmin:       true,
		EmailVerified: true,
		ShowPortal:    true,
		ShowChat:      true,
	}
	err := db.Create(user).Error
	assert.NoError(t, err)

	// Create a secret for testing
	secret := &secrets.Secret{
		VarName: "OPENAI_SECRET",
		Value:   "sk-secret-actual-key-123",
	}
	err = secrets.CreateSecret(db, secret)
	assert.NoError(t, err)

	t.Run("LLM_Endpoints_Never_Expose_API_Keys", func(t *testing.T) {
		// Create LLM directly in database for testing
		testLLM := &models.LLM{
			Name:             "Test OpenAI",
			APIKey:           "sk-direct-key-sensitive-123",
			APIEndpoint:      "https://api.openai.com/v1",
			PrivacyScore:     75,
			ShortDescription: "Test LLM",
			Vendor:           models.OPENAI,
			Active:           true,
			DefaultModel:     "gpt-4",
		}
		err = db.Create(testLLM).Error
		assert.NoError(t, err)

		// GET /llms - list endpoint should not expose API keys
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var listResponse map[string][]api.LLMResponse
		err = json.Unmarshal(w.Body.Bytes(), &listResponse)
		assert.NoError(t, err)
		assert.Greater(t, len(listResponse["data"]), 0)

		// Check that the LLM has redacted API key
		assert.Equal(t, "[redacted]", listResponse["data"][0].Attributes.APIKey, "LLM API key should be redacted")
		assert.True(t, listResponse["data"][0].Attributes.HasAPIKey, "HasAPIKey should be true")

		// Verify in database that the original key was preserved
		var llmInDB models.LLM
		err = db.First(&llmInDB, 1).Error
		assert.NoError(t, err)
		assert.Equal(t, "sk-direct-key-sensitive-123", llmInDB.APIKey, "Original API key should be preserved in database")
	})

	t.Run("Datasource_Endpoints_Never_Expose_API_Keys", func(t *testing.T) {
		// Create datasource directly in database for testing
		testDatasource := &models.Datasource{
			Name:             "Test Database",
			ShortDescription: "Test datasource",
			PrivacyScore:     75,
			UserID:           user.ID,
			DBConnAPIKey:     "sensitive-db-key-123",
			EmbedAPIKey:      "sensitive-embed-key-456",
			Active:           true,
		}
		err = db.Create(testDatasource).Error
		assert.NoError(t, err)

		// GET /datasources/{id} - response should not expose API keys
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/datasources/1", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]api.DatasourceResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response["data"].Attributes.DBConnAPIKey)
		assert.Equal(t, "[redacted]", response["data"].Attributes.EmbedAPIKey)
		assert.True(t, response["data"].Attributes.HasDBConnAPIKey)
		assert.True(t, response["data"].Attributes.HasEmbedAPIKey)

		// Verify in database that the original keys were preserved
		var datasourceInDB models.Datasource
		err = db.First(&datasourceInDB, 1).Error
		assert.NoError(t, err)
		assert.Equal(t, "sensitive-db-key-123", datasourceInDB.DBConnAPIKey, "Original DB API key should be preserved")
		assert.Equal(t, "sensitive-embed-key-456", datasourceInDB.EmbedAPIKey, "Original embed API key should be preserved")
	})

	t.Run("SecretReferences_Still_Work_Correctly", func(t *testing.T) {
		// Create LLM with secret reference directly in database
		llmWithSecret := &models.LLM{
			Name:             "Secret Reference LLM",
			APIKey:           "$SECRET/OPENAI_SECRET",
			APIEndpoint:      "https://api.openai.com/v1",
			PrivacyScore:     80,
			ShortDescription: "LLM using secret reference",
			Vendor:           models.OPENAI,
			Active:           true,
			DefaultModel:     "gpt-4",
		}
		err = db.Create(llmWithSecret).Error
		assert.NoError(t, err)

		// GET the LLM - response should redact the secret reference
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms/2", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]api.LLMResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response["data"].Attributes.APIKey, "Secret reference should be redacted in response")
		assert.True(t, response["data"].Attributes.HasAPIKey, "HasAPIKey should be true for secret references")

		// Verify the secret reference is preserved in database
		var llmInDB models.LLM
		err = db.First(&llmInDB, 2).Error
		assert.NoError(t, err)
		assert.Equal(t, "$SECRET/OPENAI_SECRET", llmInDB.APIKey, "Secret reference should be preserved in database")
	})
}