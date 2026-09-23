package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The LLM, datasource and route ACL (targetAllowed) used to be applied by the
// API-key branch only. A bearer app secret - which is what every OpenAI-shaped
// client sends on /v1 and /ai/ - authenticated the caller and then let it name
// any active LLM on the gateway, and the custom-auth and plugin-authenticated
// branches did the same. As with the tool ACL, every branch is driven through
// the same cases rather than only the one the bug was reported against.
//
// These run on the failover harness: its app is granted "primary" only, and
// "fallback" is another active LLM on the same gateway - the ungranted target.

// llmACLRoute is one way of naming an LLM.
type llmACLRoute struct {
	name string
	path func(slug string) string
	body func(slug string) string
}

func llmACLRoutes() []llmACLRoute {
	return []llmACLRoute{
		{
			name: "/llm/call",
			path: func(slug string) string { return "/llm/call/" + slug + "/v1/chat/completions" },
			body: func(string) string { return failoverChatBody },
		},
		{
			name: "/llm/rest",
			path: func(slug string) string { return "/llm/rest/" + slug + "/v1/chat/completions" },
			body: func(string) string { return failoverChatBody },
		},
		{
			name: "/ai",
			path: func(slug string) string { return "/ai/" + slug + "/v1/chat/completions" },
			body: func(string) string { return failoverChatBody },
		},
		{
			name: "/v1 unified",
			path: func(string) string { return "/v1/chat/completions" },
			body: func(slug string) string {
				return `{"model":"` + slug + `/gpt-4o","messages":[{"role":"user","content":"Say this is a test!"}]}`
			},
		},
	}
}

// assertLLMACL drives one bearer credential through every LLM route against the
// granted primary, the ungranted fallback and an unknown slug. The /ai/ and /v1
// cases cover both hops of the bridge: the granted one is only served if the
// loopback's inner /llm/call/ hop passes the check too.
func assertLLMACL(t *testing.T, h *failoverHarness, authorization string) {
	t.Helper()

	for _, route := range llmACLRoutes() {
		route := route

		t.Run(route.name+"/granted LLM is served", func(t *testing.T) {
			resp, body := h.post(route.path("primary"), route.body("primary"), "Authorization", authorization)
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
		})

		t.Run(route.name+"/ungranted LLM is refused", func(t *testing.T) {
			resp, body := h.post(route.path("fallback"), route.body("fallback"), "Authorization", authorization)
			require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
		})

		t.Run(route.name+"/unknown LLM is refused", func(t *testing.T) {
			resp, body := h.post(route.path("no-such-llm"), route.body("no-such-llm"), "Authorization", authorization)
			require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
		})
	}

	assert.Empty(t, h.fallbackVendor.calls(), "an ungranted LLM must never be reached")
}

// TestLLMACL_BearerAppSecretBranch is the reported bug: an app granted only
// "primary" got 200 from "fallback" by naming it.
func TestLLMACL_BearerAppSecretBranch(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	assertLLMACL(t, h, "Bearer "+h.apiKey)

	t.Run("/llm/stream/ungranted LLM is refused", func(t *testing.T) {
		resp, body := h.post("/llm/stream/fallback/v1/chat/completions", failoverStreamBody)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
	})

	t.Run("a refused LLM keeps the error shape of the gateway", func(t *testing.T) {
		resp, body := h.post("/ai/fallback/v1/chat/completions", failoverChatBody)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
		assert.Contains(t, string(body), "vendor 'fallback' not found or not supported by your access rights")

		// Unknown reads exactly like ungranted, so slugs cannot be enumerated.
		resp, unknown := h.post("/ai/no-such-llm/v1/chat/completions", failoverChatBody)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		assert.Equal(t, strings.ReplaceAll(string(body), "fallback", "no-such-llm"), string(unknown))
	})

	t.Run("bad credentials are still 401", func(t *testing.T) {
		resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody, "Authorization", "Bearer not-a-secret")
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "body: %s", body)
	})
}

