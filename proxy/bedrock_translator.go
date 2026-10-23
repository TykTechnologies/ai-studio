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
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// A Bedrock rung of the failover waterfall is a translator, as every other
// vendor's is: the AWS SDK client it calls is pointed at the proxy's own
// /llm/call/{slug} (newBedrockLoopbackClient), and the inner hop does the
// rest (the authentication hand-off, plugins, budget, filters, signing,
// response hooks, analytics) against the native Converse body. What fails
// before a byte is written (the request, the inner hop's refusal, the
// Converse call) is returned as an attemptFailure the loop can act on; only
// a successful call is turned into a response and written.

// bedrockConverseInput builds the Converse request for model.
func bedrockConverseInput(conf *models.LLM, req *ChatCompletionRequest, model string) (*bedrockruntime.ConverseInput, attemptFailure) {
	// Determine model ID
	modelID := bedrockVendor.GetModelID(conf, model)
	if modelID == "" {
		return nil, attemptFailure{err: fmt.Errorf("model ID is required"), status: http.StatusBadRequest, hasStatus: true}
	}

	// Convert OpenAI messages to Converse format
	chatMessages := openAIMessagesToChatMessages(req.Messages)
	converseMsgs, systemBlocks := bedrockVendor.ConvertChatMessagesToConverse(chatMessages)

	// Build inference config
	inferenceConfig := bedrockVendor.BuildInferenceConfig(req.OutputTokenLimit(), req.Temperature, req.TopP, req.Stop)

	// Build tool config
	var toolConfig *types.ToolConfiguration
	if len(req.Tools) > 0 {
		toolDefs := openAIToolsToToolDefs(req.Tools)
		toolConfig = bedrockVendor.BuildToolConfig(toolDefs)
	}

	return &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMsgs,
		System:          systemBlocks,
		InferenceConfig: inferenceConfig,
		ToolConfig:      toolConfig,
	}, attemptFailure{}
}

// bedrockChatCompletionAttempt runs one non-streaming Bedrock rung of the
// waterfall for the /ai/v1/chat/completions endpoint, translating OpenAI
// format <-> Converse API format over the loopback. Nothing is written: on
// success the caller writes the returned body, with the inner hop's plugin
// headers from relay.
func (p *Proxy) bedrockChatCompletionAttempt(ctx context.Context, r *http.Request, a llmAttempt, req *ChatCompletionRequest) (body []byte, relay *loopbackRelay, failure attemptFailure) {
	input, fail := bedrockConverseInput(a.conf, req, a.model)
	if fail.err != nil {
		return nil, nil, fail
	}

	relay = &loopbackRelay{}
	output, err := p.newBedrockLoopbackClient(r, a).Converse(bedrockLoopbackContext(ctx, r, relay), input)
	if err != nil {
		// The inner hop's refusal (a policy or budget block, the vendor's
		// own error) when the relay could read it, else the status the SDK
		// read: "no such model" is not a retryable server fault.
		return nil, relay, classifyBedrockError(fmt.Errorf("Bedrock Converse failed: %w", err), ctx).withLoopback(relay)
	}

	// Convert Converse response to OpenAI format. The model echoed is the one
	// that answered.
	respBody, err := json.Marshal(converseOutputToOpenAI(output, a.model))
	if err != nil {
		return nil, relay, attemptFailure{err: fmt.Errorf("failed to marshal response: %w", err), status: http.StatusInternalServerError, hasStatus: true}
	}
	return respBody, relay, attemptFailure{}
}

// bedrockStreamAttempt runs one streaming Bedrock rung for the
// /ai/v1/chat/completions endpoint with stream=true. It reports whether the
// stream was opened: once it is, the response is committed to this rung and
// the pump writes everything including [DONE]. A failure to open (the inner
// hop's refusal included) is returned with nothing written, so the loop can
// move on to the next rung. answer is the reply as the pump assembled it,
// for the agent tracing capture (nil with tracing off).
func (p *Proxy) bedrockStreamAttempt(
	w http.ResponseWriter,
	flusher http.Flusher,
	r *http.Request,
	a llmAttempt,
	req *ChatCompletionRequest,
	completionID string,
	created int64,
) (opened bool, answer []byte, failure attemptFailure) {
	converseInput, fail := bedrockConverseInput(a.conf, req, a.model)
	if fail.err != nil {
		return false, nil, fail
	}

	// Call ConverseStream
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:         converseInput.ModelId,
		Messages:        converseInput.Messages,
		System:          converseInput.System,
		InferenceConfig: converseInput.InferenceConfig,
		ToolConfig:      converseInput.ToolConfig,
	}

	relay := &loopbackRelay{}
	output, err := p.newBedrockLoopbackClient(r, a).ConverseStream(bedrockLoopbackContext(r.Context(), r, relay), input)
	if err != nil {
		// Nothing has been written yet, so the caller can still be given the
		// status the inner hop or Bedrock returned instead of a 200 whose
		// only frame says the request failed.
		return false, nil, classifyBedrockError(fmt.Errorf("Bedrock ConverseStream failed: %w", err), nil).withLoopback(relay)
	}

	stream := output.GetStream()
	if stream == nil {
		return false, nil, attemptFailure{err: fmt.Errorf("Bedrock returned no stream"), status: http.StatusBadGateway}
	}
	defer stream.Close()

	// The inner hop's plugin headers go out with the first frame.
	relay.applyTo(w.Header())
	return true, p.bedrockPumpStream(w, flusher, r, relay, stream, a.model, completionID, created), attemptFailure{}
}

