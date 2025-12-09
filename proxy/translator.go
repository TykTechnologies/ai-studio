package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
)

// getInternalLLMBaseURL returns the internal /llm/call/ URL for SDK endpoint hijacking.
// When the /ai/ endpoint routes requests through /llm/, this URL points to the local proxy.
// The SDK handles vendor-specific path suffixes automatically (e.g., /v1/messages for Anthropic).
// IMPORTANT: Different SDKs expect different base URL formats:
// - OpenAI SDK expects base URL with /v1 (e.g., http://host/llm/call/openai/v1) and appends /chat/completions
// - Anthropic SDK expects base URL without version (e.g., http://host/llm/call/claude) and appends /v1/messages
func (p *Proxy) getInternalLLMBaseURL(slug string, vendor models.Vendor) string {
	baseURL := fmt.Sprintf("http://127.0.0.1:%d/llm/call/%s", p.config.Port, slug)

	// OpenAI and OpenAI-compatible SDKs expect the base URL to include /v1
	// They then append /chat/completions or /completions directly
	switch vendor {
	case models.OPENAI, models.OLLAMA:
		return baseURL + "/v1"
	default:
		// Other vendors (Anthropic, Google, etc.) handle their own path construction
		return baseURL
	}
}

// Handlers
func (p *Proxy) CreateCompletionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["routeId"]

	// get the route ID from the DB to find out what back-end LLM to use
	conf, ok := p.llms[routeID]
	if !ok {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	var req CreateCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate model
	validator := NewModelValidator(conf.AllowedModels)
	if !validator.IsModelAllowed(req.Model) {
		respondWithOAIError(w, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed", req.Model), nil, false)
		return
	}

	// create a standard llangchain completion request based on the input
	llm, err := switches.FetchDriver(conf, nil, nil, func(ctx context.Context, chunk []byte) error { return nil })
	if err != nil {
		http.Error(w, "Failed to create LLM client", http.StatusInternalServerError)
		return
	}

	// send the request to the LLM
	ctx := context.Background()
	// 1. create the options
	opts := handleOptions(&req)

	if req.Stream != nil {
		http.Error(w, "Streaming is not supported", http.StatusBadRequest)
		return
	}

	// 3. call the LLM
	log.Warn().Msg("completions API is deprecated, use the /v1/chat/completions API instead (no analytics stored)")
	resp, err := llm.Call(ctx, req.Prompt, opts...)

	// convert the response to OpenAI format
	response := CompletionResponse{
		ID: "completion-" + uuid.New().String(),
		Choices: []CompletionChoice{
			{
				Text:         resp,
				FinishReason: "stop",
			},
		},
		Model: req.Model,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (p *Proxy) CreateChatCompletionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["routeId"]

	// get the route ID from the DB to find out what back-end LLM to use
	conf, ok := p.llms[routeID]
	if !ok {
		respondWithOAIError(w, http.StatusNotFound, "vendor not found", nil, false)
		return
	}

	// Capture request body for decoding
	reqBody, err := helpers.CopyRequestBody(r)
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "Failed to read request body", err, false)
		return
	}

	// NOTE: This is a PURE BRIDGE HANDLER - NO AUTH runs here
	// Auth, plugins, budget checking, analytics all happen on the internal /llm/call/ hop
	// We just translate OpenAI format -> vendor format and route internally

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithOAIError(w, http.StatusBadRequest, "Invalid request body", err, false)
		return
	}

	// Validate model (keep this here for fast-fail before internal routing)
	validator := NewModelValidator(conf.AllowedModels)
	if !validator.IsModelAllowed(req.Model) {
		respondWithOAIError(w, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed", req.Model), nil, false)
		return
	}

	// Validate required fields
	if len(req.Messages) == 0 || req.Model == "" {
		respondWithOAIError(w, http.StatusBadRequest, "missing required fields", nil, false)
		return
	}

	// Handle streaming if requested
	if req.Stream != nil && *req.Stream {
		// Fall back to non-streaming if tools are present (tool streaming is complex)
		if len(req.Tools) == 0 {
			p.handleChatCompletionStream(w, r, conf, &req, reqBody)
			return
		}
		// Continue to non-streaming path when tools are present
	}

	// Create internal routing HTTP client
	// This routes SDK requests through /llm/call/ for plugin hook execution
	originalAuth := r.Header.Get("Authorization")
	internalTransport := NewInternalRoutingTransport(originalAuth)
	internalClient := &http.Client{Transport: internalTransport}

	// Create a modified LLM config with internal endpoint
	// The SDK will route to /llm/call/{slug} instead of the external vendor
	llmSlug := slug.Make(conf.Name)
	internalConf := *conf // Copy config
	internalConf.APIEndpoint = p.getInternalLLMBaseURL(llmSlug, conf.Vendor)
	// Set a dummy API key to satisfy SDK validation (actual auth handled by /llm/)
	// The InternalRoutingTransport strips SDK-set auth headers and passes client auth instead
	internalConf.APIKey = "internal-routing-dummy-key"

	// DEBUG: Log the internal routing setup
	log.Debug().
		Str("llmSlug", llmSlug).
		Str("internalEndpoint", internalConf.APIEndpoint).
		Str("vendor", string(internalConf.Vendor)).
		Int("messageCount", len(req.Messages)).
		Msg("CreateChatCompletionHandler internal routing")

	// Create LLM driver with internal routing HTTP client
	llm, err := switches.FetchDriver(&internalConf, nil, nil, func(ctx context.Context, chunk []byte) error { return nil }, switches.WithHTTPClient(internalClient))
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "Failed to create LLM client", err, false)
		return
	}

	ctx := context.Background()
	opts := req.ToLangchainOptions(&internalConf)
	messages := req.GetMessages()

	// SDK call routes through /llm/call/ which executes all plugin hooks
	// Auth, plugins, budget, analytics all happen on the /llm/call/ hop
	resp, err := llm.GenerateContent(ctx, messages, opts...)
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "failed to generate content", err, false)
		return
	}

	// Extract token usage from ContentResponse
	usage := extractTokenUsageFromContentResponse(resp, conf.Vendor)

	// Create response with usage field populated
	response := NewChatCompletionResponse(resp, req.Model)
	response.Usage = usage

	// Marshal response
	respBody, err := json.Marshal(response)
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "Failed to marshal response", err, false)
		return
	}

	// Send response to client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

