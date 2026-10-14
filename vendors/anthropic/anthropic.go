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
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/httputil"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/anthropic"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
	"github.com/sirupsen/logrus"
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

func (v *Anthropic) GetEmbedder(spec *models.EmbedderSpec) (*embeddings.EmbedderImpl, error) {
	return nil, nil
}

func (v *Anthropic) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	var response models.ITokenResponse
	pathLower := strings.TrimSuffix(strings.ToLower(r.URL.Path), "/")
	// Only a Messages call carries a message: /v1/messages (direct API calls)
	// and a /messages suffix (SDK internal routing via /ai/). The suffix match
	// keeps /v1/messages/batches* and /v1/messages/count_tokens out: they
	// carry no usage and used to log as zero-cost chat records.
	if strings.HasSuffix(pathLower, "/messages") {
		response = &responses.AnthropicResponse{}
		err := json.Unmarshal(body, response)
		if err != nil {
			return nil, nil, nil, err
		}

		return llm, app, response, nil
	}
	// The other Messages API calls (count_tokens, batches) are proxied
	// calls without message usage: no response and no error, so the proxy
	// logs the call without a chat record.
	if strings.Contains(pathLower, AnthropicCompletionsEndpoint+"/") {
		return llm, app, nil, nil
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
				// Until a message_delta arrives, message_start's count is all we have.
				aggregate.CompletionTokens = startMsg.Message.Usage.OutputTokens

			case "message_delta":
				deltaMsg := &responses.AnthropicStreamingChunkDelta{}
				err := json.Unmarshal([]byte(body), deltaMsg)
				if err != nil {
					return nil, nil, nil, err
				}

				logrus.WithField("output_tokens", deltaMsg.Usage.OutputTokens).Debug("Processing message_delta")

				// message_delta's usage is cumulative: its output_tokens is the
				// running total for the turn, already including the tokens
				// message_start reported. Take it, never add it.
				if deltaMsg.Usage.OutputTokens > 0 {
					aggregate.CompletionTokens = deltaMsg.Usage.OutputTokens
				}

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

// Workspace and user-profile headers. The workspace is the LLM's to pin
// (models.LLM.VendorWorkspaceID); a client never picks it.
const (
	headerWorkspaceID   = "anthropic-workspace-id"
	headerUserProfileID = "anthropic-user-profile-id"
)

func (v *Anthropic) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("x-api-key", llm.APIKey)
	r.Header.Del(headerWorkspaceID)
	r.Header.Del(headerUserProfileID)
	if llm.VendorWorkspaceID != "" {
		r.Header.Set(headerWorkspaceID, llm.VendorWorkspaceID)
	}
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
			return fmt.Errorf("streaming is required for this endpoint: %s", r.URL.String())
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

func (v *Anthropic) ProvidesChat() bool {
	return true
}

func (v *Anthropic) DecisionEndpoint() string {
	return ""
}

// DriverBaseURL returns the base URL to hand the langchaingo Anthropic client
// for a configured endpoint. That client appends "/messages" to its base URL
// (its own default is https://api.anthropic.com/v1), so an endpoint configured
// without a version segment, whether the bare host or a proxy prefix such as
// https://gw/anthropic, would be called at /messages and answered 404. The
// Messages API only exists under /v1, so a version segment is appended unless
// the endpoint already ends in one. An empty endpoint is left empty so the
// client falls back to its default.
func DriverBaseURL(endpoint string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if trimmed == "" {
		return ""
	}
	last := trimmed[strings.LastIndex(trimmed, "/")+1:]
	if isVersionSegment(last) {
		return trimmed
	}
	return trimmed + "/v1"
}

// isVersionSegment reports whether a path segment names an API version (v1,
// v2, v1beta, ...).
func isVersionSegment(seg string) bool {
	if len(seg) < 2 || seg[0] != 'v' || seg[1] < '0' || seg[1] > '9' {
		return false
	}
	return true
}

// HTTPDoer is the HTTP client the langchaingo Anthropic driver calls.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// workspaceDoer sets the LLM's pinned workspace on every driver request.
type workspaceDoer struct {
	next      HTTPDoer
	workspace string
}

func (d workspaceDoer) Do(r *http.Request) (*http.Response, error) {
	r.Header.Del(headerUserProfileID)
	r.Header.Set(headerWorkspaceID, d.workspace)
	return d.next.Do(r)
}

// PinWorkspace returns client wrapped so that the langchaingo driver's calls
// (chat sessions, the translation bridges, semantic routing) run in llm's
// pinned workspace, as the proxy's calls do (ProxySetAuthHeader). Without a
// pin it returns client unchanged.
func PinWorkspace(client HTTPDoer, llm *models.LLM) HTTPDoer {
	if llm == nil || llm.VendorWorkspaceID == "" {
		return client
	}
	return workspaceDoer{next: client, workspace: llm.VendorWorkspaceID}
}

func setupAnthropicDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]anthropic.Option, 0)
	if connDef.VendorWorkspaceID != "" {
		opts = append(opts, anthropic.WithHTTPClient(PinWorkspace(httputil.DefaultClient, connDef)))
	}
	if baseURL := DriverBaseURL(connDef.APIEndpoint); baseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(baseURL))
	}

	// Always passed, even when empty: the client must refuse an empty key,
	// never fill it from the host environment.
	opts = append(opts, anthropic.WithToken(connDef.APIKey))

	if llmSettings != nil {
		opts = append(opts, anthropic.WithModel(llmSettings.ModelName))
	}

	llm, err := anthropic.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic driver: %v", err)
	}

	return llm, nil
}