// TestLLMACL_FailoverStillReachesUngrantedFallback: the inner hop for a rung
// names the fallback, which the app does not hold. The waterfall must still
// reach it, while the same app naming the fallback directly is refused.
func TestLLMACL_FailoverStillReachesUngrantedFallback(t *testing.T) {
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("from fallback"), nil)

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "true", resp.Header.Get(hdrFailover))
	require.Len(t, h.fallbackVendor.calls(), 1)

	resp, body = h.post("/ai/fallback/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
	require.Len(t, h.fallbackVendor.calls(), 1, "the direct request must not reach the fallback")
}

// TestLLMACL_BearerCustomAuthBranch: an auth plugin says which app the caller
// is; it does not decide which LLMs that app may use. The /ai/ loopback
// forwards the same bearer credential, so the plugin authenticates both hops.
func TestLLMACL_BearerCustomAuthBranch(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{
		CustomAuth: func(credential string, r *http.Request) (uint, bool, error) {
			if credential == "plugin-credential" {
				return h.app.ID, true, nil
			}
			return 0, false, nil
		},
	})

	assertLLMACL(t, h, "Bearer plugin-credential")
}

// TestLLMACL_APIKeyCustomAuthBranch is the same plugin on the API-key path,
// reached on /ai/ by a credential without the Bearer prefix. The inner hop of a
// granted request cannot re-authenticate a non-bearer key for an OpenAI LLM,
// so only the refusal is asserted.
func TestLLMACL_APIKeyCustomAuthBranch(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	h.proxy.credValidator.SetAuthHooks(&AuthHooks{
		CustomAuth: func(credential string, r *http.Request) (uint, bool, error) {
			if credential == "plugin-api-key" {
				return h.app.ID, true, nil
			}
			return 0, false, nil
		},
	})

	resp, body := h.post("/ai/fallback/v1/chat/completions", failoverChatBody, "Authorization", "plugin-api-key")
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)

	resp, body = h.post("/ai/primary/v1/chat/completions", failoverChatBody, "Authorization", "plugin-api-key")
	require.NotEqual(t, http.StatusForbidden, resp.StatusCode, "the outer hop must let a granted route through; body: %s", body)
	assert.Empty(t, h.fallbackVendor.calls())
}

// TestLLMACL_APIKeyBranch pins that CheckAPICredential, now built on the shared
// check, still refuses an ungranted route as it always has (401).
func TestLLMACL_APIKeyBranch(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)

	resp, body := h.post("/ai/fallback/v1/chat/completions", failoverChatBody, "Authorization", h.apiKey)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "body: %s", body)

	// A granted route clears the outer hop. (Its inner /llm/call/ hop cannot
	// re-authenticate a non-bearer key for an OpenAI LLM - that predates this
	// check - so the refusal to look for is the outer middleware's own.)
	resp, body = h.post("/ai/primary/v1/chat/completions", failoverChatBody, "Authorization", h.apiKey)
	require.NotContains(t, string(body), "Invalid API key or insufficient permissions", "status %d", resp.StatusCode)
	require.NotEqual(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
}

