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
