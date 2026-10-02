package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// An endpoint with auth plugins attached is authenticated by them alone, on
// every route that reaches it; an endpoint without any keeps app keys. These
// cover the LLM routes (/llm/, /ai/, the unified /v1) and datasources; tools
// are in TestAuthPlugins_Tool.

func TestAuthPlugins_LLMListIsAuthoritative(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	plugins := newFakeAuthPlugins().attach(AuthTargetLLM, "primary")
	plugins.accept["jwt-for-app"] = h.app.ID
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: plugins.hook})

	for _, path := range []string{
		"/llm/call/primary/v1/chat/completions",
		"/llm/rest/primary/v1/chat/completions",
		"/ai/primary/v1/chat/completions",
	} {
		t.Run(path+"/plugin credential is served", func(t *testing.T) {
			resp, body := h.post(path, failoverChatBody, "Authorization", "Bearer jwt-for-app")
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
		})
		t.Run(path+"/app key is refused", func(t *testing.T) {
			resp, body := h.post(path, failoverChatBody, "Authorization", "Bearer "+h.apiKey)
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "body: %s", body)
		})
		t.Run(path+"/unknown credential is refused", func(t *testing.T) {
			resp, body := h.post(path, failoverChatBody, "Authorization", "Bearer nope")
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "body: %s", body)
		})
	}

	t.Run("/v1 unified addresses the same LLM", func(t *testing.T) {
		body := `{"model":"primary/gpt-4o","messages":[{"role":"user","content":"hi"}]}`
		resp, respBody := h.post("/v1/chat/completions", body, "Authorization", "Bearer jwt-for-app")
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", respBody)
		resp, respBody = h.post("/v1/chat/completions", body, "Authorization", "Bearer "+h.apiKey)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "body: %s", respBody)
	})

	t.Run("the API-key path is held to the same list", func(t *testing.T) {
		resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody, "Authorization", h.apiKey)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "body: %s", body)
	})
}

// The /ai/ loopback hands the outer hop's identity to the inner hop, so the
// plugin is asked once per request, not once per hop.
func TestAuthPlugins_AIRouteAsksThePluginOnce(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	plugins := newFakeAuthPlugins().attach(AuthTargetLLM, "primary")
	plugins.accept["jwt-for-app"] = h.app.ID
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: plugins.hook})

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody, "Authorization", "Bearer jwt-for-app")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	require.Equal(t, 1, plugins.callCount())
	assert.True(t, strings.HasPrefix(plugins.calls[0].Path, "/ai/"), "the outer hop is the one asked")

	for _, c := range h.primaryVendor.calls() {
		for k := range c.Headers {
			assert.False(t, strings.HasPrefix(k, "X-Tyk-Auth-"), "hand-off header %s reached the vendor", k)
		}
	}
}

// The route the caller addressed decides. A route without auth plugins
// accepts an app key even when the LLM it calls has some: the inner hop takes
// the outer hop's identity rather than asking that LLM's plugins.
func TestAuthPlugins_InnerHopFollowsTheRoute(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	plugins := newFakeAuthPlugins().attach(AuthTargetLLM, "primary")
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: plugins.hook})

	// Seen from the inner hop alone, /llm/call/primary has plugins and would
	// refuse the key. Drive the inner hop with the hand-off the outer hop sends.
	req := httptest.NewRequest(http.MethodPost, "/llm/call/primary/v1/chat/completions", strings.NewReader(failoverChatBody))
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	ctx := WithAuthIdentity(context.Background(), &AuthIdentity{AppID: h.app.ID, Method: AuthMethodAppKey})
	for k, v := range h.proxy.loopbackHeaders(ctx, llmAttempt{}) {
		req.Header[k] = v
	}
	req.Header.Set(hdrInternalHop, internalHopToken)

	var got *AuthIdentity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = AuthIdentityFromContext(r.Context())
		w.WriteHeader(http.StatusTeapot)
	})
	rr := httptest.NewRecorder()
	h.proxy.credValidator.Middleware(next).ServeHTTP(rr, req)
	require.Equal(t, http.StatusTeapot, rr.Code, "body: %s", rr.Body.String())
	require.NotNil(t, got)
	assert.Equal(t, h.app.ID, got.AppID)
	assert.Equal(t, AuthMethodAppKey, got.Method)
	assert.Zero(t, plugins.callCount(), "the inner hop must not ask the LLM's plugins")
}

// A hand-off is trusted only with this process's token over the loopback.
func TestAuthPlugins_ForgedHandoffIsIgnored(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: newFakeAuthPlugins().hook})

	for name, mutate := range map[string]func(*http.Request){
		"no token": func(r *http.Request) {
			r.Header.Set(hdrInternalHop, internalHopToken)
		},
		"wrong token": func(r *http.Request) {
			r.Header.Set(hdrInternalHop, internalHopToken)
			r.Header.Set(hdrFailoverToken, "guess")
		},
		"not over loopback": func(r *http.Request) {
			r.RemoteAddr = "203.0.113.9:5555"
			r.Header.Set(hdrInternalHop, internalHopToken)
			r.Header.Set(hdrFailoverToken, h.proxy.failoverToken)
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/llm/call/primary/v1/chat/completions", strings.NewReader(failoverChatBody))
			req.RemoteAddr = "127.0.0.1:5555"
			req.Header.Set(hdrAuthHandoffApp, "1")
			req.Header.Set(hdrAuthHandoffMethod, AuthMethodPlugin)
			mutate(req)
			reached := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })
			rr := httptest.NewRecorder()
			h.proxy.credValidator.Middleware(next).ServeHTTP(rr, req)
			assert.False(t, reached)
			assert.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
		})
	}
}

