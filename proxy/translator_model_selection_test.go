package proxy

// Tests for model selection on the OpenAI-compatible shim (/ai/{slug}/v1/chat/completions).
//
// Contract under test: the client-supplied "model" field (already validated against the
// LLM config's AllowedModels before this point) must be the model sent upstream, with
// the config's DefaultModel used only as a fallback when the request omits the field.
//
// The current implementation of ChatCompletionRequest.ToLangchainOptions discards the
// request model on every non-Bedrock vendor (translator_models.go:83-98), so the
// request-model-wins tests below fail until that is fixed. Both the streaming and
// non-streaming shim handlers route through ToLangchainOptions, so this seam covers both.

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/llms"
)

// resolveModelOption applies the produced call options and returns the model that
// would actually be sent to the upstream driver.
func resolveModelOption(opts []llms.CallOption) string {
	resolved := &llms.CallOptions{}
	for _, opt := range opts {
		opt(resolved)
	}
	return resolved.Model
}

func TestToLangchainOptions_RequestModelWins(t *testing.T) {
	// The request model must win over DefaultModel for every vendor served by the
	// shim's langchaingo path. (Bedrock branches off before ToLangchainOptions and
	// already honors the request model — see bedrock_translator.go.)
	tests := []struct {
		name         string
		vendor       models.Vendor
		defaultModel string
		requestModel string
	}{
		{"openai", models.OPENAI, "gpt-4", "gpt-4o-mini"},
		{"anthropic", models.ANTHROPIC, "claude-3-opus-20240229", "claude-3-5-haiku-20241022"},
		{"google_ai", models.GOOGLEAI, "gemini-1.5-pro", "gemini-1.5-flash"},
		{"ollama", models.OLLAMA, "llama3", "mistral"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &models.LLM{
				Vendor:       tt.vendor,
				DefaultModel: tt.defaultModel,
			}
			req := &ChatCompletionRequest{Model: tt.requestModel}

			model := resolveModelOption(req.ToLangchainOptions(conf))

			assert.Equal(t, tt.requestModel, model,
				"the model requested by the client must be sent upstream, not the config's DefaultModel")
		})
	}
}

func TestToLangchainOptions_DefaultModelIsFallbackOnly(t *testing.T) {
	// When the request omits the model, DefaultModel applies.
	tests := []struct {
		name   string
		vendor models.Vendor
	}{
		{"openai", models.OPENAI},
		{"anthropic", models.ANTHROPIC},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &models.LLM{
				Vendor:       tt.vendor,
				DefaultModel: "configured-default",
			}
			req := &ChatCompletionRequest{Model: ""}

			model := resolveModelOption(req.ToLangchainOptions(conf))

			assert.Equal(t, "configured-default", model,
				"DefaultModel must apply when the request does not name a model")
		})
	}
}

func TestToLangchainOptions_RequestModelUsedWhenNoDefault(t *testing.T) {
	// A config with no DefaultModel must still pass the requested model upstream
	// rather than sending no model option at all.
	for _, vendor := range []models.Vendor{models.OPENAI, models.ANTHROPIC} {
		t.Run(string(vendor), func(t *testing.T) {
			conf := &models.LLM{Vendor: vendor, DefaultModel: ""}
			req := &ChatCompletionRequest{Model: "requested-model"}

			model := resolveModelOption(req.ToLangchainOptions(conf))

			assert.Equal(t, "requested-model", model)
		})
	}
}

func TestToLangchainOptions_DoesNotMutateRequest(t *testing.T) {
	// ToLangchainOptions must not overwrite req.Model: the handler later echoes
	// req.Model back in the response body (translator.go NewChatCompletionResponse),
	// so mutating it makes the response misreport which model served the request.
	conf := &models.LLM{
		Vendor:       models.OPENAI,
		DefaultModel: "gpt-4",
	}
	req := &ChatCompletionRequest{Model: "gpt-4o-mini"}

	req.ToLangchainOptions(conf)

	assert.Equal(t, "gpt-4o-mini", req.Model,
		"ToLangchainOptions must not mutate the request's model field")
}

func TestHandleOptions_LegacyCompletionsRespectsRequestModel(t *testing.T) {
	// Regression guard: the legacy /ai/{slug}/v1/completions path already honors the
	// request model (translator.go handleOptions). The chat completions path must
	// behave the same way.
	req := &CreateCompletionRequest{Model: "requested-model"}

	model := resolveModelOption(handleOptions(req))

	assert.Equal(t, "requested-model", model)
}