// TestLLMACL_PluginAuthenticatedBranch covers requests a microgateway auth
// plugin already authenticated. They used to pass straight through on every
// non-tool path. The context values cannot be sent over the wire, so this
// drives the handler in-process.
func TestLLMACL_PluginAuthenticatedBranch(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)
	handler := h.proxy.createHandler()

	preAuthenticated := func(path string, appID interface{}) *http.Request {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(failoverChatBody))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), "plugin_authenticated", true)
		if appID != nil {
			ctx = context.WithValue(ctx, "app_id", appID)
		}
		return req.WithContext(ctx)
	}
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	t.Run("granted LLM is served", func(t *testing.T) {
		rr := serve(preAuthenticated("/llm/call/primary/v1/chat/completions", h.app.ID))
		require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	})

	for _, path := range []string{
		"/llm/call/fallback/v1/chat/completions",
		"/llm/rest/fallback/v1/chat/completions",
		"/ai/fallback/v1/chat/completions",
		"/llm/call/no-such-llm/v1/chat/completions",
	} {
		t.Run(path+" is refused", func(t *testing.T) {
			rr := serve(preAuthenticated(path, h.app.ID))
			require.Equal(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())
		})
	}

	t.Run("no app id is refused", func(t *testing.T) {
		rr := serve(preAuthenticated("/llm/call/primary/v1/chat/completions", nil))
		require.Equal(t, http.StatusUnauthorized, rr.Code, "body: %s", rr.Body.String())
	})

	t.Run("an inactive app is refused", func(t *testing.T) {
		require.NoError(t, h.db.Model(&models.App{}).Where("id = ?", h.app.ID).Update("is_active", false).Error)
		t.Cleanup(func() {
			require.NoError(t, h.db.Model(&models.App{}).Where("id = ?", h.app.ID).Update("is_active", true).Error)
		})
		rr := serve(preAuthenticated("/llm/call/primary/v1/chat/completions", h.app.ID))
		require.Equal(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())
	})

	assert.Empty(t, h.fallbackVendor.calls())
}

// TestLLMACL_Datasources: the same hole existed for /datasource/{slug}. A
// granted datasource gets as far as the handler (which rejects the non-JSON
// body with 400); an ungranted or unknown one is refused before it.
func TestLLMACL_Datasources(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)

	granted := &models.Datasource{Name: "Granted DS", Active: true}
	require.NoError(t, h.db.Create(granted).Error)
	foreign := &models.Datasource{Name: "Foreign DS", Active: true}
	require.NoError(t, h.db.Create(foreign).Error)
	require.NoError(t, h.db.Model(h.app).Association("Datasources").Append(granted))
	require.NoError(t, h.proxy.loadResources())

	h.proxy.credValidator.SetAuthHooks(&AuthHooks{
		CustomAuth: func(credential string, r *http.Request) (uint, bool, error) {
			if credential == "plugin-credential" {
				return h.app.ID, true, nil
			}
			return 0, false, nil
		},
	})

	for _, auth := range []struct{ name, header string }{
		{"app secret", "Bearer " + h.apiKey},
		{"custom auth", "Bearer plugin-credential"},
	} {
		t.Run(auth.name+"/granted", func(t *testing.T) {
			resp, body := h.post("/datasource/granted-ds", "not json", "Authorization", auth.header)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
		})
		t.Run(auth.name+"/ungranted", func(t *testing.T) {
			resp, body := h.post("/datasource/foreign-ds", "not json", "Authorization", auth.header)
			require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
		})
		t.Run(auth.name+"/unknown", func(t *testing.T) {
			resp, body := h.post("/datasource/no-such-ds", "not json", "Authorization", auth.header)
			require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
		})
	}

	t.Run("plugin authenticated/ungranted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/datasource/foreign-ds", bytes.NewBufferString("not json"))
		ctx := context.WithValue(req.Context(), "plugin_authenticated", true)
		ctx = context.WithValue(ctx, "app_id", h.app.ID)
		rr := httptest.NewRecorder()
		h.proxy.createHandler().ServeHTTP(rr, req.WithContext(ctx))
		require.Equal(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())
	})
}

