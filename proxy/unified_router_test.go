package proxy

// Tests for the unified router endpoint (/v1/chat/completions, /v1/completions).
// The router accepts OpenRouter-style "vendor/model" model strings, resolves the
// vendor prefix to an LLM route slug, and rewrites the request to the existing
// /ai/{routeId}/v1/... shim path so the whole /ai/ chain (auth, access checks,
// translation) runs unchanged.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
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

// newUnifiedTestHandler builds a UnifiedRouterHandler whose route table contains
// the given slugs and whose downstream records the request it receives.
func newUnifiedTestHandler(routes ...string) (http.Handler, *unifiedRecordedRequest) {
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
	routeSet := make(map[string]bool, len(routes))
	for _, s := range routes {
		routeSet[s] = true
	}
	h := NewUnifiedRouterHandler(func(slug string) bool { return routeSet[slug] }, next)
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
	h, rec := newUnifiedTestHandler("openai")

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
	h, rec := newUnifiedTestHandler("my-llm")

	w := postJSON(t, h, "/v1/completions", `{"model":"my-llm/gpt-3.5-turbo-instruct","prompt":"hi"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/ai/my-llm/v1/completions", rec.path)
}

func TestUnifiedRouterHandler_PreservesUnrelatedFieldsVerbatim(t *testing.T) {
	h, rec := newUnifiedTestHandler("openai")

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
	h, rec := newUnifiedTestHandler("openai")

	w := postJSON(t, h, "/v1/chat/completions", `{"model":"gpt-4o","messages":[]}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, rec.called)
	msg := oaiErrorMessage(t, w.Body.Bytes())
	// The error must teach the caller the expected format.
	assert.Contains(t, msg, "<vendor>/<model>")
	assert.Contains(t, msg, "gpt-4o")
}

func TestUnifiedRouterHandler_EmptyVendorOrModelPart(t *testing.T) {
	h, _ := newUnifiedTestHandler("openai")

	for _, model := range []string{"/gpt-4o", "openai/", "/", ""} {
		t.Run("model="+model, func(t *testing.T) {
			w := postJSON(t, h, "/v1/chat/completions", `{"model":"`+model+`","messages":[]}`)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, oaiErrorMessage(t, w.Body.Bytes()), "<vendor>/<model>")
		})
	}
}

func TestUnifiedRouterHandler_MissingModelField(t *testing.T) {
	h, rec := newUnifiedTestHandler("openai")

	w := postJSON(t, h, "/v1/chat/completions", `{"messages":[]}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, rec.called)
	assert.Contains(t, oaiErrorMessage(t, w.Body.Bytes()), "<vendor>/<model>")
}

func TestUnifiedRouterHandler_ModelNotAString(t *testing.T) {
	h, _ := newUnifiedTestHandler("openai")

	w := postJSON(t, h, "/v1/chat/completions", `{"model":42,"messages":[]}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUnifiedRouterHandler_InvalidJSON(t *testing.T) {
	h, rec := newUnifiedTestHandler("openai")

	w := postJSON(t, h, "/v1/chat/completions", `{not json`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, rec.called)
}

func TestUnifiedRouterHandler_UnknownVendor(t *testing.T) {
	h, rec := newUnifiedTestHandler("openai")

	w := postJSON(t, h, "/v1/chat/completions", `{"model":"anthropic/claude-sonnet-5","messages":[]}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, rec.called)
	msg := oaiErrorMessage(t, w.Body.Bytes())
	assert.Contains(t, msg, "anthropic")
	assert.Contains(t, msg, "not found or not supported by your access rights")
}

func TestUnifiedRouterHandler_NonPOSTRejected(t *testing.T) {
	h, rec := newUnifiedTestHandler("openai")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.False(t, rec.called)
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

	t.Run("unknown vendor 404s before auth", func(t *testing.T) {
		w := postJSON(t, handler, "/v1/chat/completions", `{"model":"nope/gpt-4o","messages":[]}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
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
}
