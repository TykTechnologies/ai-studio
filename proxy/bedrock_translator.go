package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Bedrock is served by the AWS SDK directly rather than through the loopback
// hop, so each rung of the failover waterfall is run inline here. The shape is
// deliberately "open the call, then write": everything that can fail before a
// byte is written (auth, budget, client, model id, the Converse call itself)
// happens first and is returned as an attemptFailure the loop can act on;
// only a successful call is turned into a response and written.

// bedrockPrepare resolves the app and checks its budget against this rung.
func (p *Proxy) bedrockPrepare(r *http.Request, conf *models.LLM) (*models.App, attemptFailure) {
	// Get app from context (auth already ran via middleware)
	app, err := p.getAppFromContext(r, conf)
	if err != nil {
		return nil, attemptFailure{err: fmt.Errorf("authentication required: %w", err), status: http.StatusUnauthorized, hasStatus: true}
	}

	// Check budget
	if _, _, err := p.budgetService.CheckBudget(app, conf); err != nil {
		return app, attemptFailure{err: fmt.Errorf("budget exceeded: %w", err), status: http.StatusForbidden, hasStatus: true}
	}
	return app, attemptFailure{}
}

// bedrockConverseInput builds the client and the Converse request for model.
func bedrockConverseInput(conf *models.LLM, req *ChatCompletionRequest, model string) (*bedrockruntime.Client, string, *bedrockruntime.ConverseInput, attemptFailure) {
	// Create Bedrock client
	client, err := bedrockVendor.NewBedrockClient(conf)
	if err != nil {
		return nil, "", nil, attemptFailure{err: fmt.Errorf("%w: failed to create Bedrock client: %v", errDriverSetup, err), status: http.StatusInternalServerError, driverError: true}
	}

	// Determine model ID
	modelID := bedrockVendor.GetModelID(conf, model)
	if modelID == "" {
		return client, "", nil, attemptFailure{err: fmt.Errorf("model ID is required"), status: http.StatusBadRequest, hasStatus: true}
	}

	// Convert OpenAI messages to Converse format
	chatMessages := openAIMessagesToChatMessages(req.Messages)
	converseMsgs, systemBlocks := bedrockVendor.ConvertChatMessagesToConverse(chatMessages)

	// Build inference config
	inferenceConfig := bedrockVendor.BuildInferenceConfig(req.MaxCompletionTokens, req.Temperature, req.TopP, req.Stop)

	// Build tool config
	var toolConfig *types.ToolConfiguration
	if len(req.Tools) > 0 {
		toolDefs := openAIToolsToToolDefs(req.Tools)
		toolConfig = bedrockVendor.BuildToolConfig(toolDefs)
	}

	return client, modelID, &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMsgs,
		System:          systemBlocks,
		InferenceConfig: inferenceConfig,
		ToolConfig:      toolConfig,
	}, attemptFailure{}
}

// bedrockChatCompletionAttempt runs one non-streaming Bedrock rung of the
// waterfall for the /ai/v1/chat/completions endpoint, translating OpenAI
// format <-> Converse API format. Nothing is written: on success the caller
// writes the returned body. Every attempt leaves a ProxyLog, a failed one
// with the status Bedrock returned, so a failed primary and the fallback that
// served can be read together.
func (p *Proxy) bedrockChatCompletionAttempt(ctx context.Context, r *http.Request, a llmAttempt, req *ChatCompletionRequest, reqBody []byte) ([]byte, attemptFailure) {
	timestamp := time.Now()
	conf := a.conf

	app, fail := p.bedrockPrepare(r, conf)
	if fail.err != nil {
		return nil, fail
	}

	client, modelID, input, fail := bedrockConverseInput(conf, req, a.model)
	if fail.err != nil {
		return nil, fail
	}

	// Call Converse API
	output, err := client.Converse(ctx, input)
	if err != nil {
		// Bedrock's SDK errors carry the HTTP status the service returned;
		// flattening every one of them to 502 turned "no such model" into a
		// retryable server fault.
		fail := classifyBedrockError(fmt.Errorf("Bedrock Converse failed: %w", err), ctx)
		p.goAnalyze(func() { recordBedrockFailedAttempt(conf, app, modelID, reqBody, fail, r, timestamp) })
		return nil, fail
	}

	// Convert Converse response to OpenAI format. The model echoed is the one
	// that answered.
	response := converseOutputToOpenAI(output, a.model)

	// Marshal response
	respBody, err := json.Marshal(response)
	if err != nil {
		return nil, attemptFailure{err: fmt.Errorf("failed to marshal response: %w", err), status: http.StatusInternalServerError, hasStatus: true}
	}

	// Execute response filters (non-streaming)
	if p.hasResponseFilters(conf) {
		blocked, blockMsg, filterErr := ExecuteResponseFilters(
			conf, p.gatewayService, respBody, http.StatusOK,
			false, false, 0, "", r,
		)
		if filterErr != nil {
			log.Error().Err(filterErr).Msg("Response filter error on Bedrock response")
		} else if blocked {
			go recordBedrockAnalytics(p, conf, app, modelID, output, reqBody, []byte(blockMsg), r, timestamp)
			return nil, attemptFailure{err: fmt.Errorf("Response blocked by filter: %s", blockMsg), status: http.StatusBadRequest, hasStatus: true}
		}
	}

	// Record analytics
	go recordBedrockAnalytics(p, conf, app, modelID, output, reqBody, respBody, r, timestamp)

	return respBody, attemptFailure{}
}

