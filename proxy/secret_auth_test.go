package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestSecretReferenceInAuthHeader(t *testing.T) {
	// Set required environment variable for secrets encryption
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")

	// Setup test environment
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	// Initialize services
	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	// Initialize secrets
	secrets.SetDBRef(db)

	// Create a credential
	cred := &models.Credential{
		Secret: "test-cred-secret",
		Active: true,
	}
	require.NoError(t, db.Create(cred).Error)

	// Create a test app with the credential
	app := &models.App{
		Name:         "Test App",
		CredentialID: cred.ID,
	}
	require.NoError(t, db.Create(app).Error)

	// Setup a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the secret was properly resolved in auth header
		assert.Equal(t, "Bearer sk-test-key-123", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Return a mock response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"completion": "test response"}`))
	}))
	defer server.Close()

	// Create secrets directly in DB
	apiKeySecret := &secrets.Secret{
		VarName: "TEST_API_KEY",
		Value:   "sk-test-key-123",
	}
	require.NoError(t, secrets.CreateSecret(db, apiKeySecret))

	apiEndpointSecret := &secrets.Secret{
		VarName: "TEST_API_ENDPOINT",
		Value:   server.URL,
	}
	require.NoError(t, secrets.CreateSecret(db, apiEndpointSecret))

	// Create LLM directly in DB that uses secret references
	llm := &models.LLM{
		Name:         "Test LLM",
		APIKey:       "$SECRET/TEST_API_KEY",      // Using secret reference for API key
		APIEndpoint:  "$SECRET/TEST_API_ENDPOINT", // Using secret reference for endpoint
		Vendor:       models.MOCK_VENDOR,
		Active:       true,
		DefaultModel: "test-model",
	}
	require.NoError(t, db.Create(llm).Error)

	// Associate LLM with the app
	require.NoError(t, app.AddLLM(db, llm))

	// Create proxy with all required services
	proxy := NewProxy(service, &Config{Port: 8080}, budgetService)
	require.NoError(t, proxy.loadResources())

	// Create test request body
	reqBody := map[string]interface{}{
		"model":    "test-model",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// Create test request with correct slug path
	llmSlug := slug.Make(llm.Name) // Generate same slug as proxy
	req := httptest.NewRequest("POST", "/llm/rest/"+llmSlug+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", cred.Secret) // Add credential
	req = req.WithContext(context.WithValue(req.Context(), "app", app))

	// Create response recorder
	w := httptest.NewRecorder()

	// Use the proxy's router to handle the request
	handler := proxy.createHandler()
	handler.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	if w.Code != http.StatusOK {
		t.Logf("Response body: %s", w.Body.String())
	}
}
