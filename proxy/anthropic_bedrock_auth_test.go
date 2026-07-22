package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// TestAnthropicBridgeAuth exercises the credential-validator wiring for the
// /anthropic/{routeId}/v1/messages route end to end (through createHandler), covering
// both the x-api-key and Authorization: Bearer app-secret paths.
//
// The Bedrock LLM is configured with no default_model on purpose: a successfully
// authenticated request stops at model resolution (HTTP 400) *before* any AWS call,
// so authentication can be asserted in isolation without a live Bedrock backend.
func TestAnthropicBridgeAuth(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	p := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, p)

	llm := &models.LLM{
		Name:        "AWS Bedrock",
		Vendor:      models.BEDROCK,
		Active:      true,
		APIEndpoint: "https://bedrock-runtime.us-east-1.amazonaws.com",
		// DefaultModel intentionally empty (see doc comment).
	}
	require.NoError(t, db.Create(llm).Error)

	user := &models.User{Email: "test@example.com"}
	require.NoError(t, db.Create(user).Error)

	app := &models.App{Name: "TestApp", UserID: user.ID}
	require.NoError(t, db.Create(app).Error)

	cred := &models.Credential{Secret: "valid-secret", Active: true}
	require.NoError(t, db.Create(cred).Error)
	app.CredentialID = cred.ID
	require.NoError(t, db.Save(app).Error)
	require.NoError(t, app.AddLLM(db, llm))

	require.NoError(t, p.loadResources())

	srv := httptest.NewServer(p.createHandler())
	defer srv.Close()

	url := srv.URL + "/anthropic/" + slug.Make(llm.Name) + "/v1/messages"
	body := `{"model":"claude","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`

	do := func(t *testing.T, setAuth func(*http.Request)) *http.Response {
		t.Helper()
		req, err := http.NewRequest("POST", url, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	// Valid credentials must pass auth and reach model resolution (400), never an auth
	// rejection (401/403).
	t.Run("valid x-api-key authenticates", func(t *testing.T) {
		resp := do(t, func(r *http.Request) { r.Header.Set("x-api-key", "valid-secret") })
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "should pass auth and stop at model resolution")
		b, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(b), "default_model")
	})

	t.Run("valid Bearer token authenticates", func(t *testing.T) {
		resp := do(t, func(r *http.Request) { r.Header.Set("Authorization", "Bearer valid-secret") })
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "should pass auth and stop at model resolution")
	})

	t.Run("invalid x-api-key is rejected", func(t *testing.T) {
		resp := do(t, func(r *http.Request) { r.Header.Set("x-api-key", "wrong-secret") })
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("missing credentials is rejected", func(t *testing.T) {
		resp := do(t, func(r *http.Request) {})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}
