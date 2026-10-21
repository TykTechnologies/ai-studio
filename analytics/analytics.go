package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authidentity"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

var (
	// Legacy variables kept for backwards compatibility
	recStarted bool
	recMutex   sync.RWMutex
)

// stampIdentity records on a proxy log and chat record who the request was
// for and the agent that made it, when an auth plugin said
// (authidentity.Identity, put on the context by the credential validator).
// Values a writer already set are kept.
func stampIdentity(ctx context.Context, log *models.ProxyLog, rec *models.LLMChatRecord) {
	id := authidentity.From(ctx)
	if id == nil {
		return
	}
	onBehalfOf, actingAgent := id.OnBehalfOf(), id.ActingAgent()
	if onBehalfOf == "" && actingAgent == "" {
		return
	}
	if log != nil {
		if log.OnBehalfOf == "" {
			log.OnBehalfOf = onBehalfOf
		}
		if log.ActingAgent == "" {
			log.ActingAgent = actingAgent
		}
	}
	if rec != nil {
		if rec.OnBehalfOf == "" {
			rec.OnBehalfOf = onBehalfOf
		}
		if rec.ActingAgent == "" {
			rec.ActingAgent = actingAgent
		}
	}
}

// stampRequestIDs records on a proxy log the gateway's request id and the
// caller's (pkg/requestid), from the request context. Values a writer
// already set are kept (the hub sets an edge's from the pulse).
func stampRequestIDs(ctx context.Context, log *models.ProxyLog) {
	if log == nil {
		return
	}
	if log.RequestID == "" {
		log.RequestID = requestid.FromContext(ctx)
	}
	if log.ClientRequestID == "" {
		log.ClientRequestID = requestid.ClientFromContext(ctx)
	}
}

func RecordProxyLog(ctx context.Context, log *models.ProxyLog) {
	stampIdentity(ctx, log, nil)
	stampRequestIDs(ctx, log)

	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordProxyLog(ctx, log)
	}

	recordProxyLogMetrics(ctx, log)
}

// RecordExchange records a proxied request's proxy log together with its chat
// record (nil when the response yielded no usage). A handler that implements
// ExchangeRecorder gets both in one call; any other handler gets
// RecordProxyLog followed by RecordChatRecord, as before.
func RecordExchange(ctx context.Context, log *models.ProxyLog, rec *models.LLMChatRecord) {
	stampIdentity(ctx, log, rec)
	stampRequestIDs(ctx, log)

	// The proxy paths build chat records without a latency; the gateway put
	// the request's start on the context (WithRequestStart).
	if rec != nil && rec.TotalTimeMS == 0 {
		if ms, ok := RequestLatencyMS(ctx, rec.TimeStamp); ok {
			rec.TotalTimeMS = ms
		}
	}

	// Metrics read the records before they are handed off: a handler writes
	// them asynchronously, and GORM sets columns with a default (request_kind)
	// on the struct while it inserts, so reading after the hand-off races.
	recordProxyLogMetrics(ctx, log)
	if rec != nil {
		recordChatRecordMetrics(ctx, rec)
	}

	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if er, ok := globalHandler.(ExchangeRecorder); ok {
		er.RecordExchange(ctx, log, rec)
	} else if globalHandler != nil {
		globalHandler.RecordProxyLog(ctx, log)
		if rec != nil {
			globalHandler.RecordChatRecord(ctx, rec)
		}
	}
}

func recordProxyLogMetrics(ctx context.Context, log *models.ProxyLog) {
	metrics.RecordRequest(ctx,
		fmt.Sprintf("%d", log.AppID),
		log.Vendor,
		log.ModelName,
		log.ResponseCode,
	)
}

func RecordToolCall(ctx context.Context, name string, t time.Time, execTime int, toolID uint) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordToolCall(ctx, name, t, execTime, toolID)
	}

	metrics.RecordToolCall(ctx, name, fmt.Sprintf("%d", toolID))
	metrics.ObserveToolDuration(ctx, name, float64(execTime)/1000.0)
}

