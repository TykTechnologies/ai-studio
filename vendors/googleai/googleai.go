package googleaiVendor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/schema"
)

type GoogleAI struct{}

func New() models.LLMVendorProvider {
	return &GoogleAI{}
}

func (v *GoogleAI) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	promptTokens := helpers.KeyValueInt32OrZero(choice.GenerationInfo, "input_tokens")
	thinkingTokens := helpers.KeyValueInt32OrZero(choice.GenerationInfo, "ThinkingTokens")
	outputTokens := helpers.KeyValueInt32OrZero(choice.GenerationInfo, "output_tokens")

	// INFO: We have to include thoughts tokens as they're priced like response tokens
	// More details: https://ai.google.dev/gemini-api/docs/thinking#go_5
	responseTokens := outputTokens + thinkingTokens

	cacheTokens := helpers.KeyValueInt32OrZero(choice.GenerationInfo, "cached_content_tokens")
	totalTokens := promptTokens + responseTokens + cacheTokens

	return totalTokens, promptTokens, responseTokens
}

func (v *GoogleAI) GetDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	llm, err := setupGoogleDriver(LLMConfig, settings)
	if err != nil {
		return nil, err
	}

	return llm, nil
}

func (v *GoogleAI) GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	var llm embeddings.EmbedderClient
	var err error

	llm, err = setupGoogleAIEmbedClient(d)
	if err != nil {
		return nil, err
	}

	e, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (v *GoogleAI) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	response := &responses.GoogleAIChatResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, nil, nil, err
	}

	// Prefer modelVersion from the response body (most accurate)
	modelName := response.ModelVersion
	if modelName == "" {
		modelName, _ = extractModelIDFromGoogleURL(r.URL.Path)
	}
	if modelName == "" {
		modelName = "googleai-gemini-no-id"
	}

	response.SetModel(modelName)
	return llm, app, response, nil
}

func (v *GoogleAI) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	var streamChunks []responses.GoogleAIStreamChunk

	// Try SSE format first (when alt=sse is used, which is the standard for streaming)
	if isSSEFormat(resps) {
		streamChunks = parseSSEGoogleAIChunks(resps)
	} else {
		// Try JSON array format (non-SSE streaming)
		err := json.Unmarshal(resps, &streamChunks)
		if err != nil {
			// Fallback: try single JSON object
			var singleChunk responses.GoogleAIStreamChunk
			if singleErr := json.Unmarshal(resps, &singleChunk); singleErr == nil {
				streamChunks = append(streamChunks, singleChunk)
			} else {
				return nil, nil, nil, fmt.Errorf("failed to unmarshal googleai streaming response: %w", err)
			}
		}
	}

	if len(streamChunks) == 0 {
		return nil, nil, nil, errors.New("googleai streaming response contained no chunks")
	}

	finalChunk := streamChunks[len(streamChunks)-1]

	// Prefer modelVersion from the response itself (most accurate)
	modelName := finalChunk.ModelVersion
	if modelName == "" {
		modelName, _ = extractModelIDFromGoogleURL(r.URL.Path)
	}
	if modelName == "" {
		modelName = "googleai-gemini-no-id"
	}

	aggregate := &responses.GenericResponse{
		Choices: 1,
		Model:   modelName,
	}
	aggregate.PromptTokens = finalChunk.UsageMetadata.PromptTokenCount
	aggregate.CompletionTokens = finalChunk.UsageMetadata.CandidatesTokenCount +
		finalChunk.UsageMetadata.ThoughtsTokenCount

	return llm, app, aggregate, nil
}

// isSSEFormat checks if the response data is in SSE (Server-Sent Events) format.
func isSSEFormat(data []byte) bool {
	// Check first non-empty line for "data:" prefix
	for _, line := range strings.SplitN(string(data), "\n", 5) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, "data:")
	}
	return false
}

