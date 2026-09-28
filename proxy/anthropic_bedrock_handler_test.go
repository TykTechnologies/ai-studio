package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// anthropicErrorType decodes an Anthropic-format error body and returns error.type,
// asserting the envelope is a well-formed error.
func anthropicErrorType(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Equal(t, "error", parsed.Type)
	return parsed.Error.Type
}

// TestHandleAnthropicMessagesEntry_ErrorBranches covers the entry handler's early
// rejections, which return before any auth/AWS involvement.
func TestHandleAnthropicMessagesEntry_ErrorBranches(t *testing.T) {
	bedrockLLM := &models.LLM{Name: "bedrock", Vendor: models.BEDROCK, DefaultModel: "anthropic.claude-3-haiku-20240307-v1:0"}
	openaiLLM := &models.LLM{Name: "openai", Vendor: models.OPENAI}

	validBody := `{"model":"claude","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`

	tests := []struct {
		name        string
		llms        map[string]*models.LLM
		routeID     string
		body        string
		wantStatus  int
		wantErrType string
	}{
		{"route not found", map[string]*models.LLM{}, "missing", validBody, 404, "not_found_error"},
		{"non-bedrock vendor rejected", map[string]*models.LLM{"openai": openaiLLM}, "openai", validBody, 400, "invalid_request_error"},
		{"invalid json body", map[string]*models.LLM{"bedrock": bedrockLLM}, "bedrock", "{not json", 400, "invalid_request_error"},
		{"empty messages", map[string]*models.LLM{"bedrock": bedrockLLM}, "bedrock", `{"model":"claude","max_tokens":10,"messages":[]}`, 400, "invalid_request_error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Proxy{llms: tc.llms}
			r := httptest.NewRequest("POST", "/anthropic/"+tc.routeID+"/v1/messages", strings.NewReader(tc.body))
			r = mux.SetURLVars(r, map[string]string{"routeId": tc.routeID})
			w := httptest.NewRecorder()

			p.handleAnthropicMessagesEntry(w, r)

			assert.Equal(t, tc.wantStatus, w.Code)
			assert.Equal(t, tc.wantErrType, anthropicErrorType(t, w.Body.Bytes()))
		})
	}
}

// TestResolveAnthropicModelID covers model governance: the bridge uses the configured
// default_model (Claude Code's own model name is not a Bedrock ID) and validates it
// against allowed_models.
func TestResolveAnthropicModelID(t *testing.T) {
	p := &Proxy{}

	t.Run("missing default_model -> 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		model, ok := p.resolveAnthropicModelID(w, &models.LLM{})
		assert.False(t, ok)
		assert.Empty(t, model)
		assert.Equal(t, 400, w.Code)
		assert.Equal(t, "invalid_request_error", anthropicErrorType(t, w.Body.Bytes()))
	})

	t.Run("default_model not in allowed_models -> 403", func(t *testing.T) {
		w := httptest.NewRecorder()
		conf := &models.LLM{
			DefaultModel:  "anthropic.claude-3-haiku-20240307-v1:0",
			AllowedModels: []string{`^cohere\.`},
		}
		model, ok := p.resolveAnthropicModelID(w, conf)
		assert.False(t, ok)
		assert.Empty(t, model)
		assert.Equal(t, 403, w.Code)
		assert.Equal(t, "permission_error", anthropicErrorType(t, w.Body.Bytes()))
	})

	t.Run("allowed (empty list) -> ok, nothing written", func(t *testing.T) {
		w := httptest.NewRecorder()
		conf := &models.LLM{DefaultModel: "anthropic.claude-3-haiku-20240307-v1:0"}
		model, ok := p.resolveAnthropicModelID(w, conf)
		assert.True(t, ok)
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", model)
		assert.Equal(t, 200, w.Code) // default: handler wrote nothing
		assert.Empty(t, w.Body.String())
	})

	t.Run("allowed (matching pattern) -> ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		conf := &models.LLM{
			DefaultModel:  "anthropic.claude-3-haiku-20240307-v1:0",
			AllowedModels: []string{`^anthropic\.`},
		}
		model, ok := p.resolveAnthropicModelID(w, conf)
		assert.True(t, ok)
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", model)
	})
}

// anthropicModelList decodes a GET /v1/models discovery response.
func anthropicModelList(t *testing.T, body []byte) AnthropicModelListResponse {
	t.Helper()
	var parsed AnthropicModelListResponse
	require.NoError(t, json.Unmarshal(body, &parsed))
	return parsed
}

// callListModels invokes the discovery handler with routeId wired into the mux vars.
func callListModels(p *Proxy, routeID string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/anthropic/"+routeID+"/v1/models?limit=1000", nil)
	r = mux.SetURLVars(r, map[string]string{"routeId": routeID})
	w := httptest.NewRecorder()
	p.handleAnthropicListModels(w, r)
	return w
}

