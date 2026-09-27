package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// credentialErrService lets a test decide what GetCredentialBySecret answers,
// the way the microgateway's adapter answers when the control plane cannot be
// reached (or says the App is inactive). Everything else is the real service.
type credentialErrService struct {
	*services.Service
	credErr error
}

func (s *credentialErrService) GetCredentialBySecret(secret string) (*models.Credential, error) {
	if s.credErr != nil {
		return nil, s.credErr
	}
	return s.Service.GetCredentialBySecret(secret)
}

type statusFixture struct {
	srv          *httptest.Server
	svc          *credentialErrService
	grantedSlug  string
	otherSlug    string
	anthSlug     string
	bedrockSlug  string
	upstreamHits *atomic.Int64
}

// newStatusFixture builds a proxy with one App granted a single OpenAI LLM and
// three LLMs it is not granted (OpenAI, Anthropic, Bedrock). Every upstream
// points at a server that counts calls, so a test can assert a refused request
// never reached a vendor.
func newStatusFixture(t *testing.T) *statusFixture {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })

	hits := &atomic.Int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)

	base := services.NewService(db)
	svc := &credentialErrService{Service: base}
	budgetService := budget.NewService(db, services.NewTestNotificationService(db))
	p := New(svc, budgetService, &Config{Port: 9999})
	require.NotNil(t, p)

	mk := func(name string, vendor models.Vendor, endpoint string) *models.LLM {
		llm := &models.LLM{Name: name, Vendor: vendor, Active: true, APIEndpoint: endpoint, APIKey: "k", DefaultModel: "m"}
		require.NoError(t, db.Create(llm).Error)
		return llm
	}
	granted := mk("Granted GPT", models.OPENAI, upstream.URL+"/v1")
	other := mk("Other GPT", models.OPENAI, upstream.URL+"/v1")
	anth := mk("Other Claude", models.ANTHROPIC, upstream.URL)
	bedrock := mk("Other Bedrock", models.BEDROCK, "https://bedrock-runtime.us-east-1.amazonaws.com")

	user := &models.User{Email: "status@example.com"}
	require.NoError(t, db.Create(user).Error)
	cred := &models.Credential{Secret: "status-secret", Active: true}
	require.NoError(t, db.Create(cred).Error)
	app := &models.App{Name: "Status App", UserID: user.ID, CredentialID: cred.ID, IsActive: true}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, app.AddLLM(db, granted))

	require.NoError(t, p.loadResources())
	srv := httptest.NewServer(p.createHandler())
	t.Cleanup(srv.Close)

	return &statusFixture{
		srv: srv, svc: svc, upstreamHits: hits,
		grantedSlug: slug.Make(granted.Name), otherSlug: slug.Make(other.Name),
		anthSlug: slug.Make(anth.Name), bedrockSlug: slug.Make(bedrock.Name),
	}
}

