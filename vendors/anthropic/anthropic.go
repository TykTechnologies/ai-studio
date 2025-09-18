package anthropicVendor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/sirupsen/logrus"
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
	cacheWriteTokens := helpers.KeyValueOrZero(dat, "CacheCreationInputTokens")
	cacheReadTokens := helpers.KeyValueOrZero(dat, "CacheReadInputTokens")
	totalTokens = promptTokens + responseTokens + cacheWriteTokens + cacheReadTokens

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

	return llm, app, nil, fmt.Errorf("unknown response type: %s", r.URL.Path)
}

func (v *Anthropic) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	aggregate := &responses.GenericResponse{
		Choices: 1,
	}

	logrus.WithFields(logrus.Fields{
		"status":   statusCode,
		"response": string(resps),
		"app_id":   app.ID,
		"llm_id":   llm.ID,
	}).Debug("Analyzing streaming response")

	var startMsg *responses.AnthropicStreamingChunkStart

	asStr := string(resps)
	parts := strings.Split(asStr, "\n")
	logrus.WithField("parts_count", len(parts)).Debug("Split response into parts")
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
				startMsg = &responses.AnthropicStreamingChunkStart{}
				err := json.Unmarshal([]byte(body), startMsg)
				if err != nil {
					return nil, nil, nil, err
				}

				logrus.WithFields(logrus.Fields{
					"input_tokens": startMsg.Message.Usage.InputTokens,
					"model":        startMsg.Message.Model,
				}).Debug("Processing message_start")

				aggregate.PromptTokens = startMsg.Message.Usage.InputTokens
				aggregate.Model = startMsg.Message.Model
				aggregate.CacheWritePromptTokens = startMsg.Message.Usage.CacheCreationInputTokens
				aggregate.CacheReadPromptTokens = startMsg.Message.Usage.CacheReadInputTokens

			case "message_delta":
				deltaMsg := &responses.AnthropicStreamingChunkDelta{}
				err := json.Unmarshal([]byte(body), deltaMsg)
				if err != nil {
					return nil, nil, nil, err
				}

				logrus.WithField("output_tokens", deltaMsg.Usage.OutputTokens).Debug("Processing message_delta")

				// For streaming, we need to add both the initial output token from message_start
				// and the delta output tokens
				if startMsg != nil && aggregate.CompletionTokens == 0 {
					aggregate.CompletionTokens = startMsg.Message.Usage.OutputTokens
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

	logrus.WithFields(logrus.Fields{
		"prompt_tokens":   aggregate.PromptTokens,
		"response_tokens": aggregate.CompletionTokens,
		"model":           aggregate.Model,
		"time":            time.Now(),
	}).Debug("Returning aggregate response")

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