// TestLLMACL_BedrockOuterHop: Bedrock is served from the /ai/ hop itself, with
// no /llm/call/ loopback behind it, so the outer hop is its only check.
func TestLLMACL_BedrockOuterHop(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("primary"), serveOpenAIText("never"), nil)

	bedrock := &models.LLM{Name: "Bedrock Spare", Vendor: models.BEDROCK, Active: true, DefaultModel: "anthropic.claude-x"}
	require.NoError(t, h.db.Create(bedrock).Error)
	require.NoError(t, h.proxy.loadResources())
	_, ok := h.proxy.GetLLM("bedrock-spare")
	require.True(t, ok, "the Bedrock LLM must be loaded, or this only tests an unknown slug")

	resp, body := h.post("/ai/bedrock-spare/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)

	resp, body = h.post("/v1/chat/completions",
		`{"model":"bedrock-spare/anthropic.claude-x","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)

	resp, body = h.post("/anthropic/bedrock-spare/v1/messages",
		`{"model":"anthropic.claude-x","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
}

func TestTargetFromPath(t *testing.T) {
	tests := []struct {
		path string
		want gatewayTarget
	}{
		{"/llm/call/my-llm/v1/chat/completions", gatewayTarget{targetLLM, "my-llm"}},
		{"/llm/rest/my-llm/v1/chat/completions", gatewayTarget{targetLLM, "my-llm"}},
		{"/llm/stream/my-llm/v1/chat/completions", gatewayTarget{targetLLM, "my-llm"}},
		{"/ai/my-llm/v1/chat/completions", gatewayTarget{targetRoute, "my-llm"}},
		{"/anthropic/my-llm/v1/messages", gatewayTarget{targetRoute, "my-llm"}},
		{"/datasource/my-ds", gatewayTarget{targetDatasource, "my-ds"}},
		{"/datasource/my-ds/vector", gatewayTarget{targetDatasource, "my-ds"}},

		// A recognised prefix with no slug keeps its kind, so it is refused
		// rather than treated as naming nothing.
		{"/llm", gatewayTarget{targetLLM, ""}},
		{"/ai/", gatewayTarget{targetRoute, ""}},
		{"/datasource", gatewayTarget{targetDatasource, ""}},

		// Not LLM, datasource or route paths.
		{"/tools/my-tool/mcp", gatewayTarget{}},
		{"/v1/models", gatewayTarget{}},
		{"/.well-known/oauth-protected-resource", gatewayTarget{}},
		{"/llms/x", gatewayTarget{}},
		{"/", gatewayTarget{}},
		{"", gatewayTarget{}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, targetFromPath(tt.path))
		})
	}
}

func TestTargetAllowed_FailsClosed(t *testing.T) {
	llm := &models.LLM{ID: 1, Name: "Granted"}
	other := &models.LLM{ID: 2, Name: "Other"}
	cv := &CredentialValidator{p: &Proxy{
		llms:        map[string]*models.LLM{"granted": llm, "other": other},
		llmsByID:    map[uint]*models.LLM{1: llm, 2: other},
		datasources: map[string]*models.Datasource{"ds": {ID: 5}},
	}}
	app := &models.App{LLMs: []models.LLM{*llm}, Datasources: []models.Datasource{{ID: 5}}}
	r := httptest.NewRequest(http.MethodPost, "/", nil)

	assert.True(t, cv.targetAllowed(r, app, gatewayTarget{targetLLM, "granted"}))
	assert.True(t, cv.targetAllowed(r, app, gatewayTarget{targetRoute, "granted"}))
	assert.True(t, cv.targetAllowed(r, app, gatewayTarget{targetDatasource, "ds"}))
	assert.True(t, cv.targetAllowed(r, app, gatewayTarget{}), "a path naming nothing is not this check's to refuse")

	assert.False(t, cv.targetAllowed(r, app, gatewayTarget{targetLLM, "other"}))
	assert.False(t, cv.targetAllowed(r, app, gatewayTarget{targetRoute, "other"}))
	assert.False(t, cv.targetAllowed(r, app, gatewayTarget{targetLLM, ""}))
	assert.False(t, cv.targetAllowed(r, app, gatewayTarget{targetDatasource, "missing"}))
	assert.False(t, cv.targetAllowed(r, &models.App{}, gatewayTarget{targetLLM, "granted"}), "an app with no grants gets nothing")
	assert.False(t, cv.targetAllowed(r, nil, gatewayTarget{targetLLM, "granted"}), "a nil app must fail closed")
	assert.False(t, cv.targetAllowed(r, app, gatewayTarget{kind: "unexpected", slug: "granted"}))
}