func handleOptions(req *CreateCompletionRequest) []llms.CallOption {
	opts := make([]llms.CallOption, 0)
	if req.MaxTokens != nil {
		opts = append(opts, llms.WithMaxTokens(*req.MaxTokens))
	}

	if req.Temperature != nil {
		opts = append(opts, llms.WithTemperature(*req.Temperature))
	}

	if req.TopP != nil {
		opts = append(opts, llms.WithTopP(*req.TopP))
	}

	if req.PresencePenalty != nil {
		opts = append(opts, llms.WithPresencePenalty(*req.PresencePenalty))
	}

	if req.FrequencyPenalty != nil {
		opts = append(opts, llms.WithFrequencyPenalty(*req.FrequencyPenalty))
	}

	if req.Model != "" {
		opts = append(opts, llms.WithModel(req.Model))
	}

	if req.Stop != "" {
		stopWords := []string{}
		switch req.Stop.(type) {
		case string:
			stopWords = append(stopWords, req.Stop.(string))
		case []string:
			stopWords = req.Stop.([]string)
		}

		opts = append(opts, llms.WithStopWords(stopWords))
	}

	return opts
}

// extractTokenUsageFromContentResponse extracts token usage from langchaingo ContentResponse
func extractTokenUsageFromContentResponse(resp *llms.ContentResponse, vendor models.Vendor) CompletionUsage {
	if resp == nil || len(resp.Choices) == 0 {
		return CompletionUsage{}
	}

	totalPrompt := 0
	totalCompletion := 0

	// Sum tokens across all choices (usually just one)
	for _, choice := range resp.Choices {
		_, prompt, completion := switches.GetTokenCounts(choice, vendor)
		totalPrompt += prompt
		totalCompletion += completion
	}

	return CompletionUsage{
		PromptTokens:     totalPrompt,
		CompletionTokens: totalCompletion,
		TotalTokens:      totalPrompt + totalCompletion,
	}
}

