package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/oauthscope"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// The OAuth branch of the credential validator authenticated a bearer token and
// then forwarded the request without resolving a tool or loading an app. The MCP
// handlers resolve the tool from the URL themselves, so any valid access token
// reached any tool's MCP server - tools/list enumerated it and tools/call ran it.
// These tests pin the ACL: a token authorises the tools of the app it was bound
// to at consent, and nothing else.

// oauthACLFixture is a two-tenant world: user A owns app A which grants tool A,
// user B owns app B which grants tool B. Neither app grants the other's tool.
type oauthACLFixture struct {
	db      *gorm.DB
	service *services.Service
	proxy   *Proxy
	handler http.Handler

	userA, userB *models.User
	appA, appB   *models.App
	toolA, toolB *models.Tool
}

func newOAuthACLFixture(t *testing.T, db *gorm.DB) *oauthACLFixture {
	t.Helper()

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)

	f := &oauthACLFixture{db: db, service: service}

	f.userA = createACLUser(t, service, "owner-a@example.com", "ownera")
	f.userB = createACLUser(t, service, "owner-b@example.com", "ownerb")

	// Each tool points at its own upstream so a successful tools/call is
	// attributable, and a leak across tenants would be visible.
	upstreamA, teardownA := newMockServer(t, &mockServerConfig{
		ResponseStatus: http.StatusOK,
		ResponseBody:   `{"tenant": "a"}`,
	})
	t.Cleanup(teardownA)
	upstreamB, teardownB := newMockServer(t, &mockServerConfig{
		ResponseStatus: http.StatusOK,
		ResponseBody:   `{"tenant": "b"}`,
	})
	t.Cleanup(teardownB)

	f.toolA = createACLTool(t, service, "ACL Tool A", upstreamA.URL)
	f.toolB = createACLTool(t, service, "ACL Tool B", upstreamB.URL)

	f.appA = createACLApp(t, service, "ACL App A", f.userA.ID, f.toolA.ID)
	f.appB = createACLApp(t, service, "ACL App B", f.userB.ID, f.toolB.ID)

	p := NewProxy(service, &Config{Port: 9997}, budgetService)
	require.NoError(t, p.loadResources())
	f.proxy = p
	// Handler() is what the microgateway mounts in production; it wraps
	// createHandler with fixDoubleSlash.
	f.handler = p.Handler()

	return f
}

func createACLUser(t *testing.T, service *services.Service, email, name string) *models.User {
	t.Helper()
	user, err := service.CreateUser(services.UserDTO{
		Email:    email,
		Name:     name,
		Password: "password123",
		ShowChat: true,
	})
	require.NoError(t, err)
	require.NotNil(t, user)
	return user
}

func createACLTool(t *testing.T, service *services.Service, name, upstreamURL string) *models.Tool {
	t.Helper()

	rawSpec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":%q,"version":"1.0.0"},"servers":[{"url":%q}],"paths":{"/test":{"get":{"operationId":"getTestData","summary":"Get Test Data","description":"Retrieves test data.","responses":{"200":{"description":"Success"}}}}}}`, name, upstreamURL)

	tool, err := service.CreateTool(
		name,
		"Tool for OAuth ACL tests",
		models.ToolTypeREST,
		base64.StdEncoding.EncodeToString([]byte(rawSpec)),
		5,
		"",
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, tool)

	tool.AddOperation("getTestData")
	require.NoError(t, service.DB.Save(tool).Error)

	// Re-read so the caller gets the slug the BeforeSave hook derived.
	stored, err := service.GetToolByID(tool.ID)
	require.NoError(t, err)
	require.NotEmpty(t, stored.Slug)
	return stored
}

func createACLApp(t *testing.T, service *services.Service, name string, userID, toolID uint) *models.App {
	t.Helper()

	app, err := service.CreateApp(name, "App for OAuth ACL tests", userID, []uint{}, []uint{}, []uint{toolID}, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, app)

	require.NoError(t, service.ActivateAppCredential(app.ID))

	stored, err := service.GetAppByID(app.ID)
	require.NoError(t, err)
	return stored
}