// parseSSEGoogleAIChunks parses SSE-formatted streaming response into GoogleAIStreamChunks.
// Google's streamGenerateContent with alt=sse returns:
//
//	data: {"candidates":[...],"usageMetadata":{...},"modelVersion":"gemini-2.5-pro"}
//	data: {"candidates":[...],"usageMetadata":{...},"modelVersion":"gemini-2.5-pro"}
func parseSSEGoogleAIChunks(data []byte) []responses.GoogleAIStreamChunk {
	var chunks []responses.GoogleAIStreamChunk
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		jsonData := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if jsonData == "" || jsonData == "[DONE]" {
			continue
		}
		var chunk responses.GoogleAIStreamChunk
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

// ProxySetAuthHeader injects the required Google AI authentication credentials into the
// outgoing request. It modifies the request in-place, ensuring the API key is present
// in both the 'x-goog-api-key' header and the 'key' query parameter
// to satisfy different versions of the Vertex/Gemini APIs.
func (v *GoogleAI) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("x-goog-api-key", llm.APIKey)

	q := r.URL.Query()
	if q.Get("key") != llm.APIKey {
		q.Set("key", llm.APIKey)
		r.URL.RawQuery = q.Encode()
	}

	return nil
}

func (v *GoogleAI) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	isStream := false
	if strings.Contains(strings.ToLower(r.URL.Path), ":streamgeneratecontent") {
		isStream = true
	}

	if isStreamingChannel {
		if !isStream {
			return fmt.Errorf("streaming is required for this endpoint")
		}
		return nil
	}

	// not a streaming endpoint, but they are streaming
	if isStream {
		return fmt.Errorf("streaming is not allowed for this endpoint")
	}

	return nil
}

func (v *GoogleAI) ProvidesEmbedder() bool {
	return true
}

func setupGoogleDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]googleai.Option, 0)
	if connDef.APIKey != "" {
		opts = append(opts, googleai.WithAPIKey(connDef.APIKey))
	}

	if llmSettings != nil {
		opts = append(opts, googleai.WithDefaultModel(llmSettings.ModelName))
	}
	llm, err := googleai.New(context.Background(), opts...)

	if err != nil {
		return nil, fmt.Errorf("failed to create google_ai driver: %v", err)
	}

	return llm, nil
}

func setupGoogleAIEmbedClient(d *models.Datasource) (embeddings.EmbedderClient, error) {
	var opts = make([]googleai.Option, 0)
	if d.EmbedAPIKey != "" {
		opts = append(opts, googleai.WithAPIKey(d.EmbedAPIKey))
	}

	opts = append(opts, googleai.WithDefaultEmbeddingModel(d.EmbedModel))

	llm, err := googleai.New(context.Background(), opts...)

	if err != nil {
		return nil, fmt.Errorf("failed to create google_ai driver: %v", err)
	}

	return llm, nil
}

func extractModelIDFromGoogleURL(url string) (string, error) {
	// Regular expression pattern to match the MODEL-ID in the new URL format
	pattern := `/publishers/google/models/([^/:]+)`

	// Compile the regular expression
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to compile regex: %v", err)
	}

	// Find the first match in the URL
	match := re.FindStringSubmatch(url)

	if len(match) > 1 {
		// If a match is found, return the captured group (MODEL-ID)
		return match[1], nil
	}

	// If no match is found, try different pattern
	return extractModelIDFromGoogleURLAlternate(url)
}

func extractModelIDFromGoogleURLAlternate(url string) (string, error) {
	// Regular expression pattern to match the MODEL-ID in the new URL format
	pattern := `/v1(?:beta)?/models/([^/:]+)`

	// Compile the regular expression
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to compile regex: %v", err)
	}

	// Find the first match in the URL
	match := re.FindStringSubmatch(url)

	if len(match) > 1 {
		// If a match is found, return the captured group (MODEL-ID)
		return match[1], nil
	}

	// If no match is found, return an error
	return "", fmt.Errorf("model ID not found in URL")
}
