// pckage switches is the place where all vendor-dependent logic lives. This should make organising and extending the code easier.
package switches

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/huggingface"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
)

const (
	OpenAICompletionsEndpoint         = "/v1/chat/completions"
	AnthropicCompletionsEndpoint      = "/v1/messages"
	OllamaChatCompletionsEndpoint     = "/api/chat"
	OllamaGenerateCompletionsEndpoint = "/api/generate"
)

var AVAILABLE_LLM_DRIVERS = []models.Vendor{
	models.ANTHROPIC,
	models.OPENAI,
	models.OLLAMA,
	models.VERTEX,
	models.GOOGLEAI,
}

var AVAILABLE_EMBEDDERS = []models.Vendor{
	models.OPENAI,
	models.OLLAMA,
	models.VERTEX,
	models.GOOGLEAI,
}

// Handles token count finding for analytics, different vendors have different response types and we nee dto handle all of them
func GetTokenCounts(choice *llms.ContentChoice, vendor models.Vendor) (int, int, int) {
	promptTokens := 0
	responseTokens := 0
	totalTokens := 0

	switch vendor {
	case models.OPENAI:
		dat, ok := choice.GenerationInfo["usage"]
		if ok {
			usage := dat.(map[string]interface{})
			promptTokens = keyValueOrZero(usage, "prompt_tokens")
			responseTokens = keyValueOrZero(usage, "response_tokens")
			totalTokens = promptTokens + responseTokens

			return totalTokens, promptTokens, responseTokens
		}
	case models.ANTHROPIC:
		dat, ok := choice.GenerationInfo["usage"]
		if ok {
			usage := dat.(map[string]interface{})
			promptTokens = keyValueOrZero(usage, "input_tokens")
			responseTokens = keyValueOrZero(usage, "output_tokens")
			totalTokens = promptTokens + responseTokens

			return totalTokens, promptTokens, responseTokens
		}
	case models.OLLAMA:
		promptTokens := keyValueOrZero(choice.GenerationInfo, "PromptTokens")
		responseTokens := keyValueOrZero(choice.GenerationInfo, "CompletionTokens")
		totalTokens := promptTokens + responseTokens

		return totalTokens, promptTokens, responseTokens

	case models.VERTEX:
		promptTokens := keyValueOrZero(choice.GenerationInfo, "input_tokens")
		responseTokens := keyValueOrZero(choice.GenerationInfo, "output_tokens")
		totalTokens := promptTokens + responseTokens

		return totalTokens, promptTokens, responseTokens

	case models.GOOGLEAI:
		promptTokens := keyValueOrZero(choice.GenerationInfo, "input_tokens")
		responseTokens := keyValueOrZero(choice.GenerationInfo, "output_tokens")
		totalTokens := promptTokens + responseTokens

		return totalTokens, promptTokens, responseTokens
	case models.HUGGINGFACE:
		return 0, 0, 0

	default:
		slog.Warn("vendor not supported", "vendor", vendor)
		return 0, 0, 0
	}

	return 0, 0, 0
}

func keyValueOrZero(dat map[string]any, key string) int {
	if val, ok := dat[key]; ok {
		val, ok := val.(int)
		if ok {
			return val
		}
	}
	return 0
}

func FetchDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	var llm llms.Model
	var err error
	switch LLMConfig.Vendor {
	case models.OPENAI:
		llm, err = setupOpenAIDriver(LLMConfig, settings)
	case models.ANTHROPIC:
		llm, err = setupAnthropicDriver(LLMConfig, settings)
	case models.MOCK_VENDOR:
		llm = &DummyDriver{
			StreamingFunc: streamingFunc,
			Memory:        mem,
		}
	case models.VERTEX:
		llm, err = setupVertexDriver(LLMConfig, settings)
	case models.GOOGLEAI:
		llm, err = setupGoogleDriver(LLMConfig, settings)
	case models.HUGGINGFACE:
		llm, err = setupHuggingFaceDriver(LLMConfig, settings)
	case models.OLLAMA:
		llm, err = setupOllamaDriver(LLMConfig, settings)
	default:
		return nil, fmt.Errorf("unsupported LLM model: %s", settings.ModelName)
	}

	return llm, err
}

func GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	var llm embeddings.EmbedderClient
	var err error

	switch d.EmbedVendor {
	case models.OPENAI:
		opts := []openai.Option{}
		if d.EmbedAPIKey != "" {
			opts = append(opts, openai.WithToken(d.EmbedAPIKey))
		}
		if d.EmbedUrl != "" {
			opts = append(opts, openai.WithBaseURL(d.EmbedUrl))
		}
		if d.EmbedModel == "" {
			return nil, fmt.Errorf("missing embed model")
		}

		opts = append(opts, openai.WithEmbeddingModel(d.EmbedModel))
		llm, err = openai.New(opts...)
	case models.VERTEX:
		llm, err = setupVertexEmbedClient(d)

	case models.HUGGINGFACE:
		opts := []huggingface.Option{}
		if d.EmbedAPIKey != "" {
			opts = append(opts, huggingface.WithToken(d.EmbedAPIKey))
		}

		llm, err = NewHFWrapper(d.EmbedModel, opts...)
	case models.OLLAMA:
		opts := []ollama.Option{}
		if d.EmbedUrl != "" {
			opts = append(opts, ollama.WithServerURL(d.EmbedUrl))
		}
		if d.EmbedModel == "" {
			return nil, fmt.Errorf("missing embed model")
		}

		opts = append(opts, ollama.WithModel(d.EmbedModel))
		llm, err = ollama.New(opts...)

	case models.GOOGLEAI:
		llm, err = setupGoogleAIEmbedClient(d)

	default:
		return nil, fmt.Errorf("unsupported embed vendor")
	}

	if err != nil {
		return nil, err
	}

	e, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	var response models.ITokenResponse
	switch llm.Vendor {
	case models.OPENAI:
		if strings.Contains(strings.ToLower(r.URL.Path), OpenAICompletionsEndpoint) {
			response = &responses.OpenAIResponse{}
			err := json.Unmarshal(body, response)
			if err != nil {
				return nil, nil, nil, err
			}
			return llm, app, response, nil
		}
	case models.ANTHROPIC:
		if strings.Contains(strings.ToLower(r.URL.Path), AnthropicCompletionsEndpoint) {
			response = &responses.AnthropicResponse{}
			err := json.Unmarshal(body, response)
			if err != nil {
				return nil, nil, nil, err
			}
			return llm, app, response, nil
		}
	case models.MOCK_VENDOR:
		response = &responses.DummyResponse{}
		err := json.Unmarshal(body, response)
		if err != nil {
			return nil, nil, nil, err
		}
		return llm, app, response, nil
	case models.HUGGINGFACE:
		// does not do token counts
		mName := "huggingface-unspecified"
		if strings.Contains(r.URL.Path, "/models/") {
			n, err := ExtractModelName(r.URL.Path)
			if err != nil {
				return nil, nil, nil, err
			}

			mName = n
		}

		return llm, app, &responses.DummyResponse{
			Model: mName}, nil

	case models.OLLAMA:
		if strings.Contains(strings.ToLower(r.URL.Path), OllamaChatCompletionsEndpoint) ||
			strings.Contains(strings.ToLower(r.URL.Path), OllamaGenerateCompletionsEndpoint) {
			response = &responses.OllamaGenerateResponse{}
			err := json.Unmarshal(body, response)
			if err != nil {
				return nil, nil, nil, err
			}
			return llm, app, response, nil
		}

	case models.VERTEX:
		modelName, err := extractModelIDFromVertexURL(r.URL.Path)
		if err != nil {
			return nil, nil, nil, err
		}

		if modelName == "" {
			modelName = "googleai-gemini-no-id"
		}

		response = &responses.GoogleAIChatResponse{}
		err = json.Unmarshal(body, response)
		if err != nil {
			return nil, nil, nil, err
		}
		response.(*responses.GoogleAIChatResponse).SetModel(modelName)
		return llm, app, response, nil

	case models.GOOGLEAI:
		modelName, err := extractModelIDFromGoogleURL(r.URL.Path)
		if err != nil {
			return nil, nil, nil, err
		}

		if modelName == "" {
			modelName = "googleai-gemini-no-id"
		}

		response = &responses.GoogleAIChatResponse{}
		err = json.Unmarshal(body, response)
		if err != nil {
			return nil, nil, nil, err
		}
		response.(*responses.GoogleAIChatResponse).SetModel(modelName)
		return llm, app, response, nil
	}

	return nil, nil, nil, fmt.Errorf("unknown vendor: %s", llm.Vendor)
}

func SetVendorAuthHeader(r *http.Request, llm *models.LLM) error {
	switch llm.Vendor {
	case models.OPENAI:
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	case models.ANTHROPIC:
		r.Header.Set("x-api-key", llm.APIKey)
	case "DUMMY":
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	case models.GOOGLEAI:
		r.Header.Set("x-goog-api-key", llm.APIKey)
	case models.OLLAMA:
		r.Header.Set("Authorization", llm.APIKey)
	case models.HUGGINGFACE:
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	case models.VERTEX:
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))

	default:
		return fmt.Errorf("unknown vendor: %s", llm.Vendor)
	}

	return nil
}