// mintOAuthToken writes an access_tokens row directly. This is exactly how the
// vulnerability was originally reproduced: /oauth/token hands back the app's
// credential secret, so nothing in the product mints one of these, yet the
// validator trusted any row it found.
func (f *oauthACLFixture) mintOAuthToken(t *testing.T, token string, userID uint, appID *uint, scope string) string {
	t.Helper()

	row := &models.AccessToken{
		Token:     token,
		ClientID:  "acl-test-client",
		UserID:    userID,
		Scope:     scope,
		ExpiresAt: time.Now().Add(time.Hour),
		AppID:     appID,
	}
	require.NoError(t, f.db.Create(row).Error)

	// The client has to exist: the OAuth branch loads it for the request context.
	var count int64
	require.NoError(t, f.db.Model(&models.OAuthClient{}).Where("client_id = ?", "acl-test-client").Count(&count).Error)
	if count == 0 {
		require.NoError(t, f.db.Create(&models.OAuthClient{
			ClientID:     "acl-test-client",
			ClientSecret: "not-a-real-secret",
			ClientName:   "ACL Test Client",
			RedirectURIs: "http://localhost/callback",
			Scope:        oauthscope.MCP,
		}).Error)
	}

	return token
}

func mcpRequest(t *testing.T, toolSlug, bearer string, payload map[string]interface{}) *http.Request {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tools/"+toolSlug+"/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

func mcpListRequest(t *testing.T, toolSlug, bearer string) *http.Request {
	t.Helper()
	return mcpRequest(t, toolSlug, bearer, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})
}

// mcpInitialize performs the StreamableHTTP handshake and returns the session id.
// The transport rejects a bare tools/list with 400 "Invalid session ID", so the
// allow-path tests have to establish a session first. A denial is answered by the
// credential middleware before the transport ever sees the request, which is why
// the refusal tests need no session.
func mcpInitialize(t *testing.T, handler http.Handler, toolSlug, bearer string) string {
	t.Helper()

	req := mcpRequest(t, toolSlug, bearer, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "acl-test", "version": "1.0.0"},
		},
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "initialize failed: %s", rr.Body.String())

	sessionID := rr.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, sessionID, "initialize returned no session id")
	return sessionID
}