// An endpoint without auth plugins keeps app keys, and its plugins (of other
// endpoints) are never asked about its traffic.
func TestAuthPlugins_EndpointWithoutListKeepsAppKeys(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	plugins := newFakeAuthPlugins().attach(AuthTargetLLM, "fallback")
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: plugins.hook})

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody, "Authorization", "Bearer "+h.apiKey)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Zero(t, plugins.callCount())
}

func TestAuthPlugins_Datasource(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	ds := &models.Datasource{Name: "Docs", Active: true}
	require.NoError(t, h.db.Create(ds).Error)
	require.NoError(t, h.db.Model(h.app).Association("Datasources").Append(ds))
	require.NoError(t, h.proxy.loadResources())

	plugins := newFakeAuthPlugins().attach(AuthTargetDatasource, "docs")
	plugins.accept["jwt-for-app"] = h.app.ID
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: plugins.hook})

	// The handler rejects the non-JSON body with 400: auth let it through.
	resp, body := h.post("/datasource/docs", "not json", "Authorization", "Bearer jwt-for-app")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
	require.Equal(t, 1, plugins.callCount())
	assert.Equal(t, AuthTarget{Kind: AuthTargetDatasource, ID: ds.ID, Slug: "docs"}, plugins.calls[0].Target)
	assert.Equal(t, CredTypeBearer, plugins.calls[0].CredType)

	resp, body = h.post("/datasource/docs", "not json", "Authorization", "Bearer "+h.apiKey)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "body: %s", body)
}

// A plugin authenticates; the app it names must still hold the endpoint, be
// active and exist.
func TestAuthPlugins_AppIsStillAuthorised(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	plugins := newFakeAuthPlugins().attach(AuthTargetLLM, "fallback").attach(AuthTargetLLM, "primary")
	plugins.accept["jwt-for-app"] = h.app.ID
	plugins.accept["jwt-for-missing-app"] = 99999
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: plugins.hook})

	resp, body := h.post("/llm/call/fallback/v1/chat/completions", failoverChatBody, "Authorization", "Bearer jwt-for-app")
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "an ungranted LLM; body: %s", body)

	resp, body = h.post("/llm/call/primary/v1/chat/completions", failoverChatBody, "Authorization", "Bearer jwt-for-missing-app")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a missing app; body: %s", body)

	require.NoError(t, h.db.Model(&models.App{}).Where("id = ?", h.app.ID).Update("is_active", false).Error)
	resp, body = h.post("/llm/call/primary/v1/chat/completions", failoverChatBody, "Authorization", "Bearer jwt-for-app")
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "an inactive app; body: %s", body)
	assert.Empty(t, h.fallbackVendor.calls())
}

// Plugins that cannot be asked refuse the request (503) rather than letting
// app keys in.
func TestAuthPlugins_UnavailableFailsClosed(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{
		CustomAuth: func(*http.Request, AuthTarget, string, string) (AuthResult, error) {
			return AuthResult{}, errors.New("no auth plugin could be loaded")
		},
	})
	resp, body := h.post("/llm/call/primary/v1/chat/completions", failoverChatBody, "Authorization", "Bearer "+h.apiKey)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "body: %s", body)
	assert.Empty(t, h.primaryVendor.calls())
}

// The identity the request authenticated as is on the context for the
// handlers, analytics and the post-auth hook, whichever branch it came by.
func TestAuthPlugins_IdentityOnContext(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	plugins := newFakeAuthPlugins().attach(AuthTargetLLM, "primary")
	plugins.accept["jwt-for-app"] = h.app.ID
	plugins.subject = "user@example.com"

	var postAuthIdentity *AuthIdentity
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{
		CustomAuth: plugins.hook,
		PostAuth: func(w http.ResponseWriter, r *http.Request, appID uint) bool {
			postAuthIdentity = AuthIdentityFromContext(r.Context())
			return false
		},
	})

	serve := func(path, authorization string) *AuthIdentity {
		var got *AuthIdentity
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = AuthIdentityFromContext(r.Context())
		})
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(failoverChatBody))
		req.Header.Set("Authorization", authorization)
		h.proxy.credValidator.Middleware(next).ServeHTTP(httptest.NewRecorder(), req)
		return got
	}

	got := serve("/llm/call/primary/v1/chat/completions", "Bearer jwt-for-app")
	require.NotNil(t, got)
	assert.Equal(t, AuthIdentity{
		AppID: h.app.ID, Method: AuthMethodPlugin, PluginID: 42, PluginName: "fake-idp",
		Subject: "user@example.com", Claims: map[string]string{"auth_actor": "agent-1"},
	}, *got)
	assert.Equal(t, got, postAuthIdentity, "the post-auth hook sees the same identity")

	// No auth plugins on the route: an app key.
	plugins.attached = map[string]bool{}
	got = serve("/ai/primary/v1/chat/completions", "Bearer "+h.apiKey)
	require.NotNil(t, got)
	assert.Equal(t, AuthIdentity{AppID: h.app.ID, Method: AuthMethodAppKey}, *got)
}

