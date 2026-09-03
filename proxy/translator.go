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
	// (GetLLM takes the read lock; Reload writes this map concurrently)
	conf, ok := p.GetLLM(routeID)
	if !ok {
		respondWithOAIError(w, http.StatusNotFound, fmt.Sprintf("vendor '%s' not found or not supported by your access rights", routeID), nil, false)
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
	// (GetLLM takes the read lock; Reload writes this map concurrently)
	conf, ok := p.GetLLM(routeID)
	if !ok {
		respondWithOAIError(w, http.StatusNotFound, fmt.Sprintf("vendor '%s' not found or not supported by your access rights", routeID), nil, false)
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

	// Bedrock uses direct AWS SDK calls (no internal routing)
	if conf.Vendor == models.BEDROCK {
		if req.Stream != nil && *req.Stream {
			p.handleBedrockChatCompletionStream(w, r, conf, &req, reqBody)
		} else {
			p.handleBedrockChatCompletion(w, r, conf, &req, reqBody)
		}
		return
	}

	// Handle streaming if requested. Tools no longer divert to the buffered
	// path: a client that asked for a stream and got Content-Type
	// application/json never sees a token and has no idea why.
	if req.Stream != nil && *req.Stream {
		p.handleChatCompletionStream(w, r, conf, &req, reqBody)
		return
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
		// Surface the vendor's own status. An unknown model is a 404 upstream;
		// reporting it as our 500 tells the caller to retry a request that can
		// never succeed, and hides a client error as a server one.
		status, _ := upstreamStatusFromError(err, http.StatusBadGateway)
		respondWithOAIError(w, status, "failed to generate content", err, false)
		return
	}

	// Extract token usage from ContentResponse
	usage := extractTokenUsageFromContentResponse(resp, conf.Vendor, req.CompletionCount())

	// Create response with usage field populated
	response := NewChatCompletionResponse(resp, req.Model, req.CompletionCount())
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

// extractTokenUsageFromContentResponse extracts token usage from langchaingo ContentResponse.
//
// n is the number of completions the caller asked for. Choices beyond that are
// content blocks of a single turn (see NewChatCompletionResponse), and every
// block carries the same whole-response counters - summing them reported double
// the tokens the vendor actually charged for on Anthropic.
func extractTokenUsageFromContentResponse(resp *llms.ContentResponse, vendor models.Vendor, n int) CompletionUsage {
	if resp == nil || len(resp.Choices) == 0 {
		return CompletionUsage{}
	}

	totalPrompt := 0
	totalCompletion := 0

	// One reading per requested completion, not per content block.
	for _, group := range groupContentChoices(resp.Choices, n) {
		_, prompt, completion := switches.GetTokenCounts(group[0], vendor)
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
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(respBody), maxBodySize),
		ResponseCode: statusCode,
	}
	if llm.DontLogBodies {
		proxyLog.RequestBody = ""
		proxyLog.ResponseBody = ""
	}
	analytics.RecordProxyLog(r.Context(), proxyLog)

	// 2. Record chat analytics (if successful)
	if statusCode == http.StatusOK && contentResp != nil {
		recordTranslatorChatAnalytics(p.gatewayService, llm, app, contentResp, r, timestamp)
	}
}

// handleChatCompletionStream handles streaming chat completion requests with OpenAI-compatible SSE format
// NOTE: This is a PURE BRIDGE HANDLER - NO AUTH runs here
// Auth, plugins, budget checking, analytics all happen on the internal /llm/call/ hop
//
// Tool calls are streamed too. langchaingo hands text to the streaming callback
// as it arrives but accumulates tool-call fragments internally, so the calls are
// only known once GenerateContent returns; they are framed as tool_calls deltas
// before the terminal chunk. The client sees a well-formed OpenAI stream either
// way, which is what it dispatches on.
func (p *Proxy) handleChatCompletionStream(
	w http.ResponseWriter,
	r *http.Request,
	conf *models.LLM,
	req *ChatCompletionRequest,
	reqBody []byte,
) {
	// Check if we can flush before promising a stream.
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondWithOAIError(w, http.StatusInternalServerError, "Streaming not supported", nil, false)
		return
	}

	// Set SSE headers before any writes. Nothing is committed until the first
	// write, so a failure that happens before then can still be answered with a
	// real HTTP status and an OpenAI error envelope rather than a 200 whose
	// single frame says something went wrong.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Generate completion ID and timestamp for all chunks
	completionID := "chatcmpl-" + uuid.New().String()
	created := time.Now().Unix()
	framesSent := 0

	newChunk := func(delta ChatCompletionDelta) ChatCompletionChunk {
		// delta.role belongs on the first frame and nowhere else: clients that
		// build a message from the stream treat a repeat as a new message.
		if framesSent == 0 {
			delta.Role = "assistant"
		}
		return ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   req.Model,
			Choices: []ChatCompletionChunkChoice{{
				Index:        0,
				Delta:        delta,
				FinishReason: nil,
			}},
		}
	}

	send := func(chunk ChatCompletionChunk) error {
		jsonBytes, err := json.Marshal(chunk)
		if err != nil {
			return fmt.Errorf("failed to marshal chunk: %w", err)
		}
		fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
		flusher.Flush()
		framesSent++
		return nil
	}

	// Create streaming callback that formats chunks as OpenAI SSE events
	streamingFunc := func(ctx context.Context, chunk []byte) error {
		return send(newChunk(ChatCompletionDelta{Content: string(chunk)}))
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
		p.failStream(w, flusher, framesSent, http.StatusInternalServerError, "Failed to create LLM client", err)
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
		status, _ := upstreamStatusFromError(err, http.StatusBadGateway)
		p.failStream(w, flusher, framesSent, status, "LLM call failed", err)
		return
	}

	// Fold the driver's per-content-block choices back into the single turn the
	// caller asked for, exactly as the non-streaming path does.
	groups := groupContentChoices(resp.Choices, req.CompletionCount())
	usage := extractTokenUsageFromContentResponse(resp, conf.Vendor, req.CompletionCount())

	finishReason := "stop"
	toolCalls := 0
	if len(groups) > 0 {
		merged := newChatCompletionChoice(0, groups[0])
		finishReason = merged.FinishReason
		toolCalls = len(merged.Message.ToolCalls)

		// A tool-only turn never reaches the streaming callback, so this may be
		// the first frame of the stream - which is why delta.role is decided by
		// framesSent rather than assumed to have gone out with the text.
		for i, call := range merged.Message.ToolCalls {
			delta := ChatCompletionDelta{ToolCalls: []ChatCompletionToolCallDelta{
				toolCallDelta(i, call),
			}}
			if err := send(newChunk(delta)); err != nil {
				log.Error().Err(err).Msg("failed to send tool call chunk")
				break
			}
		}
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
	if framesSent == 0 {
		// Nothing at all came back. The terminal frame is the only one the
		// client will see, so the role has to ride on it.
		finalChunk.Choices[0].Delta.Role = "assistant"
	}
	_ = send(finalChunk)

	// Send [DONE] marker
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	log.Debug().
		Str("vendor", string(conf.Vendor)).
		Int("tool_calls", toolCalls).
		Int("frames", framesSent).
		Msg("streamed chat completion")
}