// bedrockStreamAttempt runs one streaming Bedrock rung for the
// /ai/v1/chat/completions endpoint with stream=true. It reports whether the
// stream was opened: once it is, the response is committed to this rung and
// the pump writes everything including [DONE]. A failure to open is returned
// with nothing written, so the loop can move on to the next rung.
func (p *Proxy) bedrockStreamAttempt(
	w http.ResponseWriter,
	flusher http.Flusher,
	r *http.Request,
	a llmAttempt,
	req *ChatCompletionRequest,
	reqBody []byte,
	completionID string,
	created int64,
) (bool, attemptFailure) {
	timestamp := time.Now()
	conf := a.conf

	app, fail := p.bedrockPrepare(r, conf)
	if fail.err != nil {
		return false, fail
	}

	client, modelID, converseInput, fail := bedrockConverseInput(conf, req, a.model)
	if fail.err != nil {
		return false, fail
	}

	// Call ConverseStream
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:         converseInput.ModelId,
		Messages:        converseInput.Messages,
		System:          converseInput.System,
		InferenceConfig: converseInput.InferenceConfig,
		ToolConfig:      converseInput.ToolConfig,
	}

	output, err := client.ConverseStream(r.Context(), input)
	if err != nil {
		// Nothing has been written yet, so the caller can still be given the
		// status Bedrock actually returned instead of a 200 whose only frame
		// says the request failed.
		fail := classifyBedrockError(fmt.Errorf("Bedrock ConverseStream failed: %w", err), nil)
		p.goAnalyze(func() { recordBedrockFailedAttempt(conf, app, modelID, reqBody, fail, r, timestamp) })
		return false, fail
	}

	stream := output.GetStream()
	if stream == nil {
		fail := attemptFailure{err: fmt.Errorf("Bedrock returned no stream"), status: http.StatusBadGateway}
		p.goAnalyze(func() { recordBedrockFailedAttempt(conf, app, modelID, reqBody, fail, r, timestamp) })
		return false, fail
	}
	defer stream.Close()

	p.bedrockPumpStream(w, flusher, r, conf, app, req, reqBody, stream, modelID, a.model, completionID, created, timestamp)
	return true, attemptFailure{}
}