// A tool's auth plugins authenticate its REST route and every MCP transport,
// on the bearer and API-key paths; the app key of the same app is refused.
func TestAuthPlugins_Tool(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)
	f := newOAuthACLFixture(t, db)

	plugins := newFakeAuthPlugins().attach(AuthTargetTool, f.toolA.Slug)
	plugins.accept["jwt-for-a"] = f.appA.ID
	plugins.accept["jwt-for-b"] = f.appB.ID
	f.proxy.credValidator.SetAuthHooks(&AuthHooks{CustomAuth: plugins.hook})
	t.Cleanup(func() { f.proxy.credValidator.SetAuthHooks(nil) })

	serve := func(req *http.Request) int {
		rr := httptest.NewRecorder()
		f.handler.ServeHTTP(rr, req)
		return rr.Code
	}

	t.Run("plugin credential on MCP", func(t *testing.T) {
		require.NotEqual(t, http.StatusUnauthorized, serve(mcpRequest(t, f.toolA.Slug, "jwt-for-a", initializePayloadForTest())))
	})
	t.Run("plugin credential as an API key", func(t *testing.T) {
		req := mcpRequest(t, f.toolA.Slug, "", initializePayloadForTest())
		req.Header.Set("Authorization", "jwt-for-a")
		require.NotEqual(t, http.StatusUnauthorized, serve(req))
		assert.Equal(t, CredTypeAPIKey, plugins.calls[len(plugins.calls)-1].CredType)
	})
	t.Run("the app key is refused", func(t *testing.T) {
		require.Equal(t, http.StatusUnauthorized, serve(mcpRequest(t, f.toolA.Slug, f.appA.Credential.Secret, initializePayloadForTest())))
	})
	t.Run("an app without the tool is refused", func(t *testing.T) {
		require.Equal(t, http.StatusUnauthorized, serve(mcpRequest(t, f.toolA.Slug, "jwt-for-b", initializePayloadForTest())))
	})
	t.Run("another tool keeps app keys", func(t *testing.T) {
		before := plugins.callCount()
		require.NotEqual(t, http.StatusUnauthorized, serve(mcpRequest(t, f.toolB.Slug, f.appB.Credential.Secret, initializePayloadForTest())))
		assert.Equal(t, before, plugins.callCount())
	})
	assert.Equal(t, AuthTarget{Kind: AuthTargetTool, ID: f.toolA.ID, Slug: f.toolA.Slug}, plugins.calls[0].Target)
}

// authTarget resolves /ai/{slug} to a router when the slug names one.
func TestAuthPlugins_RouterTarget(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	h.proxy.SetRouteResolver(&fakeRouteLookup{refs: map[string]RouterRef{
		"smart":    {Kind: RouterKindModel, ID: 7, Slug: "smart"},
		"semantic": {Kind: RouterKindSemantic, ID: 8, Slug: "semantic"},
	}})

	for path, want := range map[string]AuthTarget{
		"/ai/smart/v1/chat/completions":        {Kind: AuthTargetModelRouter, ID: 7, Slug: "smart"},
		"/ai/semantic/v1/chat/completions":     {Kind: AuthTargetSemanticRouter, ID: 8, Slug: "semantic"},
		"/anthropic/smart/v1/messages":         {Kind: AuthTargetModelRouter, ID: 7, Slug: "smart"},
		"/ai/primary/v1/chat/completions":      {Kind: AuthTargetLLM, ID: h.primary.ID, Slug: "primary"},
		"/llm/stream/primary/chat/completions": {Kind: AuthTargetLLM, ID: h.primary.ID, Slug: "primary"},
	} {
		got, ok := h.proxy.credValidator.authTarget(httptest.NewRequest(http.MethodPost, path, nil))
		require.True(t, ok, path)
		assert.Equal(t, want, got, path)
	}

	_, ok := h.proxy.credValidator.authTarget(httptest.NewRequest(http.MethodPost, "/ai/nothing/v1/chat/completions", nil))
	assert.False(t, ok)
}

// fakeRouteLookup is a RouteResolver that only knows which slugs are routers.
type fakeRouteLookup struct {
	refs map[string]RouterRef
}

func (f *fakeRouteLookup) Lookup(slug string) (RouterRef, bool) {
	ref, ok := f.refs[slug]
	return ref, ok
}

func (f *fakeRouteLookup) Resolve(context.Context, RouteRequest) (*RouteDecision, error) {
	return nil, ErrRouteUnavailable
}

func (f *fakeRouteLookup) Reaches(RouterRef, uint) bool { return false }

func (f *fakeRouteLookup) Models(RouterRef) []string { return nil }