// bedrockPumpStream re-encodes a Converse event stream as OpenAI SSE frames.
// servedModel is echoed on every chunk. The inner hop screened the stream; a
// block it ended the stream with (its trailer, read through relay) ends this
// one as on every other vendor's bridge. It returns the reply it relayed in
// the non-streamed shape while agent tracing is on, else nil.
func (p *Proxy) bedrockPumpStream(
	w http.ResponseWriter,
	flusher http.Flusher,
	r *http.Request,
	relay *loopbackRelay,
	stream *bedrockruntime.ConverseStreamEventStream,
	servedModel string,
	completionID string,
	created int64,
) []byte {
	isFirstChunk := true
	var textBuffer strings.Builder
	var finishReason string
	var usage *CompletionUsage

	// Bedrock numbers every content block in one sequence, text and tool use
	// alike, but OpenAI's tool_calls[].index counts tool calls only - it is what
	// a client uses to reassemble argument fragments, so the two numberings have
	// to be mapped rather than passed through.
	toolCallIndexes := map[int32]int{}
	toolArgsSeen := map[int32]bool{}
	nextToolCallIndex := 0
	// The tool calls as relayed, for the tracing capture.
	var toolCalls []map[string]interface{}

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
	addArgs := func(index int, fragment string) {
		if index >= len(toolCalls) {
			return
		}
		fn := toolCalls[index]["function"].(map[string]interface{})
		fn["arguments"] = fn["arguments"].(string) + fragment
	}

	// Every event received is sent on: the SDK closes Events when the
	// stream fails, and its reader may already have failed on the next
	// message when this one arrives, so an error is checked after the loop,
	// never per event (an event checked that way was dropped).
	for event := range stream.Events() {
		switch v := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			// Send first chunk with role
			if isFirstChunk {
				sendChunk(ChatCompletionDelta{})
			}

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
			toolCalls = append(toolCalls, map[string]interface{}{
				"id": aws.ToString(toolUse.Value.ToolUseId), "type": "function",
				"function": map[string]interface{}{"name": aws.ToString(toolUse.Value.Name), "arguments": ""},
			})

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
			addArgs(toolIndex, "{}")
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
				addArgs(toolIndex, fragment)
				sendChunk(ChatCompletionDelta{ToolCalls: []ChatCompletionToolCallDelta{{
					Index:    toolIndex,
					Function: &ChatCompletionFunctionDelta{Arguments: fragment},
				}}})
				continue
			}

			if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
				textBuffer.WriteString(textDelta.Value)
				sendChunk(ChatCompletionDelta{Content: textDelta.Value})
			}

		case *types.ConverseStreamOutputMemberMessageStop:
			finishReason = bedrockVendor.ConvertConverseStopReason(v.Value.StopReason)
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
				inputTokens := int(aws.ToInt32(v.Value.Usage.InputTokens))
				outputTokens := int(aws.ToInt32(v.Value.Usage.OutputTokens))

				// Send final chunk with usage
				usage = &CompletionUsage{
					PromptTokens:     inputTokens,
					CompletionTokens: outputTokens,
					TotalTokens:      inputTokens + outputTokens,
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
					Usage: usage,
				}
				sendSSEChunk(w, flusher, chunkResp)
			}
		}
	}
	streamErr := stream.Err()
	// The inner hop's trailer says whether its filters blocked the stream;
	// it arrives once the stream is read to its end, which Close does.
	_ = stream.Close()
	if msg := relay.streamBlocked(); msg != "" {
		framesSent := 0
		if !isFirstChunk {
			framesSent = 1
		}
		p.failStreamBlocked(w, flusher, framesSent, msg)
		return nil
	}
	if streamErr != nil {
		// The stream broke: end it with an OpenAI error object and [DONE],
		// which SDKs raise, rather than a bare [DONE] that reads as a
		// complete answer.
		log.Error().Err(streamErr).Msg("Bedrock ConverseStream error")
		b, _ := json.Marshal(ChatCompletionStreamError{Error: ChatCompletionErrorDetail{
			Message: "the Bedrock stream failed before it completed", Type: "upstream_error"}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
		flusher.Flush()
		return nil
	}

	// Send [DONE] marker
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	if agenttrace.From(r.Context()) == nil {
		return nil
	}
	// The stream was relayed as it came; the capture reads the answer in
	// the non-streamed shape.
	if finishReason == "" {
		finishReason = "stop"
	}
	msg := ChatCompletionMessage{Role: "assistant", Content: textBuffer.String(), ToolCalls: toolCalls}
	if len(toolCalls) > 0 {
		msg.Content = ""
	}
	answer := &ChatCompletionResponse{
		ID: completionID, Object: "chat.completion", Created: created, Model: servedModel,
		Choices: []ChatCompletionChoice{{Index: 0, Message: msg, FinishReason: finishReason}},
	}
	if usage != nil {
		answer.Usage = *usage
	}
	b, _ := json.Marshal(answer)
	return b
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
		cm := bedrockVendor.ChatMessage{
			Role:       msg.Role,
			Content:    content,
			ToolCallID: msg.ToolCallID,
		}
		for _, c := range msg.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, bedrockVendor.ChatToolCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
		}
		result = append(result, cm)
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

// recordBedrockFailedAttempt leaves a ProxyLog for a native Bedrock stream
// that failed before anything was relayed, with the status and body Bedrock
// returned (or the transport's failure), so a failed rung's row sits beside
// the fallback's row exactly as an OpenAI-shaped rung's would.
func recordBedrockFailedAttempt(llm *models.LLM, app *models.App, modelID string, reqBody []byte, status int, respBody []byte, r *http.Request, timestamp time.Time) {
	const maxBodySize = 65535

	proxyLog := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		LLMID:        llm.ID,
		Vendor:       string(llm.Vendor),
		ModelName:    modelID,
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(respBody), maxBodySize),
		ResponseCode: status,
	}
	if llm.DontLogBodies {
		proxyLog.RequestBody = ""
		proxyLog.ResponseBody = ""
	}
	applyFailoverMarker(proxyLog, r.Context())
	analytics.RecordProxyLog(context.WithoutCancel(r.Context()), proxyLog)
}