// bedrockPumpStream re-encodes a Converse event stream as OpenAI SSE frames.
// servedModel is echoed on every chunk.
func (p *Proxy) bedrockPumpStream(
	w http.ResponseWriter,
	flusher http.Flusher,
	r *http.Request,
	conf *models.LLM,
	app *models.App,
	req *ChatCompletionRequest,
	reqBody []byte,
	stream *bedrockruntime.ConverseStreamEventStream,
	modelID string,
	servedModel string,
	completionID string,
	created int64,
	timestamp time.Time,
) {
	isFirstChunk := true
	var inputTokens, outputTokens int32
	var cacheWriteTokens, cacheReadTokens int32
	hasResponseFilters := p.hasResponseFilters(conf)
	var textBuffer strings.Builder
	chunkIndex := 0

	// Bedrock numbers every content block in one sequence, text and tool use
	// alike, but OpenAI's tool_calls[].index counts tool calls only - it is what
	// a client uses to reassemble argument fragments, so the two numberings have
	// to be mapped rather than passed through.
	toolCallIndexes := map[int32]int{}
	toolArgsSeen := map[int32]bool{}
	nextToolCallIndex := 0

	sendChunk := func(delta ChatCompletionDelta) {
		if isFirstChunk {
			delta.Role = "assistant"
			isFirstChunk = false
		}
		sendSSEChunk(w, flusher, ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   servedModel,
			Choices: []ChatCompletionChunkChoice{{
				Index: 0,
				Delta: delta,
			}},
		})
	}

	for event := range stream.Events() {
		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Bedrock ConverseStream error")
			break
		}

		switch v := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			// Send first chunk with role
			if isFirstChunk {
				sendChunk(ChatCompletionDelta{})
			}
			_ = v

		case *types.ConverseStreamOutputMemberContentBlockStart:
			// The opening fragment of a tool call: it is the only place Bedrock
			// sends the id and the function name. Dropping this event is why
			// streamed Bedrock tool calls used to vanish entirely - the
			// arguments that follow have nothing to attach to.
			toolUse, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse)
			if !ok {
				continue
			}
			blockIndex := aws.ToInt32(v.Value.ContentBlockIndex)
			toolCallIndexes[blockIndex] = nextToolCallIndex
			nextToolCallIndex++

			sendChunk(ChatCompletionDelta{ToolCalls: []ChatCompletionToolCallDelta{{
				Index: toolCallIndexes[blockIndex],
				ID:    aws.ToString(toolUse.Value.ToolUseId),
				Type:  "function",
				Function: &ChatCompletionFunctionDelta{
					Name: aws.ToString(toolUse.Value.Name),
				},
			}}})

		case *types.ConverseStreamOutputMemberContentBlockStop:
			// A tool call whose input was empty produced no argument fragments,
			// and "" does not parse. Clients concatenate the fragments and
			// json.Unmarshal the result, so close the call with an empty object.
			blockIndex := aws.ToInt32(v.Value.ContentBlockIndex)
			toolIndex, isTool := toolCallIndexes[blockIndex]
			if !isTool || toolArgsSeen[blockIndex] {
				continue
			}
			sendChunk(ChatCompletionDelta{ToolCalls: []ChatCompletionToolCallDelta{{
				Index:    toolIndex,
				Function: &ChatCompletionFunctionDelta{Arguments: "{}"},
			}}})

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			if toolDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
				blockIndex := aws.ToInt32(v.Value.ContentBlockIndex)
				toolIndex, isTool := toolCallIndexes[blockIndex]
				if !isTool {
					continue
				}
				fragment := aws.ToString(toolDelta.Value.Input)
				if fragment == "" {
					continue
				}
				toolArgsSeen[blockIndex] = true
				sendChunk(ChatCompletionDelta{ToolCalls: []ChatCompletionToolCallDelta{{
					Index:    toolIndex,
					Function: &ChatCompletionFunctionDelta{Arguments: fragment},
				}}})
				continue
			}

			if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
				// Accumulate text for logging
				textBuffer.WriteString(textDelta.Value)

				// Execute response filters on text content
				if hasResponseFilters {
					chunkBytes, _ := json.Marshal(textDelta.Value)
					blocked, blockMsg, filterErr := ExecuteResponseFilters(
						conf, p.gatewayService, chunkBytes, http.StatusOK,
						true, true, chunkIndex, textBuffer.String(), r,
					)
					if filterErr != nil {
						log.Error().Err(filterErr).Int("chunk", chunkIndex).Msg("Response filter error on Bedrock stream")
					} else if blocked {
						p.sendStreamError(w, flusher, fmt.Sprintf("Response blocked by filter: %s", blockMsg), "content_filter")
						fmt.Fprintf(w, "data: [DONE]\n\n")
						flusher.Flush()
						return
					}
					chunkIndex++
				}

				sendChunk(ChatCompletionDelta{Content: textDelta.Value})
			}

		case *types.ConverseStreamOutputMemberMessageStop:
			finishReason := bedrockVendor.ConvertConverseStopReason(v.Value.StopReason)
			chunkResp := ChatCompletionChunk{
				ID:      completionID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   servedModel,
				Choices: []ChatCompletionChunkChoice{{
					Index:        0,
					Delta:        ChatCompletionDelta{},
					FinishReason: &finishReason,
				}},
			}
			sendSSEChunk(w, flusher, chunkResp)

		case *types.ConverseStreamOutputMemberMetadata:
			if v.Value.Usage != nil {
				inputTokens = aws.ToInt32(v.Value.Usage.InputTokens)
				outputTokens = aws.ToInt32(v.Value.Usage.OutputTokens)
				cacheWriteTokens = aws.ToInt32(v.Value.Usage.CacheWriteInputTokens)
				cacheReadTokens = aws.ToInt32(v.Value.Usage.CacheReadInputTokens)

				// Send final chunk with usage
				usage := CompletionUsage{
					PromptTokens:     int(inputTokens),
					CompletionTokens: int(outputTokens),
					TotalTokens:      int(inputTokens + outputTokens),
				}
				chunkResp := ChatCompletionChunk{
					ID:      completionID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   servedModel,
					Choices: []ChatCompletionChunkChoice{{
						Index: 0,
						Delta: ChatCompletionDelta{},
					}},
					Usage: &usage,
				}
				sendSSEChunk(w, flusher, chunkResp)
			}
		}
	}

	// Send [DONE] marker
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	// Record proxy log and analytics in a single goroutine to ensure correct ordering:
	// ProxyLog must be recorded first to create the skeleton event that ChatRecord enriches.
	responseText := textBuffer.String()
	p.goAnalyze(func() {
		recordBedrockProxyLog(p, conf, app, modelID, reqBody, responseText, r, timestamp)
		recordBedrockChatRecord(p, conf, app, modelID, int(inputTokens), int(outputTokens), int(cacheWriteTokens), int(cacheReadTokens), r, timestamp)
	})
}