// RecordMCPCalls records MCP calls a Tyk Gateway proxied, as ingested from
// Tyk Pump. It returns ErrRecorderBusy when the handler could not take the
// batch, so the sender retries it. With no handler, or one that does not
// record MCP calls (MCPCallRecorder), there is nothing to record into and a
// retry would never succeed: the batch is dropped (logged once).
func RecordMCPCalls(ctx context.Context, records []models.MCPCallRecord) error {
	if len(records) == 0 {
		return nil
	}

	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if r, ok := globalHandler.(MCPCallRecorder); ok {
		return r.RecordMCPCalls(ctx, records)
	}
	noMCPRecorderOnce.Do(func() {
		logger.Warnf("The analytics handler does not record MCP calls (%T); Tyk MCP analytics are dropped", globalHandler)
	})
	return nil
}

var noMCPRecorderOnce sync.Once

func RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int, userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
	dontLogBodies bool,
) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	// Check if analytics handler is available
	if globalHandler == nil {
		return
	}

	rec := &models.LLMChatRecord{}

	promptTokens := 0
	responseTokens := 0
	toolCalls := 0

	// Usage belongs to the response, not to a choice: the langchaingo providers
	// copy the response's full usage onto every choice (Anthropic returns one
	// choice per content block, OpenAI and Google one per candidate). Read it
	// once, from the first choice that carries it, as the translator does
	// (proxy extractTokenUsageFromContentResponse).
	var usageInfo map[string]any
	responseParts := []string{}
	for _, c := range cr.Choices {
		toolCalls += len(c.ToolCalls)
		if usageInfo == nil {
			if _, pt, rt := switches.GetTokenCounts(c, vendor); pt+rt > 0 {
				promptTokens, responseTokens = pt, rt
				usageInfo = c.GenerationInfo
			}
		}

		if c.Content != "" {
			responseParts = append(responseParts, c.Content)
		}
	}

	promptParts := []string{}
	for _, p := range mc.Parts {
		tp, ok := p.(llms.TextContent)
		if !ok {
			continue
		}

		if tp.Text != "" {
			promptParts = append(promptParts, tp.Text)
		}
	}

	prompt := strings.Join(promptParts, "\n")

	price, err := svc.GetModelPriceByModelNameAndVendor(name, string(vendor))
	if err != nil || price == nil {
		price = &models.ModelPrice{
			CPT:       0.0,
			CPIT:      0.0,
			ModelName: name,
			Vendor:    string(vendor),
			Currency:  "USD",
		}
	}

	// Get cache token information from the response if available. Value types
	// differ by provider (Google's are int32), and OpenAI reports its cached
	// prompt tokens as PromptCachedTokens.
	if usageInfo == nil && len(cr.Choices) > 0 {
		usageInfo = cr.Choices[0].GenerationInfo
	}
	if usageInfo != nil {
		info := usageInfo
		logger.Log.Debug().Interface("keys", info).Msg("GenerationInfo keys")

		rec.CacheWritePromptTokens = generationInfoInt(info, "CacheCreationInputTokens")
		rec.CacheReadPromptTokens = generationInfoInt(info, "CacheReadInputTokens")
		if rec.CacheReadPromptTokens == 0 && vendor == models.OPENAI {
			rec.CacheReadPromptTokens = generationInfoInt(info, "PromptCachedTokens")
		}
	}

	// Same accounting as the proxy path (proxy.buildChatRecordFor): carve the
	// cache tokens out of the prompt count where the vendor includes them.
	promptTokens = models.PromptTokensExcludingCache(string(vendor), promptTokens, rec.CacheWritePromptTokens, rec.CacheReadPromptTokens)

	rec.Choices = len(cr.Choices)
	// Use provided model name if available, fallback to price model name
	if name != "" {
		rec.Name = name
	} else {
		rec.Name = price.ModelName
	}
	rec.Vendor = string(vendor)
	rec.TotalTimeMS = timeMs
	rec.PromptTokens = promptTokens
	rec.ResponseTokens = responseTokens
	rec.TotalTokens = promptTokens + responseTokens + rec.CacheWritePromptTokens + rec.CacheReadPromptTokens
	rec.TimeStamp = t
	rec.UserID = userID
	rec.ToolCalls = toolCalls
	rec.ChatID = chatID
	rec.AppID = appID
	// An unset (zero) cache price bills those tokens at the input price.
	rec.Cost = models.ChatRecordCost(string(vendor), price.CPT, price.CPIT, price.CacheWritePT, price.CacheReadPT,
		promptTokens, responseTokens, rec.CacheWritePromptTokens, rec.CacheReadPromptTokens)
	rec.Currency = price.Currency
	rec.InteractionType = models.ChatInteraction
	rec.LLMID = llmID

	chatLog := &models.LLMChatLogEntry{}
	chatLog.Name = name
	chatLog.Vendor = string(vendor)
	chatLog.TimeStamp = t
	chatLog.Tokens = promptTokens
	chatLog.UserID = userID
	chatLog.ChatID = chatID

	if !dontLogBodies {
		chatLog.Prompt = prompt
		chatLog.Response = strings.Join(responseParts, "\n")
	}

	// No HTTP request context available in chat session path
	RecordChatRecord(context.Background(), rec)
	RecordChatLogEntry(context.Background(), chatLog)
}