// recordBedrockStreamAnalytics records the proxy log and chat record for
// Bedrock streaming paths where recordBedrockAnalytics (which expects a
// ConverseOutput) cannot be used.
func recordBedrockStreamAnalytics(p *Proxy, llm *models.LLM, app *models.App, modelID string, reqBody []byte, responseText string, promptTokens, responseTokens, cacheWriteTokens, cacheReadTokens int, r *http.Request, timestamp time.Time, bt *bedrockTrace) {
	proxyLog := bedrockProxyLog(llm, app, modelID, reqBody, responseText, r, timestamp)
	record := bedrockChatRecord(p, llm, app, modelID, promptTokens, responseTokens, cacheWriteTokens, cacheReadTokens, timestamp)
	recordBedrockExchange(p, llm, app, proxyLog, record, r)
	if bt != nil {
		bt.setResponse([]byte(responseText))
	}
	captureBedrock(r, llm, app, modelID, reqBody, proxyLog, record, bt)
}

// recordBedrockExchange records a Bedrock request's proxy log with its chat
// record (nil when there was no usage), then triggers the budget analysis.
func recordBedrockExchange(p *Proxy, llm *models.LLM, app *models.App, proxyLog *models.ProxyLog, record *models.LLMChatRecord, r *http.Request) {
	ctx := context.WithoutCancel(r.Context())
	analytics.RecordExchange(ctx, proxyLog, record)
	if record != nil {
		// Count the spend and trigger budget analysis
		analyzeBudget(p.budgetService, llm, app, record)
	}
}

// bedrockProxyLog builds the proxy log entry for a Bedrock streaming path.
func bedrockProxyLog(llm *models.LLM, app *models.App, modelID string, reqBody []byte, responseText string, r *http.Request, timestamp time.Time) *models.ProxyLog {
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
	return proxyLog
}

// bedrockChatRecord builds the LLMChatRecord for all Bedrock paths
// (non-streaming /ai/, streaming /ai/, and streaming /llm/stream/) the way
// every other vendor's is built (buildChatRecordFor): the same price lookup,
// cost (models.ChatRecordCost and its cache-price rules), total tokens and
// default currency. Bedrock reports cache tokens apart from the input count.
// It returns nil when the response reported no usage.
func bedrockChatRecord(p *Proxy, llm *models.LLM, app *models.App, modelID string, promptTokens int, responseTokens int, cacheWriteTokens int, cacheReadTokens int, timestamp time.Time) *models.LLMChatRecord {
	if promptTokens == 0 && responseTokens == 0 && cacheWriteTokens == 0 && cacheReadTokens == 0 {
		return nil
	}
	usage := &bedrockUsage{model: modelID, prompt: promptTokens, response: responseTokens,
		cacheWrite: cacheWriteTokens, cacheRead: cacheReadTokens}
	return buildChatRecordFor(p.gatewayService, llm, app, usage, modelID, timestamp)
}

// bedrockUsage is a Bedrock response's usage as a models.ITokenResponse.
type bedrockUsage struct {
	model                                   string
	prompt, response, cacheWrite, cacheRead int
}

func (u *bedrockUsage) GetPromptTokens() int           { return u.prompt }
func (u *bedrockUsage) GetResponseTokens() int         { return u.response }
func (u *bedrockUsage) GetChoiceCount() int            { return 1 }
func (u *bedrockUsage) GetToolCount() int              { return 0 }
func (u *bedrockUsage) GetModel() string               { return u.model }
func (u *bedrockUsage) GetCacheWritePromptTokens() int { return u.cacheWrite }
func (u *bedrockUsage) GetCacheReadPromptTokens() int  { return u.cacheRead }