// --- Helper functions ---

func openAIMessagesToChatMessages(messages []Message) []bedrockVendor.ChatMessage {
	result := make([]bedrockVendor.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		content := ""
		switch v := msg.Content.(type) {
		case string:
			content = v
		case []interface{}:
			// Multi-part content: extract text parts
			for _, part := range v {
				if partMap, ok := part.(map[string]interface{}); ok {
					if partMap["type"] == "text" {
						if text, ok := partMap["text"].(string); ok {
							content += text
						}
					}
				}
			}
		}
		result = append(result, bedrockVendor.ChatMessage{
			Role:    msg.Role,
			Content: content,
		})
	}
	return result
}

func openAIToolsToToolDefs(tools []Tool) []bedrockVendor.ToolDef {
	result := make([]bedrockVendor.ToolDef, 0, len(tools))
	for _, tool := range tools {
		result = append(result, bedrockVendor.ToolDef{
			Type: tool.Type,
			Function: bedrockVendor.ToolFuncDef{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}
	return result
}

func converseOutputToOpenAI(output *bedrockruntime.ConverseOutput, model string) *ChatCompletionResponse {
	response := &ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: make([]ChatCompletionChoice, 0),
	}

	// Extract content from output
	if msgOutput, ok := output.Output.(*types.ConverseOutputMemberMessage); ok {
		text := bedrockVendor.ExtractTextFromContentBlocks(msgOutput.Value.Content)
		toolCalls := bedrockVendor.ExtractToolCallsFromContentBlocks(msgOutput.Value.Content)

		choice := ChatCompletionChoice{
			Index: 0,
			Message: ChatCompletionMessage{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: bedrockVendor.ConvertConverseStopReason(output.StopReason),
		}

		if len(toolCalls) > 0 {
			choice.Message.ToolCalls = toolCalls
			choice.Message.Content = ""
		}

		response.Choices = append(response.Choices, choice)
	}

	// Set usage
	if output.Usage != nil {
		response.Usage = CompletionUsage{
			PromptTokens:     int(aws.ToInt32(output.Usage.InputTokens)),
			CompletionTokens: int(aws.ToInt32(output.Usage.OutputTokens)),
			TotalTokens:      int(aws.ToInt32(output.Usage.TotalTokens)),
		}
	}

	return response
}

func sendSSEChunk(w http.ResponseWriter, flusher http.Flusher, chunk ChatCompletionChunk) {
	jsonBytes, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
	flusher.Flush()
}

func recordBedrockAnalytics(p *Proxy, llm *models.LLM, app *models.App, modelID string, output *bedrockruntime.ConverseOutput, reqBody []byte, respBody []byte, r *http.Request, timestamp time.Time) {
	const maxBodySize = 65535

	// Record proxy log
	proxyLog := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		ModelName:    modelID,
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(respBody), maxBodySize),
		ResponseCode: http.StatusOK,
	}
	if llm.DontLogBodies {
		proxyLog.RequestBody = ""
		proxyLog.ResponseBody = ""
	}
	applyFailoverMarker(proxyLog, r.Context())
	ctx := context.WithoutCancel(r.Context())
	analytics.RecordProxyLog(ctx, proxyLog)

	// Record chat analytics
	if output.Usage != nil {
		recordBedrockChatRecord(p, llm, app, modelID,
			int(aws.ToInt32(output.Usage.InputTokens)),
			int(aws.ToInt32(output.Usage.OutputTokens)),
			int(aws.ToInt32(output.Usage.CacheWriteInputTokens)),
			int(aws.ToInt32(output.Usage.CacheReadInputTokens)),
			r, timestamp)
	}
}

// recordBedrockFailedAttempt leaves a ProxyLog for a Bedrock rung that failed
// before anything was written, with the status Bedrock returned, so the row
// sits beside the fallback's row exactly as an OpenAI-shaped rung's would.
func recordBedrockFailedAttempt(llm *models.LLM, app *models.App, modelID string, reqBody []byte, fail attemptFailure, r *http.Request, timestamp time.Time) {
	const maxBodySize = 65535

	detail := ""
	if fail.err != nil {
		detail = fail.err.Error()
	}
	body, _ := json.Marshal(map[string]interface{}{"error": map[string]string{"message": detail}})
	proxyLog := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		ModelName:    modelID,
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: string(body),
		ResponseCode: fail.status,
	}
	if llm.DontLogBodies {
		proxyLog.RequestBody = ""
		proxyLog.ResponseBody = ""
	}
	applyFailoverMarker(proxyLog, r.Context())
	analytics.RecordProxyLog(context.WithoutCancel(r.Context()), proxyLog)
}

