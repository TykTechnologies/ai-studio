package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// handleAnthropicMessagesEntry is the entry point for POST /anthropic/{routeId}/v1/messages.
// It lets Claude Code (which speaks the native Anthropic Messages API) drive Claude models
// hosted in Amazon Bedrock, with AI Studio applying auth, budget, filters, and analytics.
// Auth runs in the credential-validator middleware before this handler (mirroring /ai/).
func (p *Proxy) handleAnthropicMessagesEntry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["routeId"]

	// GetLLM takes the read lock; Reload writes this map concurrently.
	conf, ok := p.GetLLM(routeID)
	if !ok {
		respondWithAnthropicError(w, http.StatusNotFound, "not_found_error", "route not found")
		return
	}

	// The bridge only translates for Bedrock-backed LLMs. Other vendors already speak
	// their native API via the /llm/ pass-through.
	if conf.Vendor != models.BEDROCK {
		respondWithAnthropicError(w, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("the Anthropic Messages bridge only supports Bedrock LLMs, not %q", conf.Vendor))
		return
	}

	reqBody, err := helpers.CopyRequestBody(r)
	if err != nil {
		respondWithAnthropicError(w, http.StatusInternalServerError, "api_error", "failed to read request body")
		return
	}

	var req AnthropicMessagesRequest
	if err := json.Unmarshal(reqBody, &req); err != nil {
		respondWithAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return
	}
	if len(req.Messages) == 0 {
		respondWithAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	if req.Stream {
		p.handleBedrockAnthropicMessagesStream(w, r, conf, &req, reqBody)
	} else {
		p.handleBedrockAnthropicMessages(w, r, conf, &req, reqBody)
	}
}

// resolveAnthropicModelID returns the Bedrock model ID to call. Claude Code sends its own
// model names (e.g. "claude-sonnet-4-..."), which are not Bedrock IDs, so the configured
// default_model is the source of truth. Governance is applied by validating that configured
// model against the LLM's allowed_models.
func (p *Proxy) resolveAnthropicModelID(w http.ResponseWriter, conf *models.LLM) (string, bool) {
	modelID := bedrockVendor.GetModelID(conf, "")
	if modelID == "" {
		respondWithAnthropicError(w, http.StatusBadRequest, "invalid_request_error",
			"this Bedrock LLM has no default_model configured")
		return "", false
	}
	if !NewModelValidator(conf.AllowedModels).IsModelAllowed(modelID) {
		respondWithAnthropicError(w, http.StatusForbidden, "permission_error",
			fmt.Sprintf("model %q is not allowed", modelID))
		return "", false
	}
	return modelID, true
}

