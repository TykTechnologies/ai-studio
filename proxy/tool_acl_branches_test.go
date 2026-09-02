package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/oauthscope"
)

// The tool ACL is reached through six different authentication branches in
// CredentialValidator.Middleware. Three of them - the OAuth branch and the two
// custom-auth plugin branches - performed no tool check at all before this fix,
// and the plugin_authenticated short-circuit skipped the middleware entirely.
//
// Testing only the branch the bug was reported against would leave the same
// defect live in the others, so every branch is driven through the same matrix:
// its own app's tool must work, another app's tool must not, on every route that
// reaches a tool.

// toolRoute is one of the four URLs that reach a tool through the router.
type toolRoute struct {
	name   string
	method string
	// path builds the URL for a slug.
	path func(slug string) string
	// body is the request body, if any.
	body func(slug string) map[string]interface{}
}

func toolRoutes() []toolRoute {
	return []toolRoute{
		{
			name:   "REST",
			method: http.MethodPost,
			path:   func(slug string) string { return "/tools/" + slug },
			body: func(string) map[string]interface{} {
				return map[string]interface{}{"operation_id": "getTestData", "parameters": map[string][]string{}}
			},
		},
		{
			name:   "MCP streamable",
			method: http.MethodPost,
			path:   func(slug string) string { return "/tools/" + slug + "/mcp" },
			body:   func(string) map[string]interface{} { return initializePayloadForTest() },
		},
		{
			name:   "MCP message",
			method: http.MethodPost,
			path:   func(slug string) string { return "/tools/" + slug + "/mcp/message" },
			body:   func(string) map[string]interface{} { return initializePayloadForTest() },
		},
		{
			name:   "MCP SSE",
			method: http.MethodGet,
			path:   func(slug string) string { return "/tools/" + slug + "/mcp/sse" },
			body:   nil,
		},
	}
}

func initializePayloadForTest() map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0", "id": 0, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "acl-test", "version": "1.0.0"},
		},
	}
}

// requestForRoute builds a request for a route, authenticated with a bearer token.
func requestForRoute(t *testing.T, route toolRoute, slug, bearer string) *http.Request {
	t.Helper()

	var req *http.Request
	if route.body != nil {
		body, err := json.Marshal(route.body(slug))
		require.NoError(t, err)
		req = httptest.NewRequest(route.method, route.path(slug), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
	} else {
		req = httptest.NewRequest(route.method, route.path(slug), nil)
		req.Header.Set("Accept", "text/event-stream")
	}

	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

// serveBounded runs a request with a deadline on its context.
//
// An authorized SSE request is supposed to hold the connection open forever, so
// the granted-tool cases cannot simply call ServeHTTP and wait. Cancelling the
// context makes the streaming handler return, and by then the status code has
// already been written - which is the only thing these assertions read.
func serveBounded(t *testing.T, handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	ctx, cancel := context.WithTimeout(req.Context(), 750*time.Millisecond)
	defer cancel()

	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rr, req.WithContext(ctx))
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after its request context was cancelled")
	}
	return rr
}

// assertToolACL drives one credential through every tool route against both the
// granted tool and a foreign one.
//
// The granted case asserts "not 401" rather than 200: some routes answer with a
// transport-level error (the MCP message and SSE endpoints want a session that
// this single request has not established). What matters for the ACL is that
// authorization let the request through to the transport, and that the foreign
// tool never gets that far.
func assertToolACL(t *testing.T, handler http.Handler, bearer, grantedSlug, foreignSlug string) {
	t.Helper()

	for _, route := range toolRoutes() {
		route := route

		t.Run(route.name+"/granted tool is authorized", func(t *testing.T) {
			rr := serveBounded(t, handler, requestForRoute(t, route, grantedSlug, bearer))
			require.NotEqual(t, http.StatusUnauthorized, rr.Code,
				"the granted tool must clear authorization; body: %s", rr.Body.String())
		})

		t.Run(route.name+"/foreign tool is refused", func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, requestForRoute(t, route, foreignSlug, bearer))
			require.Equal(t, http.StatusUnauthorized, rr.Code,
				"another app's tool must be refused; body: %s", rr.Body.String())
			require.NotContains(t, rr.Body.String(), "getTestData",
				"a refused request must not enumerate the tool")
		})

		t.Run(route.name+"/unknown tool is refused", func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, requestForRoute(t, route, "no-such-tool-anywhere", bearer))
			require.Equal(t, http.StatusUnauthorized, rr.Code,
				"an unknown tool must be refused, not 404'd; body: %s", rr.Body.String())
		})
	}
}

func TestToolACL_OAuthBranch(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	token := f.mintOAuthToken(t, "branch-oauth", f.userA.ID, &f.appA.ID, oauthscope.MCP)

	assertToolACL(t, f.handler, token, f.toolA.Slug, f.toolB.Slug)
}

func TestToolACL_AppSecretBranch(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	assertToolACL(t, f.handler, f.appA.Credential.Secret, f.toolA.Slug, f.toolB.Slug)
}

