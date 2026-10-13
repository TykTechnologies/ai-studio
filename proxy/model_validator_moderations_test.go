package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
)

// "model" is optional on /v1/moderations: a missing one is OpenAI's default
// moderation model (checked against the allow-list); everywhere else,
// /v1/responses included, it is still required.
func TestOpenAIModelExtractorModerationsDefault(t *testing.T) {
	for _, path := range []string{"/llm/rest/openai/v1/moderations", "/llm/call/openai/v1/moderations/"} {
		m, err := OpenAIModelExtractor(httptest.NewRequest("POST", path, nil), []byte(`{"input":"text"}`))
		require.NoError(t, err, path)
		assert.Equal(t, openaiVendor.DefaultModerationModel, m)
	}
	m, err := OpenAIModelExtractor(httptest.NewRequest("POST", "/llm/rest/openai/v1/moderations", nil), []byte(`{"model":"text-moderation-stable","input":"x"}`))
	require.NoError(t, err)
	assert.Equal(t, "text-moderation-stable", m)

	for _, path := range []string{"/llm/rest/openai/v1/responses", "/llm/rest/openai/v1/chat/completions", "/llm/rest/openai/v1/embeddings"} {
		_, err := OpenAIModelExtractor(httptest.NewRequest("POST", path, nil), []byte(`{"input":"text"}`))
		var bad *BadRequestError
		assert.True(t, errors.As(err, &bad), path)
	}

	v := NewModelValidator([]string{"^gpt-4o$"})
	assert.False(t, v.IsModelAllowed(openaiVendor.DefaultModerationModel), "an allow-list without the default refuses a model-less moderation")
	assert.True(t, NewModelValidator(nil).IsModelAllowed(openaiVendor.DefaultModerationModel), "no allow-list: passes")
}

// End to end: a moderation without a model reaches OpenAI when the LLM has
// no allow-list or allows the default model, and is refused (403) when its
// allow-list leaves the default out. /v1/responses still needs a model.
func TestModerationsWithoutModelThroughTheGateway(t *testing.T) {
	h := newResponsesHarness(t)
	setAllowed := func(patterns []string) {
		t.Helper()
		h.settle()
		h.primary.AllowedModels = patterns
		require.NoError(t, h.db.Save(h.primary).Error)
		require.NoError(t, h.proxy.loadResources())
	}
	moderate := func() int {
		resp, _ := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/moderations", `{"input":"some text"}`)
		return resp.StatusCode
	}

	setAllowed(nil)
	assert.NotEqual(t, http.StatusBadRequest, moderate(), "no allow-list: forwarded")
	assert.Len(t, h.api.callsTo(http.MethodPost, "/v1/moderations"), 1)

	setAllowed([]string{"^gpt-4o$"})
	assert.Equal(t, http.StatusForbidden, moderate(), "the allow-list is checked against the default model")
	assert.Len(t, h.api.callsTo(http.MethodPost, "/v1/moderations"), 1)

	setAllowed([]string{"^gpt-4o$", "^omni-moderation-latest$"})
	assert.NotEqual(t, http.StatusForbidden, moderate())
	assert.Len(t, h.api.callsTo(http.MethodPost, "/v1/moderations"), 2)

	resp, out := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"input":"hi"}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	assert.Contains(t, string(out), "model field is required")
}