func (f *statusFixture) do(t *testing.T, path string, setAuth func(*http.Request)) (int, http.Header, string) {
	t.Helper()
	model := "m"
	if strings.HasPrefix(path, "/v1/") {
		// The unified router needs a "slug/model" it can route before auth.
		model = f.grantedSlug + "/m"
	}
	body := `{"model":"` + model + `","max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	setAuth(req)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

func bearer(secret string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+secret) }
}

func xAPIKey(secret string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("x-api-key", secret) }
}

func rawAuthorization(secret string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", secret) }
}

// An App that authenticates but is not granted the target is a 403 on every
// header variant. The API-key branch used to answer 401 ("Invalid API key or
// insufficient permissions"), which tells the caller to fix a key that is fine.
func TestUngrantedTargetIsForbiddenOnEveryHeaderVariant(t *testing.T) {
	f := newStatusFixture(t)

	cases := []struct {
		name string
		path string
		auth func(*http.Request)
	}{
		{"anthropic bridge, x-api-key", "/anthropic/" + f.bedrockSlug + "/v1/messages", xAPIKey("status-secret")},
		{"anthropic bridge, bearer", "/anthropic/" + f.bedrockSlug + "/v1/messages", bearer("status-secret")},
		{"llm rest anthropic, x-api-key", "/llm/rest/" + f.anthSlug + "/v1/messages", xAPIKey("status-secret")},
		{"llm rest anthropic, bearer", "/llm/rest/" + f.anthSlug + "/v1/messages", bearer("status-secret")},
		{"llm rest openai, bearer", "/llm/rest/" + f.otherSlug + "/v1/chat/completions", bearer("status-secret")},
		{"ai route, raw authorization", "/ai/" + f.otherSlug + "/v1/chat/completions", rawAuthorization("status-secret")},
		{"ai route, bearer", "/ai/" + f.otherSlug + "/v1/chat/completions", bearer("status-secret")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := f.upstreamHits.Load()
			code, _, body := f.do(t, tc.path, tc.auth)
			assert.Equal(t, http.StatusForbidden, code, "body: %s", body)
			assert.Equal(t, before, f.upstreamHits.Load(), "a refused request must never reach the vendor")
		})
	}

	// A wrong key is still a 401 on the API-key branch.
	code, _, body := f.do(t, "/llm/rest/"+f.anthSlug+"/v1/messages", xAPIKey("wrong-secret"))
	assert.Equal(t, http.StatusUnauthorized, code, "body: %s", body)
}

// When the credential cannot be checked right now (the microgateway's control
// plane is unreachable), the answer is a retryable 503, not a 401 that clients
// read as "your key is wrong" and never retry.
func TestCredentialCheckUnavailableIsRetryable503(t *testing.T) {
	f := newStatusFixture(t)
	f.svc.credErr = fmt.Errorf("invalid token: token validation failed: %w", services.ErrCredentialCheckUnavailable)

	cases := []struct {
		name string
		path string
		auth func(*http.Request)
	}{
		{"ai route, bearer", "/ai/" + f.grantedSlug + "/v1/chat/completions", bearer("status-secret")},
		{"ai route, raw authorization", "/ai/" + f.grantedSlug + "/v1/chat/completions", rawAuthorization("status-secret")},
		{"unified v1, bearer", "/v1/chat/completions", bearer("status-secret")},
		{"llm rest, bearer", "/llm/rest/" + f.grantedSlug + "/v1/chat/completions", bearer("status-secret")},
		{"llm stream, bearer", "/llm/stream/" + f.grantedSlug + "/v1/chat/completions", bearer("status-secret")},
		{"llm rest anthropic, x-api-key", "/llm/rest/" + f.anthSlug + "/v1/messages", xAPIKey("status-secret")},
		{"anthropic bridge, x-api-key", "/anthropic/" + f.bedrockSlug + "/v1/messages", xAPIKey("status-secret")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, hdr, body := f.do(t, tc.path, tc.auth)
			require.Equal(t, http.StatusServiceUnavailable, code, "body: %s", body)
			assert.NotEmpty(t, hdr.Get("Retry-After"), "a 503 must say when to retry")
			assert.Empty(t, hdr.Get("WWW-Authenticate"), "not an authentication challenge")

			var env struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &env), "OpenAI-shaped body expected: %s", body)
			assert.NotEmpty(t, env.Error.Message)
			assert.Equal(t, "api_error", env.Error.Type)
			assert.Equal(t, "credential_check_unavailable", env.Error.Code)
		})
	}

	req, err := http.NewRequest(http.MethodGet, f.srv.URL+"/v1/models", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer status-secret")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "/v1/models too")
	assert.Equal(t, int64(0), f.upstreamHits.Load())
}

// The control plane rejecting a token as belonging to an inactive App is a 403
// "app is inactive", the answer the embedded gateway gives for the same App.
func TestInactiveAppFromCredentialCheckIsForbidden(t *testing.T) {
	f := newStatusFixture(t)
	f.svc.credErr = fmt.Errorf("invalid token: %w", services.ErrAppInactive)

	for _, auth := range []func(*http.Request){bearer("status-secret"), rawAuthorization("status-secret")} {
		code, hdr, body := f.do(t, "/ai/"+f.grantedSlug+"/v1/chat/completions", auth)
		assert.Equal(t, http.StatusForbidden, code, "body: %s", body)
		assert.Contains(t, body, "app is inactive")
		assert.Empty(t, hdr.Get("Retry-After"))
	}
}

// A plain rejection keeps its 401.
func TestRejectedCredentialStays401(t *testing.T) {
	f := newStatusFixture(t)
	f.svc.credErr = fmt.Errorf("invalid token: Invalid token")

	code, _, body := f.do(t, "/ai/"+f.grantedSlug+"/v1/chat/completions", bearer("status-secret"))
	assert.Equal(t, http.StatusUnauthorized, code, "body: %s", body)
	code, _, body = f.do(t, "/ai/"+f.grantedSlug+"/v1/chat/completions", rawAuthorization("status-secret"))
	assert.Equal(t, http.StatusUnauthorized, code, "body: %s", body)
}