// TestToolACL_APIKeyBranch covers CheckAPICredential. The app secret doubles as
// the API key; presenting it without the Bearer prefix takes the API-key path
// instead of the bearer one.
func TestToolACL_APIKeyBranch(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)
	apiKey := f.appA.Credential.Secret

	apiKeyRequest := func(t *testing.T, slug string) *http.Request {
		t.Helper()
		req := mcpRequest(t, slug, "", initializePayloadForTest())
		req.Header.Set("Authorization", apiKey) // no "Bearer " prefix
		return req
	}

	t.Run("granted tool is authorized", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, apiKeyRequest(t, f.toolA.Slug))
		require.NotEqual(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("foreign tool is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, apiKeyRequest(t, f.toolB.Slug))
		require.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("api key in the query string is subject to the same ACL", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/tools/"+f.toolB.Slug+"/mcp?apiKey="+apiKey, nil)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})
}

// TestToolACL_BearerCustomAuthBranch covers an auth plugin that authenticates a
// bearer credential. The plugin says which app the caller is; it does not get to
// decide which tools that app may reach.
func TestToolACL_BearerCustomAuthBranch(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	// The plugin authenticates any credential as app A.
	f.proxy.credValidator.SetAuthHooks(&AuthHooks{
		CustomAuth: func(credential string, r *http.Request) (uint, bool, error) {
			if credential == "plugin-credential" {
				return f.appA.ID, true, nil
			}
			return 0, false, nil
		},
	})
	t.Cleanup(func() { f.proxy.credValidator.SetAuthHooks(nil) })

	assertToolACL(t, f.handler, "plugin-credential", f.toolA.Slug, f.toolB.Slug)
}

// TestToolACL_APIKeyCustomAuthBranch is the same plugin on the API-key path.
func TestToolACL_APIKeyCustomAuthBranch(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	f.proxy.credValidator.SetAuthHooks(&AuthHooks{
		CustomAuth: func(credential string, r *http.Request) (uint, bool, error) {
			if credential == "plugin-api-key" {
				return f.appA.ID, true, nil
			}
			return 0, false, nil
		},
	})
	t.Cleanup(func() { f.proxy.credValidator.SetAuthHooks(nil) })

	pluginKeyRequest := func(t *testing.T, slug string) *http.Request {
		t.Helper()
		req := mcpRequest(t, slug, "", initializePayloadForTest())
		req.Header.Set("Authorization", "plugin-api-key") // no Bearer prefix -> API key path
		return req
	}

	t.Run("granted tool is authorized", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, pluginKeyRequest(t, f.toolA.Slug))
		require.NotEqual(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("foreign tool is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, pluginKeyRequest(t, f.toolB.Slug))
		require.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})
}

// TestToolACL_PluginAuthenticatedShortCircuit covers the branch that skips
// credential validation entirely because a microgateway plugin already
// authenticated the request. It previously let any tool through on the MCP
// routes while the REST route failed closed for want of a context tool.
func TestToolACL_PluginAuthenticatedShortCircuit(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	f := newOAuthACLFixture(t, db)

	// Reproduce what microgateway's plugin middleware puts on the context.
	preAuthenticated := func(t *testing.T, slug string, appID interface{}) *http.Request {
		t.Helper()
		req := mcpRequest(t, slug, "", initializePayloadForTest())
		ctx := context.WithValue(req.Context(), "plugin_authenticated", true)
		if appID != nil {
			ctx = context.WithValue(ctx, "app_id", appID)
		}
		return req.WithContext(ctx)
	}

	t.Run("granted tool is authorized", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, preAuthenticated(t, f.toolA.Slug, f.appA.ID))
		require.NotEqual(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("foreign tool is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, preAuthenticated(t, f.toolB.Slug, f.appA.ID))
		require.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("no app id on the context is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, preAuthenticated(t, f.toolA.Slug, nil))
		require.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("an unresolvable app id is refused", func(t *testing.T) {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, preAuthenticated(t, f.toolA.Slug, uint(999999)))
		require.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	// The microgateway middleware writes a uint; other producers on this context
	// key use int/int64. All must resolve rather than silently fail open.
	for _, appID := range []interface{}{f.appA.ID, int(f.appA.ID), int64(f.appA.ID), uint32(f.appA.ID)} {
		t.Run(fmt.Sprintf("app id of type %T is honoured", appID), func(t *testing.T) {
			rr := httptest.NewRecorder()
			f.handler.ServeHTTP(rr, preAuthenticated(t, f.toolB.Slug, appID))
			require.Equal(t, http.StatusUnauthorized, rr.Code,
				"a resolvable app must still be held to the ACL; body: %s", rr.Body.String())

			rr = httptest.NewRecorder()
			f.handler.ServeHTTP(rr, preAuthenticated(t, f.toolA.Slug, appID))
			require.NotEqual(t, http.StatusUnauthorized, rr.Code,
				"the granted tool must clear authorization; body: %s", rr.Body.String())
		})
	}

	// A non-tool path is untouched by this branch and still passes straight
	// through, as it did before.
	t.Run("non-tool paths still short-circuit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/llm/call/whatever/v1/chat/completions", nil)
		ctx := context.WithValue(req.Context(), "plugin_authenticated", true)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, req)
		require.NotEqual(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})
}