// handleBedrockAnthropicMessages handles a non-streaming /v1/messages request.
func (p *Proxy) handleBedrockAnthropicMessages(w http.ResponseWriter, r *http.Request, conf *models.LLM, req *AnthropicMessagesRequest, reqBody []byte) {
	timestamp := time.Now()

	app, err := p.getAppFromContext(r, conf)
	if err != nil {
		respondWithAnthropicError(w, http.StatusUnauthorized, "authentication_error", "authentication required")
		return
	}

	if _, _, err := p.budgetService.CheckBudget(app, conf); err != nil {
		respondWithAnthropicError(w, http.StatusForbidden, "permission_error", fmt.Sprintf("budget exceeded: %s", err.Error()))
		return
	}

	modelID, ok := p.resolveAnthropicModelID(w, conf)
	if !ok {
		return
	}

	// NewBedrockClient returns a cached *bedrockruntime.Client (sync.Map keyed by LLM ID
	// + credential fingerprint) — it is not constructed per request.
	client, err := bedrockVendor.NewBedrockClient(conf)
	if err != nil {
		respondWithAnthropicError(w, http.StatusInternalServerError, "api_error", "failed to create Bedrock client")
		return
	}

	input, err := p.buildConverseInputFromAnthropic(modelID, req)
	if err != nil {
		respondWithAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	output, err := client.Converse(r.Context(), input)
	if err != nil {
		respondWithAnthropicError(w, http.StatusBadGateway, "api_error", fmt.Sprintf("Bedrock Converse failed: %s", err.Error()))
		return
	}

	response := converseOutputToAnthropicResponse(output, echoModel(req.Model, modelID))
	respBody, err := json.Marshal(response)
	if err != nil {
		respondWithAnthropicError(w, http.StatusInternalServerError, "api_error", "failed to marshal response")
		return
	}

	// Response filters (non-streaming)
	if p.hasResponseFilters(conf) {
		blocked, blockMsg, filterErr := ExecuteResponseFilters(
			conf, p.gatewayService, respBody, http.StatusOK, false, false, 0, "", r,
		)
		if filterErr != nil {
			log.Error().Err(filterErr).Msg("Response filter error on Bedrock Anthropic response")
		} else if blocked {
			respondWithAnthropicError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Response blocked by filter: %s", blockMsg))
			go recordBedrockAnalytics(p, conf, app, modelID, output, reqBody, []byte(blockMsg), r, timestamp)
			return
		}
	}

	go recordBedrockAnalytics(p, conf, app, modelID, output, reqBody, respBody, r, timestamp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

// handleBedrockAnthropicMessagesStream handles a streaming /v1/messages request, translating
// the Bedrock Converse event stream into the Anthropic Messages SSE event sequence.
func (p *Proxy) handleBedrockAnthropicMessagesStream(w http.ResponseWriter, r *http.Request, conf *models.LLM, req *AnthropicMessagesRequest, reqBody []byte) {
	timestamp := time.Now()

	app, err := p.getAppFromContext(r, conf)
	if err != nil {
		respondWithAnthropicError(w, http.StatusUnauthorized, "authentication_error", "authentication required")
		return
	}

	if _, _, err := p.budgetService.CheckBudget(app, conf); err != nil {
		respondWithAnthropicError(w, http.StatusForbidden, "permission_error", fmt.Sprintf("budget exceeded: %s", err.Error()))
		return
	}

	modelID, ok := p.resolveAnthropicModelID(w, conf)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondWithAnthropicError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}

	// NewBedrockClient returns a cached *bedrockruntime.Client (sync.Map keyed by LLM ID
	// + credential fingerprint) — it is not constructed per request.
	client, err := bedrockVendor.NewBedrockClient(conf)
	if err != nil {
		writeAnthropicSSEError(w, flusher, "api_error", "failed to create Bedrock client")
		return
	}

	streamInput, err := p.buildConverseInputFromAnthropic(modelID, req)
	if err != nil {
		writeAnthropicSSEError(w, flusher, "invalid_request_error", err.Error())
		return
	}

	output, err := client.ConverseStream(r.Context(), &bedrockruntime.ConverseStreamInput{
		ModelId:         streamInput.ModelId,
		Messages:        streamInput.Messages,
		System:          streamInput.System,
		InferenceConfig: streamInput.InferenceConfig,
		ToolConfig:      streamInput.ToolConfig,
	})
	if err != nil {
		writeAnthropicSSEError(w, flusher, "api_error", fmt.Sprintf("Bedrock ConverseStream failed: %s", err.Error()))
		return
	}

	stream := output.GetStream()
	if stream == nil {
		writeAnthropicSSEError(w, flusher, "api_error", "Bedrock returned no stream")
		return
	}
	defer stream.Close()

	st := &anthropicStreamState{
		w:          w,
		flusher:    flusher,
		msgID:      "msg_" + uuid.New().String(),
		echoModel:  echoModel(req.Model, modelID),
		started:    map[int32]bool{},
		stopped:    map[int32]bool{},
		hasFilters: p.hasResponseFilters(conf),
		proxy:      p,
		conf:       conf,
		request:    r,
	}

	for event := range stream.Events() {
		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Bedrock Anthropic stream error")
			break
		}
		if blocked := st.handleEvent(event); blocked {
			return // filter blocked and already emitted an error event
		}
	}

	st.finish()

	// Record analytics after the stream completes (ProxyLog first so ChatRecord can enrich it).
	responseText := st.textBuffer
	inputTokens, outputTokens := st.inputTokens, st.outputTokens
	cacheWrite, cacheRead := st.cacheWriteTokens, st.cacheReadTokens
	go func() {
		recordBedrockProxyLog(p, conf, app, modelID, reqBody, responseText, r, timestamp)
		recordBedrockChatRecord(p, conf, app, modelID, int(inputTokens), int(outputTokens), int(cacheWrite), int(cacheRead), r, timestamp)
	}()
}

// buildConverseInputFromAnthropic maps an Anthropic Messages request to a Bedrock Converse input.
func (p *Proxy) buildConverseInputFromAnthropic(modelID string, req *AnthropicMessagesRequest) (*bedrockruntime.ConverseInput, error) {
	msgs, system, err := anthropicMessagesToVendor(req)
	if err != nil {
		return nil, err
	}

	converseMsgs, systemBlocks := bedrockVendor.AnthropicMessagesToConverse(msgs, system)

	var maxTokens *int
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		maxTokens = &mt
	}
	var stop any
	if len(req.StopSequences) > 0 {
		stop = req.StopSequences
	}
	inferenceConfig := bedrockVendor.BuildInferenceConfig(maxTokens, req.Temperature, req.TopP, stop)

	toolConfig := bedrockVendor.AnthropicToolsToToolConfig(anthropicToolsToVendor(req.Tools), anthropicToolChoiceToVendor(req.ToolChoice))

	return &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMsgs,
		System:          systemBlocks,
		InferenceConfig: inferenceConfig,
		ToolConfig:      toolConfig,
	}, nil
}

