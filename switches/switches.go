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
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
)

const (
	OpenAICompletionsEndpoint    = "/v1/chat/completions"
	AnthropicCompletionsEndpoint = "/v1/messages"
)

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
			promptTokens = int(usage["prompt_tokens"].(int))
			responseTokens = int(usage["response_tokens"].(int))
			totalTokens = promptTokens + responseTokens

			return totalTokens, promptTokens, responseTokens
		}
	case models.ANTHROPIC:
		dat, ok := choice.GenerationInfo["usage"]
		if ok {
			usage := dat.(map[string]interface{})
			promptTokens = int(usage["input_tokens"].(int))
			responseTokens = int(usage["output_tokens"].(int))
			totalTokens = promptTokens + responseTokens

			return totalTokens, promptTokens, responseTokens
		}
		// Vertex
		// Huggingface
		// ollama
		// GoogleAI
	default:
		slog.Warn("vendor not supported", "vendor", vendor)
		return 0, 0, 0
	}

	return 0, 0, 0
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
	// Vertex
	// Huggingface
	// ollama
	// GoogleAI
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

		// Vertex
		// Huggingface
		// ollama
		// GoogleAI
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
	}
	// Vertex
	// Huggingface
	// ollama
	// GoogleAI

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
	// Vertex
	// Huggingface
	// ollama
	// GoogleAI
	default:
		return fmt.Errorf("unknown vendor: %s", llm.Vendor)
	}

	return nil
}

func setupOpenAIDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]openai.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, openai.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, openai.WithToken(connDef.APIKey))
	}

	opts = append(opts, openai.WithModel(llmSettings.ModelName))

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI driver: %v", err)
	}

	return llm, nil
}

func setupAnthropicDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]anthropic.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, anthropic.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, anthropic.WithToken(connDef.APIKey))
	}

	opts = append(opts, anthropic.WithModel(llmSettings.ModelName))

	llm, err := anthropic.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI driver: %v", err)
	}

	return llm, nil
}
