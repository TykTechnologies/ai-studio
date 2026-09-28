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

	// GET /v1/models (Claude Code model discovery) sits behind the same credential
	// middleware as POST /v1/messages. With no default_model configured, an authenticated
	// request gets 200 with an empty list, so a 200 proves auth and the app grant passed.
	otherApp := &models.App{Name: "OtherApp", UserID: user.ID}
	require.NoError(t, db.Create(otherApp).Error)
	otherCred := &models.Credential{KeyID: "other-key", Secret: "other-secret", Active: true}
	require.NoError(t, db.Create(otherCred).Error)
	otherApp.CredentialID = otherCred.ID
	require.NoError(t, db.Save(otherApp).Error) // holds no LLM
	require.NoError(t, p.loadResources())

	modelsURL := func(routeSlug string) string {
		return srv.URL + "/anthropic/" + routeSlug + "/v1/models?limit=1000"
	}
	getModels := func(t *testing.T, url string, setAuth func(*http.Request)) *http.Response {
		t.Helper()
		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)
		setAuth(req)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}
	llmModels := modelsURL(slug.Make(llm.Name))

	t.Run("models: valid x-api-key authenticates", func(t *testing.T) {
		resp := getModels(t, llmModels, func(r *http.Request) { r.Header.Set("x-api-key", "valid-secret") })
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "should pass auth and return an (empty) list")
		b, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(b), `"data":[]`)
	})

	t.Run("models: valid Bearer token authenticates", func(t *testing.T) {
		resp := getModels(t, llmModels, func(r *http.Request) { r.Header.Set("Authorization", "Bearer valid-secret") })
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Claude Code v2.1.248+ sends both headers when both resolve.
	t.Run("models: x-api-key and Bearer together authenticate", func(t *testing.T) {
		resp := getModels(t, llmModels, func(r *http.Request) {
			r.Header.Set("x-api-key", "valid-secret")
			r.Header.Set("Authorization", "Bearer valid-secret")
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("models: invalid x-api-key is rejected", func(t *testing.T) {
		resp := getModels(t, llmModels, func(r *http.Request) { r.Header.Set("x-api-key", "wrong-secret") })
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("models: missing credentials is rejected", func(t *testing.T) {
		resp := getModels(t, llmModels, func(r *http.Request) {})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("models: app not granted the LLM is forbidden", func(t *testing.T) {
		resp := getModels(t, llmModels, func(r *http.Request) { r.Header.Set("x-api-key", "other-secret") })
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	// Unknown answers like ungranted (403, not 404), so slugs cannot be enumerated.
	t.Run("models: unknown connection is forbidden, not 404", func(t *testing.T) {
		resp := getModels(t, modelsURL("no-such-llm"), func(r *http.Request) { r.Header.Set("x-api-key", "valid-secret") })
		defer resp.Body.Close()
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}