// mcpListTools runs a full handshake-then-list and returns the tools/list response body.
func mcpListTools(t *testing.T, handler http.Handler, toolSlug, bearer string) (int, string) {
	t.Helper()

	sessionID := mcpInitialize(t, handler, toolSlug, bearer)

	req := mcpListRequest(t, toolSlug, bearer)
	req.Header.Set("Mcp-Session-Id", sessionID)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

func mcpCallRequest(t *testing.T, toolSlug, bearer string) *http.Request {
	t.Helper()

	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "getTestData",
			"arguments": map[string]interface{}{},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tools/"+toolSlug+"/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

func restToolRequest(t *testing.T, toolSlug, bearer string) *http.Request {
	t.Helper()

	body, err := json.Marshal(map[string]interface{}{
		"operation_id": "getTestData",
		"parameters":   map[string][]string{},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tools/"+toolSlug, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

// TestOAuthTokenCannotReachAnotherAppsTool is the regression test for the
// reported vulnerability.
func TestOAuthTokenCannotReachAnotherAppsTool(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-user-a-app-a", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	t.Run("initialize on a foreign tool is refused", func(t *testing.T) {
		// The handshake is the first thing an MCP client sends. Refusing it means
		// the caller never gets a session, so nothing downstream is reachable.
		// Before the fix this returned a session id.
		req := mcpRequest(t, f.toolB.Slug, token, map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      0,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo":      map[string]interface{}{"name": "acl-test", "version": "1.0.0"},
			},
		})

		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
		require.Empty(t, rr.Header().Get("Mcp-Session-Id"), "a refused handshake must not hand out a session")
	})

	t.Run("tools/list on a foreign tool is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolB.Slug, token))

		require.Equal(t, http.StatusUnauthorized, rr.Code)
		require.NotContains(t, rr.Body.String(), "getTestData", "a refused request must not enumerate the tool's operations")
	})

	t.Run("tools/call on a foreign tool is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, mcpCallRequest(t, f.toolB.Slug, token))

		require.Equal(t, http.StatusUnauthorized, rr.Code)
		require.NotContains(t, rr.Body.String(), `"tenant": "b"`, "a refused request must not reach the upstream")
	})

	t.Run("REST endpoint on a foreign tool is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, restToolRequest(t, f.toolB.Slug, token))

		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("the SSE transport on a foreign tool is refused", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/tools/"+f.toolB.Slug+"/mcp/sse", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

// TestOAuthTokenReachesItsOwnAppsTool is the other half: the fix must not break
// the case it is meant to allow.
func TestOAuthTokenReachesItsOwnAppsTool(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-user-a-own-tool", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	code, body := mcpListTools(t, f.handler, f.toolA.Slug, token)

	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "getTestData", "the granted tool's operations should be enumerated")
}

// TestOAuthTokenFullMCPSession is the happy path end to end: handshake, list,
// call. Without it the suite would only prove that things are refused.
func TestOAuthTokenFullMCPSession(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-full-session", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	session := mcpInitialize(t, f.handler, f.toolA.Slug, token)

	listReq := mcpListRequest(t, f.toolA.Slug, token)
	listReq.Header.Set("Mcp-Session-Id", session)
	listRR := httptest.NewRecorder()
	f.handler.ServeHTTP(listRR, listReq)
	require.Equal(t, http.StatusOK, listRR.Code)
	require.Contains(t, listRR.Body.String(), "getTestData")

	callReq := mcpCallRequest(t, f.toolA.Slug, token)
	callReq.Header.Set("Mcp-Session-Id", session)
	callRR := httptest.NewRecorder()
	f.handler.ServeHTTP(callRR, callReq)
	require.Equal(t, http.StatusOK, callRR.Code)
	// The call must actually reach tool A's upstream, not merely be permitted.
	require.Contains(t, callRR.Body.String(), "tenant", "tools/call should return upstream data")
	require.Contains(t, callRR.Body.String(), `a`, "tool A's upstream identifies itself as tenant a")
	require.NotContains(t, callRR.Body.String(), `"tenant": "b"`)

	// A session granted for tool A must not be reusable against tool B.
	replay := mcpListRequest(t, f.toolB.Slug, token)
	replay.Header.Set("Mcp-Session-Id", session)
	replayRR := httptest.NewRecorder()
	f.handler.ServeHTTP(replayRR, replay)
	require.Equal(t, http.StatusUnauthorized, replayRR.Code, "a session must not be replayable across tools")
	require.NotContains(t, replayRR.Body.String(), `"tenant": "b"`)
}

// TestOAuthTokenReachesRESTEndpoint covers the REST tool endpoint over OAuth.
// Before the fix this 401'd because the OAuth branch never put a tool on the
// context, which is why the vulnerability showed up on MCP only.
func TestOAuthTokenReachesRESTEndpoint(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-rest", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, restToolRequest(t, f.toolA.Slug, token))

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	require.Contains(t, rr.Body.String(), "tenant")
}

// TestOAuthRequestCarriesAppContext pins the contract the rest of the gateway
// depends on: budget, analytics, filters and the plugin hooks all read "app" off
// the request context, and the OAuth branch never used to set it.
func TestOAuthRequestCarriesAppContext(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-context", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	var seen *http.Request
	spy := f.proxy.credValidator.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r
	}))

	rr := httptest.NewRecorder()
	spy.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, token))
	require.NotNil(t, seen, "the request should have been passed through")

	app, ok := seen.Context().Value("app").(*models.App)
	require.True(t, ok, "an authenticated OAuth request must carry its app")
	require.Equal(t, f.appA.ID, app.ID)

	tool, ok := seen.Context().Value("tool").(*models.Tool)
	require.True(t, ok, "an authorised tool request must carry its tool")
	require.Equal(t, f.toolA.ID, tool.ID)

	require.Equal(t, f.toolA.Slug, seen.Context().Value("toolSlug"))

	user, ok := seen.Context().Value("user").(*models.User)
	require.True(t, ok, "the OAuth branch should still identify the user")
	require.Equal(t, f.userA.ID, user.ID)

	require.Equal(t, oauthscope.MCP, seen.Context().Value("scope"))
	require.NotNil(t, seen.Context().Value("oauthClient"))
}

func TestOAuthTokenRejectedWithoutAppBinding(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	// A token carrying no app binding was authorised against nothing at consent,
	// so it must grant nothing - including the tool of an app its own user owns.
	token := f.mintOAuthToken(t, "token-unbound", f.userA.ID, nil, oauthscope.MCP)

	for _, slug := range []string{f.toolA.Slug, f.toolB.Slug} {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, mcpListRequest(t, slug, token))
		require.Equal(t, http.StatusUnauthorized, rr.Code, "unbound token must not reach %s", slug)
	}
}