// getAppFromContext retrieves the App from request context (set by credential validator middleware)
func (p *Proxy) getAppFromContext(r *http.Request, llm *models.LLM) (*models.App, error) {
	// Try context first (from credential validator middleware)
	if appObj := r.Context().Value("app"); appObj != nil {
		if app, ok := appObj.(*models.App); ok {
			return app, nil
		}
	}

	// If not in context, authentication likely failed
	return nil, fmt.Errorf("app context not found - authentication required")
}

// countToolCalls counts total tool calls across all choices
func countToolCalls(resp *llms.ContentResponse) int {
	if resp == nil {
		return 0
	}
	count := 0
	for _, choice := range resp.Choices {
		count += len(choice.ToolCalls)
	}
	return count
}

// recordTranslatorAnalytics records analytics and proxy logs for /ai/ endpoint requests
func (p *Proxy) recordTranslatorAnalytics(
	llm *models.LLM,
	app *models.App,
	statusCode int,
	respBody []byte,
	reqBody []byte,
	r *http.Request,
	contentResp *llms.ContentResponse,
	timestamp time.Time,
) {
	const maxBodySize = 65535 // Maximum size for TEXT column (64KB)

	// 1. Record proxy log
	proxyLog := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(respBody), maxBodySize),
		ResponseCode: statusCode,
	}
	analytics.RecordProxyLog(proxyLog)

	// 2. Record chat analytics (if successful)
	if statusCode == http.StatusOK && contentResp != nil {
		recordTranslatorChatAnalytics(p.gatewayService, llm, app, contentResp, r, timestamp)
	}
}

// handleChatCompletionStream handles streaming chat completion requests with OpenAI-compatible SSE format
// NOTE: This is a PURE BRIDGE HANDLER - NO AUTH runs here
// Auth, plugins, budget checking, analytics all happen on the internal /llm/call/ hop
func (p *Proxy) handleChatCompletionStream(
	w http.ResponseWriter,
	r *http.Request,
	conf *models.LLM,
	req *ChatCompletionRequest,
	reqBody []byte,
) {
	// Set SSE headers before any writes
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Check if we can flush
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondWithOAIError(w, http.StatusInternalServerError, "Streaming not supported", nil, false)
		return
	}

	// Generate completion ID and timestamp for all chunks
	completionID := "chatcmpl-" + uuid.New().String()
	created := time.Now().Unix()
	isFirstChunk := true

	// Create streaming callback that formats chunks as OpenAI SSE events
	streamingFunc := func(ctx context.Context, chunk []byte) error {
		chunkResp := ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   req.Model,
			Choices: []ChatCompletionChunkChoice{{
				Index:        0,
				Delta:        ChatCompletionDelta{},
				FinishReason: nil,
			}},
		}

		// First chunk includes role
		if isFirstChunk {
			chunkResp.Choices[0].Delta.Role = "assistant"
			isFirstChunk = false
		}

		// Add content to delta
		chunkResp.Choices[0].Delta.Content = string(chunk)

		// Marshal and send
		jsonBytes, err := json.Marshal(chunkResp)
		if err != nil {
			return fmt.Errorf("failed to marshal chunk: %w", err)
		}

		fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
		flusher.Flush()
		return nil
	}

	// Create internal routing HTTP client
	// This routes SDK requests through /llm/call/ for plugin hook execution
	originalAuth := r.Header.Get("Authorization")
	internalTransport := NewInternalRoutingTransport(originalAuth)
	internalClient := &http.Client{Transport: internalTransport}

	// Create a modified LLM config with internal endpoint
	llmSlug := slug.Make(conf.Name)
	internalConf := *conf // Copy config
	internalConf.APIEndpoint = p.getInternalLLMBaseURL(llmSlug, conf.Vendor)
	// Set a dummy API key to satisfy SDK validation (actual auth handled by /llm/)
	// The InternalRoutingTransport strips SDK-set auth headers and passes client auth instead
	internalConf.APIKey = "internal-routing-dummy-key"

	// Create LLM driver with internal routing HTTP client and streaming callback
	llmDriver, err := switches.FetchDriver(&internalConf, nil, nil, streamingFunc, switches.WithHTTPClient(internalClient))
	if err != nil {
		p.sendStreamError(w, flusher, "Failed to create LLM client", "server_error")
		return
	}

	ctx := context.Background()
	opts := req.ToLangchainOptions(&internalConf)
	// Add streaming function to options
	opts = append(opts, llms.WithStreamingFunc(streamingFunc))
	messages := req.GetMessages()

	// SDK call routes through /llm/call/ which executes all plugin hooks
	// Auth, plugins, budget, analytics all happen on the /llm/call/ hop
	resp, err := llmDriver.GenerateContent(ctx, messages, opts...)
	if err != nil {
		p.sendStreamError(w, flusher, fmt.Sprintf("LLM call failed: %s", err.Error()), "server_error")
		return
	}

	// Send final chunk with finish_reason and usage
	usage := extractTokenUsageFromContentResponse(resp, conf.Vendor)
	finishReason := "stop"
	if len(resp.Choices) > 0 && resp.Choices[0].StopReason != "" {
		finishReason = convertFinishReason(resp.Choices[0].StopReason)
	}

	finalChunk := ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   req.Model,
		Choices: []ChatCompletionChunkChoice{{
			Index:        0,
			Delta:        ChatCompletionDelta{}, // Empty delta for final chunk
			FinishReason: &finishReason,
		}},
		Usage: &usage,
	}

	jsonBytes, err := json.Marshal(finalChunk)
	if err == nil {
		fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
		flusher.Flush()
	}

	// Send [DONE] marker
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// sendStreamError sends an error in SSE format
func (p *Proxy) sendStreamError(w http.ResponseWriter, flusher http.Flusher, message, errorType string) {
	errResp := ChatCompletionStreamError{
		Error: ChatCompletionErrorDetail{
			Message: message,
			Type:    errorType,
		},
	}
	jsonBytes, _ := json.Marshal(errResp)
	fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
	flusher.Flush()
}

