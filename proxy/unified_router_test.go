package proxy

// Tests for the unified router endpoint (/v1/chat/completions, /v1/completions).
// The router accepts OpenRouter-style "vendor/model" model strings, resolves the
// vendor prefix to an LLM route slug, and rewrites the request to the existing
// /ai/{routeId}/v1/... shim path so the whole /ai/ chain (auth, access checks,
// translation) runs unchanged.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUnifiedModel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRoute string
		wantModel string
		wantErr   bool
	}{
		{"simple", "openai/gpt-4o", "openai", "gpt-4o", false},
		{"model contains slashes", "openai/ft:gpt-4o/my-org/suffix", "openai", "ft:gpt-4o/my-org/suffix", false},
		{"route slug with dashes", "my-azure-llm/gpt-4o-mini", "my-azure-llm", "gpt-4o-mini", false},
		{"no prefix", "gpt-4o", "", "", true},
		{"empty", "", "", "", true},
		{"empty vendor", "/gpt-4o", "", "", true},
		{"empty model", "openai/", "", "", true},
		{"only slash", "/", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			route, model, err := ParseUnifiedModel(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantRoute, route)
			assert.Equal(t, tc.wantModel, model)
		})
	}
}

// newUnifiedTestHandler builds a UnifiedRouterHandler whose downstream records
// the request it receives.
func newUnifiedTestHandler() (http.Handler, *unifiedRecordedRequest) {
	rec := &unifiedRecordedRequest{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.called = true
		rec.path = r.URL.Path
		rec.method = r.Method
		buf := new(bytes.Buffer)
		if r.Body != nil {
			_, _ = buf.ReadFrom(r.Body)
		}
		rec.body = buf.Bytes()
		rec.contentLength = r.ContentLength
		w.WriteHeader(http.StatusOK)
	})
	h := NewUnifiedRouterHandler(next)
	return h, rec
}

type unifiedRecordedRequest struct {
	called        bool
	path          string
	method        string
	body          []byte
	contentLength int64
}

func postJSON(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// oaiErrorMessage extracts the "error.message" field of an OpenAI-style error body.
func oaiErrorMessage(t *testing.T, body []byte) string {
	t.Helper()
	var resp OAIErrorResponse
	require.NoError(t, json.Unmarshal(body, &resp), "expected OpenAI-style error body, got: %s", body)
	require.NotNil(t, resp.Error)
	return resp.Error.Message
}

func TestUnifiedRouterHandler_RewritesChatCompletions(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/chat/completions",
		`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.True(t, rec.called)
	assert.Equal(t, "/ai/openai/v1/chat/completions", rec.path)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.body, &got))
	assert.Equal(t, `"gpt-4o"`, string(got["model"]), "vendor prefix must be stripped for the shim")
	assert.Equal(t, `[{"role":"user","content":"hi"}]`, string(got["messages"]))
	assert.Equal(t, int64(len(rec.body)), rec.contentLength)
}

func TestUnifiedRouterHandler_RewritesCompletions(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/completions", `{"model":"my-llm/gpt-3.5-turbo-instruct","prompt":"hi"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/ai/my-llm/v1/completions", rec.path)
}

func TestUnifiedRouterHandler_PreservesUnrelatedFieldsVerbatim(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	// Values chosen so any lossy decode/re-encode (float64 round-trip) would corrupt them.
	body := `{"model":"openai/gpt-4o","messages":[],"temperature":0.30000000000000004,"seed":9007199254740993,"stream":true}`
	w := postJSON(t, h, "/v1/chat/completions", body)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.body, &got))
	assert.Equal(t, "0.30000000000000004", string(got["temperature"]))
	assert.Equal(t, "9007199254740993", string(got["seed"]))
	assert.Equal(t, "true", string(got["stream"]))
}

func TestUnifiedRouterHandler_ModelWithoutVendorPrefix(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, rec.called)
	msg := oaiErrorMessage(t, w.Body.Bytes())
	// The error must teach the caller the expected format.
	assert.Contains(t, msg, "<vendor>/<model>")
	assert.Contains(t, msg, "gpt-4o")
}