// TestOAuthTokenRejectedForForeignApp covers a binding the token has no right
// to. Consent and the token endpoint both verify ownership, but the validator is
// where the binding is actually trusted, so it re-checks rather than believing
// the token about its own authority.
func TestOAuthTokenRejectedForForeignApp(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	// User A's token, bound to user B's app, aimed at user B's tool. Every
	// individual fact is internally consistent - app B really does grant tool B -
	// and only the ownership check stands in the way.
	token := f.mintOAuthToken(t, "token-foreign-app", f.userA.ID, &f.appB.ID, oauthscope.MCP)

	for _, slug := range []string{f.toolA.Slug, f.toolB.Slug} {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, mcpListRequest(t, slug, token))
		require.Equal(t, http.StatusUnauthorized, rr.Code,
			"a token bound to another user's app must be refused for %s", slug)
	}
}

// TestOAuthTokenRejectedForMissingApp covers a binding that no longer resolves,
// for instance an app deleted after the token was issued.
func TestOAuthTokenRejectedForMissingApp(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	missing := uint(999999)
	token := f.mintOAuthToken(t, "token-missing-app", f.userA.ID, &missing, oauthscope.MCP)

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, token))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestOAuthTokenRejectedAfterAppLosesTool covers revocation: the grant is read
// per request, so removing the tool from the app takes effect immediately rather
// than at token expiry.
func TestOAuthTokenRejectedAfterAppLosesTool(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-revoked-grant", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, token))
	require.NotEqual(t, http.StatusUnauthorized, rr.Code, "precondition: the grant works")

	// Revoke the tool from the app.
	require.NoError(t, f.service.DB.Model(f.appA).Association("Tools").Clear())

	rr = httptest.NewRecorder()
	f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, token))
	require.Equal(t, http.StatusUnauthorized, rr.Code,
		"removing the tool from the app must take effect immediately")
}

// TestOAuthTokenRejectedWhenExpired covers the check that was already there, so
// that the added checks cannot accidentally short-circuit ahead of it.
func TestOAuthTokenRejectedWhenExpired(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	expired := &models.AccessToken{
		Token:     "token-expired",
		ClientID:  "acl-test-client",
		UserID:    f.userA.ID,
		Scope:     oauthscope.MCP,
		ExpiresAt: time.Now().Add(-time.Hour),
		AppID:     &f.appA.ID,
	}
	require.NoError(t, f.db.Create(expired).Error)

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, "token-expired"))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestOAuthTokenRequiresMCPScope(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	// The scope the original report used: identity only, no resource access.
	token := f.mintOAuthToken(t, "token-openid-only", f.userA.ID, &f.appA.ID, oauthscope.OpenID)

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, token))

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.NotEmpty(t, rr.Header().Get("WWW-Authenticate"), "an insufficient-scope refusal should challenge the client")
}

func TestOAuthTokenRejectedWithEmptyScope(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	// An empty scope grants nothing. /oauth/authorize defaults an omitted scope
	// to mcp, so a token that reaches here with none was not issued by that flow.
	token := f.mintOAuthToken(t, "token-empty-scope", f.userA.ID, &f.appA.ID, "")

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, token))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestOAuthTokenScopeIsNotSubstringMatched guards the scope check against the
// classic mistake of a strings.Contains.
func TestOAuthTokenScopeIsNotSubstringMatched(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	for _, scope := range []string{"mcpx", "notmcp", "supermcp", "MCP"} {
		t.Run(scope, func(t *testing.T) {
			token := f.mintOAuthToken(t, "token-scope-"+scope, f.userA.ID, &f.appA.ID, scope)

			rr := httptest.NewRecorder()
			f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, token))
			require.Equal(t, http.StatusUnauthorized, rr.Code,
				"scope %q must not be accepted as %q", scope, oauthscope.MCP)
		})
	}
}

func TestOAuthTokenScopeAcceptsAdditionalScopes(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-multi-scope", f.userA.ID, &f.appA.ID, "openid profile mcp")

	code, _ := mcpListTools(t, f.handler, f.toolA.Slug, token)

	require.Equal(t, http.StatusOK, code)
}