// recordTranslatorChatAnalytics records detailed chat analytics for /ai/ endpoint requests
func recordTranslatorChatAnalytics(
	service services.ServiceInterface,
	llm *models.LLM,
	app *models.App,
	contentResp *llms.ContentResponse,
	r *http.Request,
	timestamp time.Time,
) {
	// Extract token counts
	var promptTokens, responseTokens, totalTokens int
	for _, choice := range contentResp.Choices {
		tt, pt, rt := switches.GetTokenCounts(choice, llm.Vendor)
		totalTokens += tt
		promptTokens += pt
		responseTokens += rt
	}

	// Get model name from context or request
	model := ""
	if modelFromCtx := r.Context().Value("model_name"); modelFromCtx != nil {
		if modelStr, ok := modelFromCtx.(string); ok {
			model = modelStr
		}
	}

	// Get pricing information
	var cpt, cpit float64
	var currency string = "USD"
	price, err := service.GetModelPriceByModelNameAndVendor(model, string(llm.Vendor))
	if err == nil && price != nil {
		cpt = price.CPT
		cpit = price.CPIT
		currency = price.Currency
	}

	// Record analytics
	rec := &models.LLMChatRecord{
		LLMID:           llm.ID,
		Name:            model,
		Vendor:          string(llm.Vendor),
		PromptTokens:    promptTokens,
		ResponseTokens:  responseTokens,
		TotalTokens:     totalTokens,
		TimeStamp:       timestamp,
		Choices:         len(contentResp.Choices),
		ToolCalls:       countToolCalls(contentResp),
		AppID:           app.ID,
		UserID:          app.UserID,
		Cost:            ((cpt * float64(responseTokens)) + (cpit * float64(promptTokens))) * 10000,
		Currency:        currency,
		InteractionType: models.ProxyInteraction,
	}

	analytics.RecordChatRecord(rec)

	// Budget analysis
	if s, ok := service.(*services.Service); ok && s.Budget != nil {
		s.Budget.AnalyzeBudgetUsage(app, llm)
	} else if budgetService, ok := service.(interface {
		GetBudgetService() services.BudgetService
	}); ok {
		if bs := budgetService.GetBudgetService(); bs != nil {
			bs.AnalyzeBudgetUsage(app, llm)
		}
	}
}