// recordBedrockProxyLog records a proxy log entry for Bedrock streaming paths where
// recordBedrockAnalytics (which expects a ConverseOutput) cannot be used.
func recordBedrockProxyLog(p *Proxy, llm *models.LLM, app *models.App, modelID string, reqBody []byte, responseText string, r *http.Request, timestamp time.Time) {
	const maxBodySize = 65535

	proxyLog := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		ModelName:    modelID,
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(responseText, maxBodySize),
		ResponseCode: http.StatusOK,
	}
	if llm.DontLogBodies {
		proxyLog.RequestBody = ""
		proxyLog.ResponseBody = ""
	}
	applyFailoverMarker(proxyLog, r.Context())
	ctx := context.WithoutCancel(r.Context())
	analytics.RecordProxyLog(ctx, proxyLog)
}

// recordBedrockChatRecord is the single place that calculates cost and records an LLMChatRecord
// for all Bedrock paths (non-streaming /ai/, streaming /ai/, and streaming /llm/stream/).
func recordBedrockChatRecord(p *Proxy, llm *models.LLM, app *models.App, modelID string, promptTokens int, responseTokens int, cacheWriteTokens int, cacheReadTokens int, r *http.Request, timestamp time.Time) {
	if promptTokens == 0 && responseTokens == 0 && cacheWriteTokens == 0 && cacheReadTokens == 0 {
		return
	}

	price, err := p.gatewayService.GetModelPriceByModelNameAndVendor(modelID, string(llm.Vendor))
	if err != nil {
		log.Debug().Str("model", modelID).Str("vendor", string(llm.Vendor)).Msg("No pricing found for Bedrock model")
		price = &models.ModelPrice{}
	}

	cost := ((price.CPT * float64(responseTokens)) +
		(price.CPIT * float64(promptTokens)) +
		(price.CacheWritePT * float64(cacheWriteTokens)) +
		(price.CacheReadPT * float64(cacheReadTokens))) * 10000

	record := &models.LLMChatRecord{
		LLMID:                  llm.ID,
		Name:                   modelID,
		Vendor:                 string(llm.Vendor),
		PromptTokens:           promptTokens,
		ResponseTokens:         responseTokens,
		TotalTokens:            promptTokens + responseTokens,
		CacheWritePromptTokens: cacheWriteTokens,
		CacheReadPromptTokens:  cacheReadTokens,
		Cost:                   cost,
		Currency:               price.Currency,
		Choices:                1,
		TimeStamp:              timestamp,
		AppID:                  app.ID,
		UserID:                 app.UserID,
		InteractionType:        models.ProxyInteraction,
	}
	ctx := context.WithoutCancel(r.Context())
	analytics.RecordChatRecord(ctx, record)

	// Trigger budget analysis
	p.budgetService.AnalyzeBudgetUsage(app, llm)
}
