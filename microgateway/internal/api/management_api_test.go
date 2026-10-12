package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/gin-gonic/gin"
)

// recordingAuthProvider accepts every token as an admin token and records
// whether anything asked it to issue one.
type recordingAuthProvider struct {
	generated int
}

func (p *recordingAuthProvider) ValidateToken(string) (*auth.AuthResult, error) {
	return &auth.AuthResult{Valid: true, AppID: 1, Scopes: []string{"admin"}}, nil
}

func (p *recordingAuthProvider) GenerateToken(uint, string, []string, time.Duration) (string, error) {
	p.generated++
	return "minted", nil
}

func (p *recordingAuthProvider) RevokeToken(string) error { return nil }

func (p *recordingAuthProvider) GetTokenInfo(string) (*auth.TokenInfo, error) {
	return &auth.TokenInfo{}, nil
}

const mintAdminTokenBody = `{"app_id":1,"name":"x","scopes":["admin"]}`

// TestSetupRouter_NoPublicTokenIssuance: POST /api/v1/auth/token issued a
// token of any scope, admin included, to an unauthenticated caller. It must not
// be routed in any mode; the authenticated management API stays.
func TestSetupRouter_NoPublicTokenIssuance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &recordingAuthProvider{}
	router := SetupRouter(&RouterConfig{AuthProvider: provider, Gateway: &stubGateway{}})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", strings.NewReader(mintAdminTokenBody)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /api/v1/auth/token: expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if provider.generated != 0 {
		t.Fatalf("a token was issued to an unauthenticated caller")
	}

	// The management API itself is still mounted and still requires auth.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/tokens without a token: expected 401, got %d", w.Code)
	}
}

// TestSetupRouter_EdgeHasNoManagementAPI: an edge gateway is configured by
// the control plane only, so no /api/v1 route answers, even with a token the
// auth provider would accept, and none falls through to the AI gateway.
func TestSetupRouter_EdgeHasNoManagementAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	provider := &recordingAuthProvider{}
	stub := &stubGateway{}
	router := SetupRouter(&RouterConfig{
		AuthProvider:         provider,
		Gateway:              stub,
		DisableManagementAPI: true,
	})

	requests := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/auth/token", mintAdminTokenBody},
		{http.MethodPost, "/api/v1/auth/validate", `{"token":"t"}`},
		{http.MethodGet, "/api/v1/tokens", ""},
		{http.MethodPost, "/api/v1/tokens", mintAdminTokenBody},
		{http.MethodGet, "/api/v1/llms", ""},
		{http.MethodPost, "/api/v1/apps", `{}`},
		{http.MethodPut, "/api/v1/llms/1/plugins", `{}`},
		{http.MethodPost, "/api/v1/plugins", `{}`},
		{http.MethodPost, "/api/v1/system/reload", ""},
		{http.MethodGet, "/api/v1/analytics/events", ""},
	}
	for _, r := range requests {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			stub.gotPath = ""
			req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
			req.Header.Set("Authorization", "Bearer any")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
			}
			if stub.gotPath != "" {
				t.Fatalf("request reached the gateway handler as %q", stub.gotPath)
			}
		})
	}
	if provider.generated != 0 {
		t.Fatalf("a token was issued on an edge gateway")
	}

	// The data plane is unaffected.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/llm/rest/gpt/v1/chat/completions", strings.NewReader(`{}`)))
	if stub.gotPath != "/llm/rest/gpt/v1/chat/completions" {
		t.Fatalf("expected the LLM route to reach the gateway handler, got %q (status %d)", stub.gotPath, w.Code)
	}
}