// generationInfoInt reads a token count from a langchaingo GenerationInfo map,
// whatever integer or float type the provider stored it as; 0 when absent.
func generationInfoInt(info map[string]any, key string) int {
	switch v := info[key].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func RecordChatRecord(ctx context.Context, record *models.LLMChatRecord) {
	stampIdentity(ctx, nil, record)

	// Before the hand-off; see RecordExchange.
	recordChatRecordMetrics(ctx, record)

	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordChatRecord(ctx, record)
	}
}

func recordChatRecordMetrics(ctx context.Context, record *models.LLMChatRecord) {
	appID := fmt.Sprintf("%d", record.AppID)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "prompt", record.PromptTokens)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "completion", record.ResponseTokens)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "cache_read", record.CacheReadPromptTokens)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "cache_write", record.CacheWritePromptTokens)
	metrics.RecordCost(ctx, record.Vendor, record.Name, appID, record.Cost)
	// Only a call the model answered or refused adds series: a failed call
	// (possibly naming a model the caller invented) counts nothing, so the
	// model label comes from the vendor's own response.
	if record.RequestKind == models.DecisionRequest && record.Choices+record.DecisionRefusals > 0 {
		metrics.RecordDecisionQuestions(ctx, record.Vendor, record.Name, record.Choices, record.DecisionRefusals)
	}
}

func RecordChatLogEntry(ctx context.Context, log *models.LLMChatLogEntry) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordChatLogEntry(ctx, log)
	}
}

// RecordComplianceEvents records filter script compliance events asynchronously
func RecordComplianceEvents(ctx context.Context, events []*models.ComplianceEvent) {
	if len(events) == 0 {
		return
	}
	// The portal shows an App's owners the events marked for them: on the
	// hub this covers events recorded here and those an edge sent.
	for _, e := range events {
		if e != nil && !e.OwnerVisible && strings.Contains(e.Metadata, `"owner_visible":true`) {
			var m map[string]any
			if json.Unmarshal([]byte(e.Metadata), &m) == nil {
				e.OwnerVisible, _ = m["owner_visible"].(bool)
			}
		}
	}

	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordComplianceEvents(ctx, events)
	}

	for _, event := range events {
		metrics.RecordComplianceEvent(ctx, event.EventType, event.Severity, event.FilterName)
	}
}

// RecordChatRecordsBatch records multiple chat records in a batch for improved performance
func RecordChatRecordsBatch(ctx context.Context, records []*models.LLMChatRecord) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordChatRecordsBatch(ctx, records)
	}
}

// RecordProxyLogsBatch records multiple proxy logs in a batch for improved performance
func RecordProxyLogsBatch(ctx context.Context, logs []*models.ProxyLog) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordProxyLogsBatch(ctx, logs)
	}
}

// InitDefault initializes the default database analytics handler
func InitDefault(ctx context.Context, db *gorm.DB) {
	handlerMu.Lock()
	defer handlerMu.Unlock()

	if globalHandler == nil {
		globalHandler = NewDatabaseHandler(ctx, db)
	}
}

// Init initializes analytics with default database handler (for backward compatibility)
func Init(ctx context.Context, db *gorm.DB) {
	InitDefault(ctx, db)
}

// StartRecording is deprecated, use InitDefault instead
func StartRecording(ctx context.Context, db *gorm.DB) {
	// Just delegate to the new interface-based system
	InitDefault(ctx, db)

	// Set the legacy flag for any code that checks it
	recMutex.Lock()
	recStarted = true
	recMutex.Unlock()

	// Wait for context cancellation to reset the flag
	go func() {
		<-ctx.Done()
		recMutex.Lock()
		recStarted = false
		recMutex.Unlock()
	}()
}