func TestUnifiedRouterHandler_EmptyVendorOrModelPart(t *testing.T) {
	h, _ := newUnifiedTestHandler()

	for _, model := range []string{"/gpt-4o", "openai/", "/", ""} {
		t.Run("model="+model, func(t *testing.T) {
			w := postJSON(t, h, "/v1/chat/completions", `{"model":"`+model+`","messages":[]}`)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, oaiErrorMessage(t, w.Body.Bytes()), "<vendor>/<model>")
		})
	}
}

func TestUnifiedRouterHandler_MissingModelField(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/chat/completions", `{"messages":[]}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, rec.called)
	assert.Contains(t, oaiErrorMessage(t, w.Body.Bytes()), "<vendor>/<model>")
}

func TestUnifiedRouterHandler_ModelNotAString(t *testing.T) {
	h, _ := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/chat/completions", `{"model":42,"messages":[]}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUnifiedRouterHandler_InvalidJSON(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/chat/completions", `{not json`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, rec.called)
}

func TestUnifiedRouterHandler_UnknownVendorForwardsToAuthChain(t *testing.T) {
	// The handler does NOT resolve the vendor itself: every well-formed request
	// is forwarded so authentication runs first (an unauthenticated caller must
	// not be able to enumerate route slugs from 404-vs-401 responses). The /ai/
	// shim produces the vendor-not-found 404 after auth.
	h, rec := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/chat/completions", `{"model":"anthropic/claude-sonnet-5","messages":[]}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, rec.called)
	assert.Equal(t, "/ai/anthropic/v1/chat/completions", rec.path)
}

func TestUnifiedRouterHandler_RejectsUnsafeRouteSlugs(t *testing.T) {
	// Defense-in-depth: the route prefix becomes a path segment, so it must
	// never contain traversal sequences or other unsafe characters.
	h, rec := newUnifiedTestHandler()

	for _, model := range []string{"../etc/passwd", "..%2F/x", "a b/model", "route?/model", "ro#ute/model"} {
		t.Run("model="+model, func(t *testing.T) {
			rec.called = false
			body, err := json.Marshal(map[string]any{"model": model, "messages": []any{}})
			require.NoError(t, err)
			w := postJSON(t, h, "/v1/chat/completions", string(body))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.False(t, rec.called, "unsafe route slug must never be forwarded")
			assert.Contains(t, oaiErrorMessage(t, w.Body.Bytes()), "<vendor>/<model>")
		})
	}
}

func TestUnifiedRouterHandler_OversizedBodyRejected(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	// Just over the cap; the padding field keeps it valid JSON in shape.
	padding := strings.Repeat("x", maxUnifiedRouterBodyBytes+1)
	body := `{"model":"openai/gpt-4o","padding":"` + padding + `"}`
	w := postJSON(t, h, "/v1/chat/completions", body)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.False(t, rec.called)
}

func TestShimHandlers_UnknownRouteMessage(t *testing.T) {
	// After auth, the /ai/ shims produce the unified router's vendor-not-found
	// error; the wording must not reveal whether the route exists for others.
	p := &Proxy{llms: map[string]*models.LLM{}}

	t.Run("chat completions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/ai/nope/v1/chat/completions", strings.NewReader(`{}`))
		req = mux.SetURLVars(req, map[string]string{"routeId": "nope"})
		w := httptest.NewRecorder()
		p.CreateChatCompletionHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		msg := oaiErrorMessage(t, w.Body.Bytes())
		assert.Contains(t, msg, "nope")
		assert.Contains(t, msg, "not found or not supported by your access rights")
	})

	t.Run("completions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/ai/nope/v1/completions", strings.NewReader(`{}`))
		req = mux.SetURLVars(req, map[string]string{"routeId": "nope"})
		w := httptest.NewRecorder()
		p.CreateCompletionHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, oaiErrorMessage(t, w.Body.Bytes()), "not found or not supported by your access rights")
	})
}

func TestUnifiedRouterHandler_NonPOSTPassesThrough(t *testing.T) {
	// Non-completion requests (e.g. GET /v1/models) are not model-routed; they
	// pass through untouched so the authenticated chain can serve them.
	h, rec := newUnifiedTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.True(t, rec.called)
	assert.Equal(t, "/v1/models", rec.path, "pass-through must not rewrite the path")
	assert.Equal(t, http.MethodGet, rec.method)
}

