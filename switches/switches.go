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
	OpenAIEmbeddingsEndpoint          = "/v1/embeddings"
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
		usage := choice.GenerationInfo
		promptTokens = keyValueOrZero(usage, "PromptTokens")
		responseTokens = keyValueOrZero(usage, "TotalTokens")
		totalTokens = promptTokens + responseTokens
		fmt.Println(totalTokens)
		return totalTokens, promptTokens, responseTokens

	case models.ANTHROPIC:
		dat := choice.GenerationInfo
		promptTokens = keyValueOrZero(dat, "InputTokens")
		responseTokens = keyValueOrZero(dat, "OutputTokens")
		totalTokens = promptTokens + responseTokens

		return totalTokens, promptTokens, responseTokens

	case models.OLLAMA:
		promptTokens := keyValueOrZero(choice.GenerationInfo, "PromptTokens")
		responseTokens := keyValueOrZero(choice.GenerationInfo, "CompletionTokens")
		totalTokens := promptTokens + responseTokens

		return totalTokens, promptTokens, responseTokens

	case models.VERTEX:
		promptTokens := keyValueInt32OrZero(choice.GenerationInfo, "input_tokens")
		responseTokens := keyValueInt32OrZero(choice.GenerationInfo, "output_tokens")
		totalTokens := promptTokens + responseTokens

		return totalTokens, promptTokens, responseTokens

	case models.GOOGLEAI:
		promptTokens := keyValueInt32OrZero(choice.GenerationInfo, "input_tokens")
		responseTokens := keyValueInt32OrZero(choice.GenerationInfo, "output_tokens")
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
		// embedding replies have the same usage section
		if strings.Contains(strings.ToLower(r.URL.Path), OpenAICompletionsEndpoint) ||
			strings.Contains(strings.ToLower(r.URL.Path), OpenAIEmbeddingsEndpoint) {
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
		mName := extractHuggingfaceModelID(r.URL.Path)

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

	case "TestVendor":
		response := &responses.ProxyDummyResponse{}
		err := json.Unmarshal(body, response)
		if err != nil {
			return nil, nil, nil, err
		}

		return llm, app, response, nil
	}

	return nil, nil, nil, fmt.Errorf("[analyse response] unknown vendor: %s", llm.Vendor)
}

func AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps [][]byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	var response models.ITokenResponse
	switch llm.Vendor {
	case models.OPENAI:
		aggregate := &responses.GenericResponse{}
		for _, body := range resps {
			tempResp := &responses.OpenAIStreamingResponse{}
			err := json.Unmarshal(body, tempResp)
			if err != nil {
				return nil, nil, nil, err
			}

			aggregate.Choices += tempResp.GetChoiceCount()
			aggregate.ToolCalls += tempResp.GetToolCount()

			if tempResp.Usage.CompletionTokens > 0 {
				aggregate.PromptTokens = tempResp.Usage.PromptTokens
				aggregate.CompletionTokens = tempResp.Usage.CompletionTokens
			}

			return llm, app, aggregate, nil
		}

	case models.ANTHROPIC:
		aggregate := &responses.GenericResponse{
			Choices: 1,
		}
		for _, body := range resps {
			tempResp := map[string]interface{}{}
			err := json.Unmarshal(body, tempResp)
			if err != nil {
				return nil, nil, nil, err
			}

			tp, ok := tempResp["type"]
			if !ok {
				switch tp {
				case "message_start":
					startMsg := &responses.AnthropicStreamingChunkStart{}
					err := json.Unmarshal(body, startMsg)
					if err != nil {
						return nil, nil, nil, err
					}

					aggregate.PromptTokens = startMsg.Message.Usage.InputTokens
				case "message_delta":
					deltaMsg := &responses.AnthropicStreamingChunkDelta{}
					err := json.Unmarshal(body, deltaMsg)
					if err != nil {
						return nil, nil, nil, err
					}

					aggregate.CompletionTokens += deltaMsg.Usage.OutputTokens

				case "content_block_start":
					startBlock := &responses.AnthropicStreamingChunkCBStart{}
					err := json.Unmarshal(body, startBlock)
					if err != nil {
						return nil, nil, nil, err
					}

					if startBlock.ContentBlock.Type == "tool_use" {
						aggregate.ToolCalls += 1
					}
				}
			}

			return llm, app, aggregate, nil
		}

	case models.MOCK_VENDOR:
		response = &responses.DummyResponse{}
		return llm, app, response, nil

	case models.HUGGINGFACE:
		// does not do token counts
		return llm, app, &responses.DummyResponse{
			Model: "huggingFace"}, nil

	case models.OLLAMA:
		return llm, app, &responses.DummyResponse{
			Model: "ollama"}, nil

	case models.VERTEX:
		return llm, app, &responses.DummyResponse{
			Model: "vertex"}, nil

	case models.GOOGLEAI:
		return llm, app, &responses.DummyResponse{
			Model: "google-ai"}, nil

	case "TestVendor":
		return llm, app, &responses.DummyResponse{
			Model: "test-vendor"}, nil

	}

	return nil, nil, nil, fmt.Errorf("[analyse response] unknown vendor: %s", llm.Vendor)
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
		return fmt.Errorf("[auth header] unknown vendor: %s", llm.Vendor)
	}

	return nil
}