// TestOAuthTokenRejectedOnNonToolPaths pins the rule that OAuth access tokens are
// issued for MCP and authorise the tool endpoints only.
func TestOAuthTokenRejectedOnNonToolPaths(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "token-non-tool-path", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	req := httptest.NewRequest(http.MethodPost, "/llm/call/some-llm/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestAppSecretToolACLUnchanged guards the branch that was already correct, since
// it now shares an implementation with the branch that was not.
func TestAppSecretToolACLUnchanged(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	secretA := f.appA.Credential.Secret
	require.NotEmpty(t, secretA)

	t.Run("own tool over MCP is allowed", func(t *testing.T) {
		code, body := mcpListTools(t, f.handler, f.toolA.Slug, secretA)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, body, "getTestData")
	})

	t.Run("own tool over REST is allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, restToolRequest(t, f.toolA.Slug, secretA))
		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("foreign tool over MCP is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolB.Slug, secretA))
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("foreign tool over REST is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, restToolRequest(t, f.toolB.Slug, secretA))
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("an unknown tool is refused without saying so", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, mcpListRequest(t, "no-such-tool", secretA))
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		require.NotContains(t, rr.Body.String(), "not found", "must not leak whether the tool exists")
	})
}

// TestProtectedResourceMetadataStaysPublic guards the discovery endpoint. It is
// how a client learns where to authenticate, so it has to answer whether or not
// a credential is presented - including a credential the gateway would refuse
// everywhere else.
func TestProtectedResourceMetadataStaysPublic(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	credentials := map[string]string{
		"no credential":       "",
		"an OAuth token":      f.mintOAuthToken(t, "token-metadata", f.userA.ID, &f.appA.ID, oauthscope.MCP),
		"an app secret":       f.appA.Credential.Secret,
		"a nonsense token":    "not-a-real-token",
		"an unbound token":    f.mintOAuthToken(t, "token-metadata-unbound", f.userA.ID, nil, oauthscope.MCP),
		"an out-of-scope one": f.mintOAuthToken(t, "token-metadata-openid", f.userA.ID, &f.appA.ID, oauthscope.OpenID),
	}

	for name, credential := range credentials {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
			if credential != "" {
				req.Header.Set("Authorization", "Bearer "+credential)
			}

			rr := httptest.NewRecorder()
			f.handler.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			require.Contains(t, rr.Body.String(), oauthscope.MCP)
			require.NotContains(t, rr.Body.String(), "mcp_read", "only enforced scopes should be advertised")
		})
	}
}

func TestMCPRequestWithoutCredentialsIsRefused(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, mcpListRequest(t, f.toolA.Slug, ""))

	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestMCPHandlersFailClosedWithoutContextTool covers the defence in depth
// directly: whatever authenticated the request, a handler reached without an
// authorised tool on the context must refuse rather than resolve the tool from
// the URL itself.
func TestMCPHandlersFailClosedWithoutContextTool(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	p := NewProxy(service, &Config{Port: 9996}, budget.NewService(db, notificationSvc))

	tool := createACLTool(t, service, "Bare Handler Tool", "http://127.0.0.1:1")

	handlers := map[string]http.HandlerFunc{
		"streamable": p.handleMCPToolStreamable,
		"sse":        p.handleMCPToolSSE,
		"message":    p.handleMCPToolMessage,
	}

	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/tools/"+tool.Slug+"/mcp", nil)
			rr := httptest.NewRecorder()
			handler(rr, req)

			require.Equal(t, http.StatusUnauthorized, rr.Code)
		})
	}
}

// TestMCPHandlerRefusesMismatchedSlug stops a tool authorised on one path from
// being used to serve another.
func TestMCPHandlerRefusesMismatchedSlug(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	service := f.service
	notificationSvc := services.NewTestNotificationService(db)
	p := NewProxy(service, &Config{Port: 9995}, budget.NewService(db, notificationSvc))

	// Context carries tool A; the URL names tool B.
	req := httptest.NewRequest(http.MethodPost, "/tools/"+f.toolB.Slug+"/mcp", nil)
	req = req.WithContext(context.WithValue(req.Context(), "tool", f.toolA))
	req = mux.SetURLVars(req, map[string]string{"toolSlug": f.toolB.Slug})

	rr := httptest.NewRecorder()
	p.handleMCPToolStreamable(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
}
