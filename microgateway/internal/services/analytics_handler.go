// internal/services/analytics_handler.go
package services

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	internalPlugins "github.com/TykTechnologies/midsommar/microgateway/internal/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MicrogatewaAnalyticsHandler implements the midsommar analytics interface
// and converts analytics data to microgateway's analytics_events format
type MicrogatewaAnalyticsHandler struct {
	db            *gorm.DB
	config        *config.AnalyticsConfig
	// writer takes the analytics rows; nil writes each row directly.
	writer *AnalyticsWriter
	// lastIDNanos keeps request IDs unique (nextRequestID).
	lastIDNanos atomic.Int64
	mu            sync.RWMutex
	pluginManager *plugins.PluginManager // For global data collection plugins
	budgetService BudgetServiceInterface // For recording budget usage
	// Batch processing channels for async non-blocking batch operations
	chatRecordBatchChan chan []*models.LLMChatRecord
	proxyLogBatchChan   chan []*models.ProxyLog
	ctx                 context.Context
	cancel              context.CancelFunc
	workerStarted       bool
	workerMutex         sync.Mutex
}

// NewMicrogatewaAnalyticsHandler creates a new analytics handler for the microgateway
func NewMicrogatewaAnalyticsHandler(db *gorm.DB, analyticsConfig *config.AnalyticsConfig, pluginManager *plugins.PluginManager, budgetService BudgetServiceInterface) *MicrogatewaAnalyticsHandler {
	ctx, cancel := context.WithCancel(context.Background())

	batchBufferSize := 100 // Default batch channel buffer size
	if analyticsConfig != nil && analyticsConfig.BufferSize > 0 {
		batchBufferSize = analyticsConfig.BufferSize / 10
		if batchBufferSize < 10 {
			batchBufferSize = 10
		}
	}

	return &MicrogatewaAnalyticsHandler{
		db:            db,
		config:        analyticsConfig,
		pluginManager: pluginManager,
		budgetService: budgetService,
		chatRecordBatchChan: make(chan []*models.LLMChatRecord, batchBufferSize),
		proxyLogBatchChan:   make(chan []*models.ProxyLog, batchBufferSize),
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// ensureWorkerStarted ensures the async batch worker is running
func (h *MicrogatewaAnalyticsHandler) ensureWorkerStarted() {
	h.workerMutex.Lock()
	defer h.workerMutex.Unlock()

	if !h.workerStarted {
		h.workerStarted = true
		go h.startBatchWorker()
	}
}

// startBatchWorker runs the batch processing worker
func (h *MicrogatewaAnalyticsHandler) startBatchWorker() {
	for {
		select {
		case records := <-h.chatRecordBatchChan:
			h.processChatRecordsBatchSync(records)
		case logs := <-h.proxyLogBatchChan:
			h.processProxyLogsBatchSync(logs)
		case <-h.ctx.Done():
			log.Debug().Msg("Shutting down microgateway analytics batch worker")
			close(h.chatRecordBatchChan)
			close(h.proxyLogBatchChan)
			return
		}
	}
}

// processChatRecordsBatchSync processes chat records batch synchronously in worker
func (h *MicrogatewaAnalyticsHandler) processChatRecordsBatchSync(records []*models.LLMChatRecord) {
	startTime := time.Now()

	events := make([]*database.AnalyticsEvent, len(records))
	for i, record := range records {
		event := &database.AnalyticsEvent{
			RequestID:              fmt.Sprintf("chat_%d_%d", record.AppID, record.TimeStamp.UnixNano()),
			AppID:                  record.AppID,
			LLMID:                  &record.LLMID,

			// Fields matching LLMChatRecord for parity
			UserID:                 record.UserID,
			Name:                   record.Name,
			Vendor:                 record.Vendor,
			InteractionType:        string(record.InteractionType),
			Choices:                record.Choices,
			ToolCalls:              record.ToolCalls,
			ChatID:                 record.ChatID,
			Currency:               record.Currency,

			// Request/Response details
			Endpoint:               "/v1/chat/completions",
			Method:                 "POST",
			StatusCode:             200, // Determined from success of chat interaction

			// Token tracking
			PromptTokens:           record.PromptTokens,
			ResponseTokens:         record.ResponseTokens,
			TotalTokens:            record.TotalTokens,
			CacheWritePromptTokens: record.CacheWritePromptTokens,
			CacheReadPromptTokens:  record.CacheReadPromptTokens,

			// Cost and timing
			Cost:                   record.Cost,
			TotalTimeMS:            record.TotalTimeMS,

			ErrorMessage:           "",
			TimeStamp:              record.TimeStamp,
			CreatedAt:              record.TimeStamp,
		}
		events[i] = event
	}

	// Use GORM CreateInBatches for efficient bulk insert
	err := h.db.CreateInBatches(events, 100).Error
	processingTime := time.Since(startTime)

	if err != nil {
		log.Error().Err(err).Int("count", len(records)).Int64("processing_time_ms", processingTime.Milliseconds()).
			Msg("Failed to create chat record batch")
	} else {
		log.Debug().Int("count", len(records)).Int64("processing_time_ms", processingTime.Milliseconds()).
			Float64("records_per_second", float64(len(records))/processingTime.Seconds()).
			Msg("Created chat record batch successfully")
	}
}

// processProxyLogsBatchSync processes proxy logs batch synchronously in worker
// Note: Batch ProxyLogs typically don't have corresponding ChatRecords (e.g., from pulse reception)
// So we create complete events from ProxyLog data only
func (h *MicrogatewaAnalyticsHandler) processProxyLogsBatchSync(logs []*models.ProxyLog) {
	startTime := time.Now()

	events := make([]*database.AnalyticsEvent, len(logs))
	for i, proxyLog := range logs {
		// For batch processing, create skeleton events
		// These are typically from edge pulse reception where we don't get corresponding ChatRecords
		event := &database.AnalyticsEvent{
			RequestID:    fmt.Sprintf("proxy_%d_%d", proxyLog.AppID, proxyLog.TimeStamp.UnixNano()),
			AppID:        proxyLog.AppID,
			UserID:       proxyLog.UserID,
			Vendor:       proxyLog.Vendor,
			StatusCode:   proxyLog.ResponseCode,
			TimeStamp:    proxyLog.TimeStamp,
			CreatedAt:    proxyLog.TimeStamp,

			// NO PARSED DATA - batch ProxyLogs don't have corresponding ChatRecords
			// If this is from pulse reception, the control server will have the full data
		}

		// Add request/response bodies if configured
		event.RequestBody = h.requestBodyToStore(proxyLog.RequestBody)
		event.ResponseBody = h.responseBodyToStore(proxyLog.ResponseBody)

		events[i] = event
	}

	// Use GORM CreateInBatches for efficient bulk insert
	err := h.db.CreateInBatches(events, 100).Error
	processingTime := time.Since(startTime)

	if err != nil {
		log.Error().Err(err).Int("count", len(logs)).Int64("processing_time_ms", processingTime.Milliseconds()).
			Msg("Failed to create proxy log batch")
	} else {
		log.Debug().Int("count", len(logs)).Int64("processing_time_ms", processingTime.Milliseconds()).
			Float64("records_per_second", float64(len(logs))/processingTime.Seconds()).
			Str("first_vendor", logs[0].Vendor).
			Msg("Created proxy log batch successfully")
	}
}

// SetWriter hands the handler's rows to w instead of writing each directly.
func (h *MicrogatewaAnalyticsHandler) SetWriter(w *AnalyticsWriter) {
	h.writer = w
}

// Stop gracefully shuts down the analytics handler
func (h *MicrogatewaAnalyticsHandler) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
}

// RecordExchange implements analytics.ExchangeRecorder: a proxied request's
// proxy log and its chat record (nil when the response yielded no usage)
// become one analytics row, built in memory and handed to the writer.
//
// The two used to arrive as separate calls, and the row was inserted from the
// proxy log, updated from the chat record and read back for the pulse: three
// statements per request, paired up again by app and second, which merged
// concurrent requests into each other's rows.
func (h *MicrogatewaAnalyticsHandler) RecordExchange(_ context.Context, proxyLog *models.ProxyLog, record *models.LLMChatRecord) {
	requestID := h.nextRequestID("proxy", proxyLog.AppID)

	// Execute data collection plugins for proxy logs
	if h.pluginManager != nil {
		pluginData := &interfaces.ProxyLogData{
			AppID:        proxyLog.AppID,
			UserID:       proxyLog.UserID,
			Vendor:       proxyLog.Vendor,
			RequestBody:  []byte(proxyLog.RequestBody),
			ResponseBody: []byte(proxyLog.ResponseBody),
			ResponseCode: proxyLog.ResponseCode,
			Timestamp:    proxyLog.TimeStamp,
			RequestID:    requestID,
		}
		if err := h.pluginManager.ExecuteDataCollectionPlugins("proxy_log", pluginData); err != nil {
			log.Error().Err(err).Msg("Failed to execute proxy log data collection plugins")
		}
	}

	event := h.eventFromProxyLog(proxyLog, requestID)
	if record != nil {
		applyChatRecord(event, record)

		// Budget usage needs the parsed cost, so it is recorded with the
		// chat record.
		h.recordBudgetUsage(record)

		// Analytics plugins (the pulse) get the complete row.
		if h.pluginManager != nil {
			analyticsData := &interfaces.AnalyticsData{
				LLMID:                  record.LLMID,
				ModelName:              record.Name,
				Vendor:                 record.Vendor,
				PromptTokens:           record.PromptTokens,
				ResponseTokens:         record.ResponseTokens,
				TotalTokens:            record.TotalTokens,
				CacheWritePromptTokens: record.CacheWritePromptTokens,
				CacheReadPromptTokens:  record.CacheReadPromptTokens,
				Cost:                   record.Cost,
				Currency:               record.Currency,
				AppID:                  record.AppID,
				UserID:                 record.UserID,
				Timestamp:              record.TimeStamp,
				ToolCalls:              record.ToolCalls,
				Choices:                record.Choices,
				RequestID:              requestID,
				StatusCode:             event.StatusCode, // Pass actual HTTP status code (e.g., 403 for budget exceeded)
				// Bodies as stored: empty unless ANALYTICS_STORE_REQUESTS/RESPONSES
				RequestBody:         event.RequestBody,
				ResponseBody:        event.ResponseBody,
				FailoverFromLLMID:   event.FailoverFromLLMID,
				FailoverAttempt:     event.FailoverAttempt,
				RouterKind:          event.RouterKind,
				RouterSlug:          event.RouterSlug,
				RouterPool:          event.RouterPoolName,
				Route:               event.Route,
				RouteReason:         event.RouteReason,
				RouterSourceModel:   event.RouterSourceModel,
				RouterTargetModel:   event.RouterTargetModel,
				RouterSelectionAlgo: event.RouterSelectionAlgo,
				RouteScore:          event.RouteScore,
				ShadowRoute:         event.ShadowRoute,
			}
			if err := h.pluginManager.ExecuteDataCollectionPlugins("analytics", analyticsData); err != nil {
				log.Error().Err(err).Msg("Failed to execute analytics plugins for exchange")
			}
		}
	}

	h.store(event)
}

// RecordChatRecord implements the midsommar analytics interface
// Records a chat record that arrived without a proxy log (a standalone chat
// interaction); proxied requests come through RecordExchange.
func (h *MicrogatewaAnalyticsHandler) RecordChatRecord(_ context.Context, record *models.LLMChatRecord) {
	log.Debug().
		Uint("app_id", record.AppID).
		Uint("llm_id", record.LLMID).
		Str("model", record.Name).
		Str("interaction_type", string(record.InteractionType)).
		Int("total_tokens", record.TotalTokens).
		Msg("Recording standalone chat record analytics")

	requestID := h.nextRequestID("chat", record.AppID)

	// Execute analytics data collection plugins
	if h.pluginManager != nil {
		analyticsData := &interfaces.AnalyticsData{
			LLMID:                  record.LLMID,
			ModelName:              record.Name,
			Vendor:                 record.Vendor,
			PromptTokens:           record.PromptTokens,
			ResponseTokens:         record.ResponseTokens,
			TotalTokens:            record.TotalTokens,
			CacheWritePromptTokens: record.CacheWritePromptTokens,
			CacheReadPromptTokens:  record.CacheReadPromptTokens,
			Cost:                   record.Cost,
			Currency:               record.Currency,
			AppID:                  record.AppID,
			UserID:                 record.UserID,
			Timestamp:              record.TimeStamp,
			ToolCalls:              record.ToolCalls,
			Choices:                record.Choices,
			RequestID:              requestID,
			StatusCode:             200, // Standalone chat interactions are successful by definition
		}

		// Execute analytics plugins
		if err := h.pluginManager.ExecuteDataCollectionPlugins("analytics", analyticsData); err != nil {
			log.Error().Err(err).Msg("Failed to execute analytics data collection plugins for chat record")
		}

		// Check if any plugins are configured to replace database storage for analytics
		if h.pluginManager.ShouldReplaceDatabaseStorage("analytics") {
			log.Debug().Msg("Analytics database storage replaced by plugin - skipping database write for chat record")
			return
		}
	}

	// Create analytics event from chat record (standalone chat interaction)
	event := &database.AnalyticsEvent{
		RequestID: requestID,
		AppID:     record.AppID,

		// Request/Response details
		Endpoint:   "/v1/chat/completions",
		Method:     "POST",
		StatusCode: 200, // Determined from success of chat interaction

		ErrorMessage: "",
		TimeStamp:    record.TimeStamp,
		CreatedAt:    record.TimeStamp,
	}
	applyChatRecord(event, record)
	h.store(event)

	// Record budget usage for standalone events if budget service is available and cost > 0
	h.recordBudgetUsage(record)
}

// eventFromProxyLog builds the analytics row for a proxied request from its
// proxy log.
func (h *MicrogatewaAnalyticsHandler) eventFromProxyLog(proxyLog *models.ProxyLog, requestID string) *database.AnalyticsEvent {
	event := &database.AnalyticsEvent{
		RequestID:  requestID,
		AppID:      proxyLog.AppID,
		UserID:     proxyLog.UserID,
		Vendor:     proxyLog.Vendor,
		StatusCode: proxyLog.ResponseCode,
		TimeStamp:  proxyLog.TimeStamp,
		CreatedAt:  proxyLog.TimeStamp,

		// Store request/response bodies (if configured)
		RequestBody:  h.requestBodyToStore(proxyLog.RequestBody),
		ResponseBody: h.responseBodyToStore(proxyLog.ResponseBody),
	}

	// Attribute the event to the specific LLM entry (not just the vendor type)
	// so the LLM detail view can isolate logs when several entries share a vendor.
	if proxyLog.LLMID != 0 {
		llmID := proxyLog.LLMID
		event.LLMID = &llmID
	}
	// Failover marker: carried through to the pulse so the hub's ProxyLog can
	// say which primary this rung was failing over from.
	if proxyLog.FailoverFromLLMID != nil {
		from := *proxyLog.FailoverFromLLMID
		event.FailoverFromLLMID = &from
		event.FailoverAttempt = proxyLog.FailoverAttempt
	}

	// Routing decision, when the request was addressed to a router. Carried
	// on the proxy log by the gateway's loopback marker, so it belongs to this
	// exact request (it used to be looked up by second, which mixed up
	// concurrent requests).
	event.RouterKind = proxyLog.RouterKind
	event.RouterSlug = proxyLog.RouterSlug
	event.RouterPoolName = proxyLog.RouterPool
	event.Route = proxyLog.Route
	event.RouteReason = proxyLog.RouteReason
	event.RouterSourceModel = proxyLog.RouteSourceModel
	event.RouterTargetModel = proxyLog.RouteTargetModel
	event.RouterSelectionAlgo = proxyLog.RouteSelection
	event.RouteScore = proxyLog.RouteScore
	event.ShadowRoute = proxyLog.ShadowRoute
	return event
}

// applyChatRecord fills the row's parsed usage from the chat record. The chat
// record's data comes from the vendor-specific parsers.
func applyChatRecord(event *database.AnalyticsEvent, record *models.LLMChatRecord) {
	llmID := record.LLMID
	event.LLMID = &llmID
	event.UserID = record.UserID
	event.Name = record.Name
	event.Vendor = record.Vendor
	event.InteractionType = string(record.InteractionType)
	event.PromptTokens = record.PromptTokens
	event.ResponseTokens = record.ResponseTokens
	event.TotalTokens = record.TotalTokens
	event.CacheWritePromptTokens = record.CacheWritePromptTokens
	event.CacheReadPromptTokens = record.CacheReadPromptTokens
	event.Cost = record.Cost // Already in AI Studio format (dollars * 10000) from proxy layer
	event.Currency = record.Currency
	event.Choices = record.Choices
	event.ToolCalls = record.ToolCalls
	event.ChatID = record.ChatID
	event.TotalTimeMS = record.TotalTimeMS
}

// recordBudgetUsage records a priced request against its App's budget.
func (h *MicrogatewaAnalyticsHandler) recordBudgetUsage(record *models.LLMChatRecord) {
	if h.budgetService == nil || record.Cost <= 0 {
		return
	}
	if err := h.budgetService.RecordUsage(
		record.AppID,
		&record.LLMID,
		int64(record.TotalTokens),
		record.Cost,
		int64(record.PromptTokens),
		int64(record.ResponseTokens),
	); err != nil {
		log.Warn().Err(err).
			Uint("app_id", record.AppID).
			Float64("cost", record.Cost).
			Msg("Failed to record budget usage")
		// Don't fail analytics recording if budget recording fails
	}
}

// store hands the row to the writer, or writes it directly when the handler
// has none.
func (h *MicrogatewaAnalyticsHandler) store(event *database.AnalyticsEvent) {
	if h.writer != nil {
		h.writer.Enqueue(event)
		return
	}
	if err := h.db.Create(event).Error; err != nil {
		log.Error().Err(err).Str("request_id", event.RequestID).Msg("Failed to create analytics event")
	}
}

// nextRequestID returns a request ID unique within this process:
// <prefix>_<app>_<nanoseconds>, the nanoseconds taken from the clock but
// never repeated. Clock-derived IDs alone collided under load, and the
// request_id column is unique.
func (h *MicrogatewaAnalyticsHandler) nextRequestID(prefix string, appID uint) string {
	now := time.Now().UnixNano()
	for {
		last := h.lastIDNanos.Load()
		n := now
		if n <= last {
			n = last + 1
		}
		if h.lastIDNanos.CompareAndSwap(last, n) {
			return fmt.Sprintf("%s_%d_%d", prefix, appID, n)
		}
	}
}

// RecordChatLogEntry implements the midsommar analytics interface
// For detailed logging - we can store this in analytics metadata or ignore for now
func (h *MicrogatewaAnalyticsHandler) RecordChatLogEntry(_ context.Context, entry *models.LLMChatLogEntry) {
	log.Debug().
		Str("prompt", entry.Prompt[:min(50, len(entry.Prompt))]).
		Str("vendor", entry.Vendor).
		Msg("Chat log entry (stored in analytics metadata)")
	
	// For now, we'll just log this - could store in analytics event metadata if needed
}

// RecordProxyLog implements the midsommar analytics interface
// Records a proxy log that has no chat record (a refused or failed request);
// proxied requests with usage come through RecordExchange.
func (h *MicrogatewaAnalyticsHandler) RecordProxyLog(ctx context.Context, proxyLog *models.ProxyLog) {
	h.RecordExchange(ctx, proxyLog, nil)
}

// truncateBody truncates request/response bodies to the configured maximum size
func (h *MicrogatewaAnalyticsHandler) truncateBody(body string, maxSize int) string {
	if maxSize <= 0 {
		return "" // Disabled
	}

	if len(body) <= maxSize {
		return body
	}

	return body[:maxSize] + "... [truncated]"
}

// requestBodyToStore returns the request body as it may be stored, or "" when
// ANALYTICS_STORE_REQUESTS is off. The pulse forwards bodies read back from the
// stored event, so this also decides whether the body leaves the gateway.
func (h *MicrogatewaAnalyticsHandler) requestBodyToStore(body string) string {
	if h.config == nil || !h.config.StoreRequestBodies {
		return ""
	}
	return h.truncateBody(body, h.config.MaxBodySize)
}

// responseBodyToStore is requestBodyToStore for ANALYTICS_STORE_RESPONSES.
func (h *MicrogatewaAnalyticsHandler) responseBodyToStore(body string) string {
	if h.config == nil || !h.config.StoreResponseBodies {
		return ""
	}
	return h.truncateBody(body, h.config.MaxBodySize)
}

// RecordToolCall implements the midsommar analytics interface
// Records tool usage analytics
// Note: Tool calls in microgateway context are typically tracked within LLM responses
// This standalone method is for AI Studio compatibility
func (h *MicrogatewaAnalyticsHandler) RecordToolCall(_ context.Context, name string, timestamp time.Time, execTimeMs int, toolID uint) {
	log.Debug().
		Str("tool_name", name).
		Uint("tool_id", toolID).
		Int("exec_time_ms", execTimeMs).
		Msg("Recording standalone tool call analytics")

	if h.pluginManager == nil {
		return
	}

	// Forward to the pulse plugin so the control plane can attribute the call
	// to a tool and an operation. The aggregate ToolCalls counter that rides on
	// an analytics event carries no operation, so without this an edge-served
	// tool call is invisible to the control plane's tool analytics, and every
	// chart under-reports by whatever share of traffic the edges handle.
	h.pluginManager.BufferToolCalls([]internalPlugins.ToolCallBuffer{{
		ToolID:      uint32(toolID),
		OperationID: name,
		ExecTimeMs:  int32(execTimeMs),
		Timestamp:   timestamp,
	}})
}

// RecordChatRecordsBatch implements batch recording for microgateway analytics
// This method is non-blocking and returns immediately to avoid impacting request latency
func (h *MicrogatewaAnalyticsHandler) RecordChatRecordsBatch(_ context.Context, records []*models.LLMChatRecord) {
	if len(records) == 0 {
		return
	}

	// Ensure async worker is started
	h.ensureWorkerStarted()

	// Send batch to async worker - non-blocking to avoid request latency
	select {
	case h.chatRecordBatchChan <- records:
		log.Debug().Int("count", len(records)).Msg("Sent chat record batch to async worker")
	default:
		log.Warn().Int("count", len(records)).Msg("Chat record batch buffer full, dropping batch")
	}
}

// RecordProxyLogsBatch implements batch recording for microgateway analytics
// This method is non-blocking and returns immediately to avoid impacting request latency
func (h *MicrogatewaAnalyticsHandler) RecordProxyLogsBatch(_ context.Context, logs []*models.ProxyLog) {
	if len(logs) == 0 {
		return
	}

	// Ensure async worker is started
	h.ensureWorkerStarted()

	// Send batch to async worker - non-blocking to avoid request latency
	select {
	case h.proxyLogBatchChan <- logs:
		log.Debug().Int("count", len(logs)).Msg("Sent proxy log batch to async worker")
	default:
		log.Warn().Int("count", len(logs)).Msg("Proxy log batch buffer full, dropping batch")
	}
}

// RecordComplianceEvents forwards compliance events to the analytics pulse plugin
// for batch transmission to the control plane during the next pulse.
func (h *MicrogatewaAnalyticsHandler) RecordComplianceEvents(_ context.Context, events []*models.ComplianceEvent) {
	if len(events) == 0 || h.pluginManager == nil {
		return
	}

	bufferEvents := make([]internalPlugins.ComplianceEventBuffer, len(events))
	for i, e := range events {
		bufferEvents[i] = internalPlugins.ComplianceEventBuffer{
			AppID:       uint32(e.AppID),
			UserID:      uint32(e.UserID),
			LLMID:       uint32(e.LLMID),
			FilterName:  e.FilterName,
			FilterScope: e.FilterScope,
			EventType:   e.EventType,
			Severity:    e.Severity,
			Description: e.Description,
			Metadata:    e.Metadata,
			Vendor:      e.Vendor,
			ModelName:   e.ModelName,
			Timestamp:   e.TimeStamp,
		}
	}

	h.pluginManager.BufferComplianceEvents(bufferEvents)
}

// SetAsGlobalHandler sets this handler as the global midsommar analytics handler
func (h *MicrogatewaAnalyticsHandler) SetAsGlobalHandler() {
	log.Debug().Msg("Setting microgateway analytics handler as global handler")
	analytics.SetHandler(h)
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}