func TestUnifiedRouterHandler_POSTNonCompletionPathPassesThrough(t *testing.T) {
	h, rec := newUnifiedTestHandler()

	w := postJSON(t, h, "/v1/embeddings", `{"model":"openai/text-embedding-3-small","input":"x"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, rec.called)
	assert.Equal(t, "/v1/embeddings", rec.path, "only the completion endpoints are model-routed")
}

// modelIDs extracts the id field of every entry in an OpenAI model-list response.
func modelIDs(t *testing.T, body []byte) []string {
	t.Helper()
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp), "body: %s", body)
	assert.Equal(t, "list", resp.Object)
	ids := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		assert.Equal(t, "model", m.Object)
		ids = append(ids, m.ID)
	}
	return ids
}

func TestHandleUnifiedListModels_ScopedToAppAccess(t *testing.T) {
	p := &Proxy{llms: map[string]*models.LLM{
		"openai": {ID: 1, Vendor: models.OPENAI, DefaultModel: "gpt-4o", AllowedModels: []string{"gpt-4o", "gpt-4o-mini"}},
		"claude": {ID: 2, Vendor: models.ANTHROPIC, DefaultModel: "claude-sonnet-5"},
	}}

	app := &models.App{ID: 10, LLMs: []models.LLM{{ID: 1}}} // access to "openai" only
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(context.WithValue(req.Context(), "app", app))
	w := httptest.NewRecorder()
	p.handleUnifiedListModels(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	ids := modelIDs(t, w.Body.Bytes())
	// AllowedModels + DefaultModel, deduped (gpt-4o appears in both), sorted.
	assert.Equal(t, []string{"openai/gpt-4o", "openai/gpt-4o-mini"}, ids)
	for _, id := range ids {
		assert.NotContains(t, id, "claude", "inaccessible route leaked into model list")
	}
}

func TestHandleUnifiedListModels_RouteWithNoKnownModelsOmitted(t *testing.T) {
	p := &Proxy{llms: map[string]*models.LLM{
		"bare": {ID: 1, Vendor: models.OPENAI}, // no AllowedModels, no DefaultModel
	}}

	app := &models.App{ID: 10, LLMs: []models.LLM{{ID: 1}}}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(context.WithValue(req.Context(), "app", app))
	w := httptest.NewRecorder()
	p.handleUnifiedListModels(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, modelIDs(t, w.Body.Bytes()))
}

func TestHandleUnifiedListModels_NoAppInContext(t *testing.T) {
	p := &Proxy{llms: map[string]*models.LLM{}}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	p.handleUnifiedListModels(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestUnifiedRouter_ProxyWiring exercises the endpoint through the full proxy
// handler chain: the vendor prefix is resolved against the live route table
// BEFORE auth (404 for unknown vendors), and valid vendors are forwarded into
// the authenticated /ai/ chain (401 without credentials proves the forward).
func TestUnifiedRouter_ProxyWiring(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	llm := &models.LLM{Name: "Mock LLM", Vendor: models.MOCK_VENDOR, Active: true}
	require.NoError(t, db.Create(llm).Error)

	p := New(service, budgetSvc, &Config{Port: 0})
	require.NoError(t, p.Reload())
	handler := p.Handler()

	t.Run("unknown vendor without credentials gets 401, not 404", func(t *testing.T) {
		// Auth runs before route resolution: an anonymous caller must not be able
		// to distinguish existing route slugs from unknown ones.
		w := postJSON(t, handler, "/v1/chat/completions", `{"model":"nope/gpt-4o","messages":[]}`)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("prefixless model 400s with format guidance", func(t *testing.T) {
		w := postJSON(t, handler, "/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, oaiErrorMessage(t, w.Body.Bytes()), "<vendor>/<model>")
	})

	t.Run("known vendor forwards into authenticated /ai/ chain", func(t *testing.T) {
		w := postJSON(t, handler, "/v1/chat/completions", `{"model":"mock-llm/mock-model","messages":[{"role":"user","content":"hi"}]}`)
		// No credentials supplied: reaching the /ai/ auth middleware (401) proves
		// the rewrite + forward happened instead of a routing 404.
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("models listing requires authentication", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		// The route is registered behind the credential middleware: unauthenticated
		// requests must never see the model list.
		assert.NotEqual(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "mock-llm")
	})
}
