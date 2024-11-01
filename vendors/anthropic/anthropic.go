package anthropicVendor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/schema"
)

type Anthropic struct{}

func New() models.LLMVendorProvider {
	return &Anthropic{}
}

const AnthropicCompletionsEndpoint = "/v1/messages"

func (v *Anthropic) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	promptTokens := 0
	responseTokens := 0
	totalTokens := 0

	dat := choice.GenerationInfo
	promptTokens = helpers.KeyValueOrZero(dat, "InputTokens")
	responseTokens = helpers.KeyValueOrZero(dat, "OutputTokens")
	totalTokens = promptTokens + responseTokens

	return totalTokens, promptTokens, responseTokens
}

func (v *Anthropic) GetDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	llm, err := setupAnthropicDriver(LLMConfig, settings)
	if err != nil {
		return nil, err
	}

	return llm, nil
}

func (v *Anthropic) GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	return nil, nil
}

func (v *Anthropic) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	var response models.ITokenResponse
	if strings.Contains(strings.ToLower(r.URL.Path), AnthropicCompletionsEndpoint) {
		response = &responses.AnthropicResponse{}
		err := json.Unmarshal(body, response)
		if err != nil {
			return nil, nil, nil, err
		}

		return llm, app, response, nil
	}

	return llm, app, nil, fmt.Errorf("unknown response type")
}

func (v *Anthropic) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	aggregate := &responses.GenericResponse{
		Choices: 1,
	}

	asStr := string(resps)
	parts := strings.Split(asStr, "\n")
	for _, part := range parts {
		if part == "" || strings.Index(part, "event:") == 0 {
			continue
		}

		body := strings.TrimPrefix(part, "data:")

		tempResp := map[string]interface{}{}
		err := json.Unmarshal([]byte(body), &tempResp)
		if err != nil {
			return nil, nil, nil, err
		}

		tp, ok := tempResp["type"]
		if ok {
			switch tp {
			case "message_start":
				startMsg := &responses.AnthropicStreamingChunkStart{}
				err := json.Unmarshal([]byte(body), startMsg)
				if err != nil {
					return nil, nil, nil, err
				}

				aggregate.PromptTokens = startMsg.Message.Usage.InputTokens
				aggregate.Model = startMsg.Message.Model

			case "message_delta":
				deltaMsg := &responses.AnthropicStreamingChunkDelta{}
				err := json.Unmarshal([]byte(body), deltaMsg)
				if err != nil {
					return nil, nil, nil, err
				}

				aggregate.CompletionTokens += deltaMsg.Usage.OutputTokens

			case "content_block_start":
				startBlock := &responses.AnthropicStreamingChunkCBStart{}
				err := json.Unmarshal([]byte(body), startBlock)
				if err != nil {
					return nil, nil, nil, err
				}

				if startBlock.ContentBlock.Type == "tool_use" {
					aggregate.ToolCalls += 1
				}
			}
		}
	}

	return llm, app, aggregate, nil
}

func (v *Anthropic) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("x-api-key", llm.APIKey)
	return nil
}

func (v *Anthropic) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	b, err := helpers.CopyRequestBody(r)
	if err != nil {
		return err
	}

	var req responses.AnthropicRequest
	if err := json.Unmarshal(b, &req); err != nil {
		return err
	}

	if isStreamingChannel {
		if !req.Stream {
			return fmt.Errorf("streaming is required for this endpoint")
		}
		return nil
	}

	// not a streaming endpoint, but they are streaming
	if req.Stream {
		fmt.Println("streaming is not allowed for this endpoint")
		return fmt.Errorf("streaming is not allowed for this endpoint")
	}

	return nil
}

func (v *Anthropic) ProvidesEmbedder() bool {
	return false
}

func setupAnthropicDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]anthropic.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, anthropic.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, anthropic.WithToken(connDef.APIKey))
	}

	if llmSettings != nil {
		opts = append(opts, anthropic.WithModel(llmSettings.ModelName))
	}

	llm, err := anthropic.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic driver: %v", err)
	}

	return llm, nil
}