// --- wire -> vendor mapping helpers ---

func anthropicMessagesToVendor(req *AnthropicMessagesRequest) ([]bedrockVendor.AnthropicMsg, []bedrockVendor.AnthropicSystemBlock, error) {
	msgs := make([]bedrockVendor.AnthropicMsg, 0, len(req.Messages))
	for _, m := range req.Messages {
		blocks, err := parseAnthropicBlocks(m.Content)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid message content: %w", err)
		}
		vBlocks := make([]bedrockVendor.AnthropicBlock, 0, len(blocks))
		for _, b := range blocks {
			vb := bedrockVendor.AnthropicBlock{
				Type:       b.Type,
				Text:       b.Text,
				CachePoint: b.CacheControl != nil,
			}
			switch b.Type {
			case "tool_use":
				vb.ToolUseID = b.ID
				vb.Name = b.Name
				if len(b.Input) > 0 {
					_ = json.Unmarshal(b.Input, &vb.Input)
				}
			case "tool_result":
				vb.ToolUseID = b.ToolUseID
				vb.ResultText = anthropicContentToText(b.Content)
				vb.IsError = b.IsError
			}
			vBlocks = append(vBlocks, vb)
		}
		msgs = append(msgs, bedrockVendor.AnthropicMsg{Role: m.Role, Blocks: vBlocks})
	}

	systemBlocks, err := parseAnthropicBlocks(req.System)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid system prompt: %w", err)
	}
	system := make([]bedrockVendor.AnthropicSystemBlock, 0, len(systemBlocks))
	for _, b := range systemBlocks {
		system = append(system, bedrockVendor.AnthropicSystemBlock{
			Text:       b.Text,
			CachePoint: b.CacheControl != nil,
		})
	}

	return msgs, system, nil
}

func anthropicToolsToVendor(tools []AnthropicTool) []bedrockVendor.AnthropicToolSpec {
	if len(tools) == 0 {
		return nil
	}
	out := make([]bedrockVendor.AnthropicToolSpec, 0, len(tools))
	for _, t := range tools {
		out = append(out, bedrockVendor.AnthropicToolSpec{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			CachePoint:  t.CacheControl != nil,
		})
	}
	return out
}