// toolCallDelta renders one fully-accumulated tool call as a streamed fragment.
// index is required on the wire: it is the only thing a client can use to tie
// argument fragments back to the call they belong to.
func toolCallDelta(index int, call map[string]interface{}) ChatCompletionToolCallDelta {
	out := ChatCompletionToolCallDelta{Index: index, Type: "function"}
	if id, ok := call["id"].(string); ok {
		out.ID = id
	}
	if t, ok := call["type"].(string); ok && t != "" {
		out.Type = t
	}
	fn, _ := call["function"].(map[string]interface{})
	name, _ := fn["name"].(string)
	args, _ := fn["arguments"].(string)
	if args == "" {
		// An empty string does not parse; clients that json.Unmarshal the
		// concatenation would get "unexpected end of JSON input".
		args = "{}"
	}
	out.Function = &ChatCompletionFunctionDelta{Name: name, Arguments: args}
	return out
}

// failStream reports a streaming failure. Before the first frame nothing is
// committed, so the client can still be given a real HTTP status and an OpenAI
// error envelope; after it, an in-band error frame followed by [DONE] is all
// the protocol allows.
func (p *Proxy) failStream(w http.ResponseWriter, flusher http.Flusher, framesSent, status int, message string, err error) {
	if framesSent == 0 {
		w.Header().Del("Content-Type")
		respondWithOAIError(w, status, message, err, false)
		return
	}
	detail := message
	if err != nil {
		detail = fmt.Sprintf("%s: %s", message, err.Error())
	}
	p.sendStreamError(w, flusher, detail, oaiErrorType(status))
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

	analytics.RecordChatRecord(r.Context(), rec)

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