// TestHandleAnthropicListModels covers the model-discovery endpoint consumed by Claude Code's
// /model picker: the listed entry, its display name, and the empty and error branches.
func TestHandleAnthropicListModels(t *testing.T) {
	t.Run("configured connection lists its model", func(t *testing.T) {
		p := &Proxy{llms: map[string]*models.LLM{
			"bedrock": {Name: "My Claude", Vendor: models.BEDROCK, DefaultModel: "anthropic.claude-3-haiku-20240307-v1:0"},
		}}
		w := callListModels(p, "bedrock")

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		resp := anthropicModelList(t, w.Body.Bytes())
		assert.False(t, resp.HasMore)
		require.Len(t, resp.Data, 1)
		assert.Equal(t, "model", resp.Data[0].Type)
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", resp.Data[0].ID)
		assert.Equal(t, "My Claude — anthropic.claude-3-haiku-20240307-v1:0", resp.Data[0].DisplayName)
	})

	t.Run("region-prefixed model id is returned unchanged", func(t *testing.T) {
		for _, id := range []string{
			"eu.anthropic.claude-3-5-sonnet-20241022-v2:0",
			"us.anthropic.claude-3-5-sonnet-20241022-v2:0",
			"global.anthropic.claude-sonnet-4-5-20250929-v1:0",
			"us-gov.anthropic.claude-3-haiku-20240307-v1:0",
		} {
			t.Run(id, func(t *testing.T) {
				p := &Proxy{llms: map[string]*models.LLM{
					"bedrock": {Name: "Regional Claude", Vendor: models.BEDROCK, DefaultModel: id},
				}}
				w := callListModels(p, "bedrock")

				assert.Equal(t, 200, w.Code)
				resp := anthropicModelList(t, w.Body.Bytes())
				require.Len(t, resp.Data, 1)
				assert.Equal(t, id, resp.Data[0].ID)
				// Claude Code keeps an id containing "claude" or "anthropic".
				assert.Contains(t, resp.Data[0].ID, "anthropic")
				assert.Equal(t, "Regional Claude — "+id, resp.Data[0].DisplayName)
			})
		}
	})

	t.Run("empty name falls back to model id", func(t *testing.T) {
		p := &Proxy{llms: map[string]*models.LLM{
			"bedrock": {Vendor: models.BEDROCK, DefaultModel: "anthropic.claude-3-haiku-20240307-v1:0"},
		}}
		w := callListModels(p, "bedrock")

		resp := anthropicModelList(t, w.Body.Bytes())
		require.Len(t, resp.Data, 1)
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", resp.Data[0].DisplayName)
	})

	t.Run("no default_model -> 200 empty list", func(t *testing.T) {
		p := &Proxy{llms: map[string]*models.LLM{
			"bedrock": {Name: "Bedrock", Vendor: models.BEDROCK},
		}}
		w := callListModels(p, "bedrock")

		assert.Equal(t, 200, w.Code)
		resp := anthropicModelList(t, w.Body.Bytes())
		assert.False(t, resp.HasMore)
		assert.Empty(t, resp.Data)
		// Must serialize as [], never null.
		assert.Contains(t, w.Body.String(), `"data":[]`)
	})

	t.Run("default_model refused by allowed_models -> 200 empty list, as /v1/messages refuses it", func(t *testing.T) {
		conf := &models.LLM{
			Name:          "Bedrock",
			Vendor:        models.BEDROCK,
			DefaultModel:  "anthropic.claude-3-haiku-20240307-v1:0",
			AllowedModels: []string{`^anthropic\.claude-3-5-sonnet`},
		}
		p := &Proxy{llms: map[string]*models.LLM{"bedrock": conf}}
		w := callListModels(p, "bedrock")

		assert.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), `"data":[]`)
		// The patterns are never listed as models.
		assert.NotContains(t, w.Body.String(), "sonnet")

		// The real endpoint refuses the same model, so the list and /v1/messages agree.
		mw := httptest.NewRecorder()
		_, ok := p.resolveAnthropicModelID(mw, conf)
		assert.False(t, ok)
		assert.Equal(t, 403, mw.Code)
	})

	t.Run("non-bedrock vendor -> 400", func(t *testing.T) {
		p := &Proxy{llms: map[string]*models.LLM{"openai": {Name: "openai", Vendor: models.OPENAI}}}
		w := callListModels(p, "openai")

		assert.Equal(t, 400, w.Code)
		assert.Equal(t, "invalid_request_error", anthropicErrorType(t, w.Body.Bytes()))
	})

	t.Run("route not found -> 404", func(t *testing.T) {
		p := &Proxy{llms: map[string]*models.LLM{}}
		w := callListModels(p, "missing")

		assert.Equal(t, 404, w.Code)
		assert.Equal(t, "not_found_error", anthropicErrorType(t, w.Body.Bytes()))
	})
}