func anthropicToolChoiceToVendor(tc *AnthropicToolChoice) *bedrockVendor.AnthropicToolChoiceSpec {
	if tc == nil {
		return nil
	}
	return &bedrockVendor.AnthropicToolChoiceSpec{Type: tc.Type, Name: tc.Name}
}

// --- response mapping ---

func converseOutputToAnthropicResponse(output *bedrockruntime.ConverseOutput, model string) *AnthropicMessagesResponse {
	resp := &AnthropicMessagesResponse{
		ID:      "msg_" + uuid.New().String(),
		Type:    "message",
		Role:    "assistant",
		Model:   model,
		Content: []AnthropicResponseBlock{},
	}

	for _, b := range bedrockVendor.ConverseOutputToAnthropicBlocks(output) {
		block := AnthropicResponseBlock{Type: b.Type, Text: b.Text, ID: b.ID, Name: b.Name}
		if b.Type == "tool_use" {
			if raw, err := json.Marshal(b.Input); err == nil {
				block.Input = raw
			}
		}
		resp.Content = append(resp.Content, block)
	}

	if output != nil {
		resp.StopReason = bedrockVendor.ConvertConverseStopReasonAnthropic(output.StopReason)
		if output.Usage != nil {
			resp.Usage = AnthropicUsage{
				InputTokens:              int(aws.ToInt32(output.Usage.InputTokens)),
				OutputTokens:             int(aws.ToInt32(output.Usage.OutputTokens)),
				CacheCreationInputTokens: int(aws.ToInt32(output.Usage.CacheWriteInputTokens)),
				CacheReadInputTokens:     int(aws.ToInt32(output.Usage.CacheReadInputTokens)),
			}
		}
	}

	return resp
}

// echoModel returns the model name to echo back to the client: the client's requested model
// when present (Claude Code expects its own name), otherwise the Bedrock model ID.
func echoModel(requested, fallback string) string {
	if requested != "" {
		return requested
	}
	return fallback
}

// --- SSE helpers ---

func writeAnthropicSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	flusher.Flush()
}

func writeAnthropicSSEError(w http.ResponseWriter, flusher http.Flusher, errType, message string) {
	writeAnthropicSSE(w, flusher, "error", map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

func respondWithAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

// --- streaming state machine ---

// anthropicStreamState translates a Bedrock Converse event stream into Anthropic SSE events.
type anthropicStreamState struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	msgID     string
	echoModel string

	sentStart        bool
	started          map[int32]bool // content_block_start emitted for index
	stopped          map[int32]bool // content_block_stop emitted for index
	stopReason       string
	inputTokens      int32
	outputTokens     int32
	cacheWriteTokens int32
	cacheReadTokens  int32
	textBuffer       string

	// filters
	hasFilters bool
	chunkIndex int
	proxy      *Proxy
	conf       *models.LLM
	request    *http.Request
}

func (s *anthropicStreamState) ensureMessageStart() {
	if s.sentStart {
		return
	}
	s.sentStart = true
	writeAnthropicSSE(s.w, s.flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         s.echoModel,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})
	writeAnthropicSSE(s.w, s.flusher, "ping", map[string]any{"type": "ping"})
}

func (s *anthropicStreamState) ensureBlockStart(idx int32, isTool bool, toolID, toolName string) {
	if s.started[idx] {
		return
	}
	s.started[idx] = true
	var cb map[string]any
	if isTool {
		cb = map[string]any{"type": "tool_use", "id": toolID, "name": toolName, "input": map[string]any{}}
	} else {
		cb = map[string]any{"type": "text", "text": ""}
	}
	writeAnthropicSSE(s.w, s.flusher, "content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         idx,
		"content_block": cb,
	})
}

// handleEvent processes a single Converse stream event. It returns true if a response filter
// blocked the stream (an error event was already emitted and the stream should end).
func (s *anthropicStreamState) handleEvent(event types.ConverseStreamOutput) bool {
	switch v := event.(type) {
	case *types.ConverseStreamOutputMemberMessageStart:
		s.ensureMessageStart()

	case *types.ConverseStreamOutputMemberContentBlockStart:
		s.ensureMessageStart()
		idx := aws.ToInt32(v.Value.ContentBlockIndex)
		if tu, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
			s.ensureBlockStart(idx, true, aws.ToString(tu.Value.ToolUseId), aws.ToString(tu.Value.Name))
		} else {
			s.ensureBlockStart(idx, false, "", "")
		}

	case *types.ConverseStreamOutputMemberContentBlockDelta:
		s.ensureMessageStart()
		idx := aws.ToInt32(v.Value.ContentBlockIndex)
		switch d := v.Value.Delta.(type) {
		case *types.ContentBlockDeltaMemberText:
			s.ensureBlockStart(idx, false, "", "")
			s.textBuffer += d.Value
			if s.hasFilters && s.runTextFilter(d.Value) {
				return true
			}
			writeAnthropicSSE(s.w, s.flusher, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{"type": "text_delta", "text": d.Value},
			})
		case *types.ContentBlockDeltaMemberToolUse:
			s.ensureBlockStart(idx, true, "", "")
			writeAnthropicSSE(s.w, s.flusher, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": aws.ToString(d.Value.Input)},
			})
		}

	case *types.ConverseStreamOutputMemberContentBlockStop:
		idx := aws.ToInt32(v.Value.ContentBlockIndex)
		s.emitBlockStop(idx)

	case *types.ConverseStreamOutputMemberMessageStop:
		s.stopReason = bedrockVendor.ConvertConverseStopReasonAnthropic(v.Value.StopReason)

	case *types.ConverseStreamOutputMemberMetadata:
		if v.Value.Usage != nil {
			s.inputTokens = aws.ToInt32(v.Value.Usage.InputTokens)
			s.outputTokens = aws.ToInt32(v.Value.Usage.OutputTokens)
			s.cacheWriteTokens = aws.ToInt32(v.Value.Usage.CacheWriteInputTokens)
			s.cacheReadTokens = aws.ToInt32(v.Value.Usage.CacheReadInputTokens)
		}
	}
	return false
}

func (s *anthropicStreamState) runTextFilter(text string) bool {
	chunkBytes, _ := json.Marshal(text)
	blocked, blockMsg, filterErr := ExecuteResponseFilters(
		s.conf, s.proxy.gatewayService, chunkBytes, http.StatusOK,
		true, true, s.chunkIndex, s.textBuffer, s.request,
	)
	s.chunkIndex++
	if filterErr != nil {
		log.Error().Err(filterErr).Int("chunk", s.chunkIndex).Msg("Response filter error on Bedrock Anthropic stream")
		return false
	}
	if blocked {
		writeAnthropicSSEError(s.w, s.flusher, "invalid_request_error", fmt.Sprintf("Response blocked by filter: %s", blockMsg))
		return true
	}
	return false
}

func (s *anthropicStreamState) emitBlockStop(idx int32) {
	if !s.started[idx] || s.stopped[idx] {
		return
	}
	s.stopped[idx] = true
	writeAnthropicSSE(s.w, s.flusher, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": idx,
	})
}

// finish emits any missing content_block_stop events, then message_delta and message_stop.
func (s *anthropicStreamState) finish() {
	s.ensureMessageStart()
	// Close any content blocks Bedrock started but never stopped.
	for idx := range s.started {
		s.emitBlockStop(idx)
	}
	if s.stopReason == "" {
		s.stopReason = "end_turn"
	}
	writeAnthropicSSE(s.w, s.flusher, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   s.stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":                s.inputTokens,
			"output_tokens":               s.outputTokens,
			"cache_creation_input_tokens": s.cacheWriteTokens,
			"cache_read_input_tokens":     s.cacheReadTokens,
		},
	})
	writeAnthropicSSE(s.w, s.flusher, "message_stop", map[string]any{"type": "message_stop"})
}
