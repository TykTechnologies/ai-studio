// internal/services/analytics_handler.go
package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MicrogatewaAnalyticsHandler implements the midsommar analytics interface
// and converts analytics data to microgateway's analytics_events format
type MicrogatewaAnalyticsHandler struct {
	db            *gorm.DB
	config        *config.AnalyticsConfig
	pendingEvents map[string]uint // Map request ID to event ID for matching
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
		pendingEvents: make(map[string]uint),
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
		if h.config != nil {
			if h.config.StoreRequestBodies {
				event.RequestBody = h.truncateBody(proxyLog.RequestBody, h.config.MaxBodySize)
			}

			if h.config.StoreResponseBodies {
				event.ResponseBody = h.truncateBody(proxyLog.ResponseBody, h.config.MaxBodySize)
			}
		}

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

// Stop gracefully shuts down the analytics handler
func (h *MicrogatewaAnalyticsHandler) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
}

// RecordChatRecord implements the midsommar analytics interface
// Merges ChatRecord data into existing ProxyLog event (deduplication) or creates standalone event
func (h *MicrogatewaAnalyticsHandler) RecordChatRecord(record *models.LLMChatRecord) {
	log.Debug().
		Uint("app_id", record.AppID).
		Uint("llm_id", record.LLMID).
		Str("model", record.Name).
		Str("interaction_type", string(record.InteractionType)).
		Int("total_tokens", record.TotalTokens).
		Msg("Recording chat record analytics - attempting merge with proxy log event")

	// Try to find matching ProxyLog event created ~microseconds ago
	matchKey := fmt.Sprintf("pending_%d_%d", record.AppID, record.TimeStamp.Unix())
	existingEventID, found := h.findEventForMerge(matchKey)

	if found {
		// MERGE: Update existing event with ChatRecord data (richer token/cost info)
		log.Debug().
			Uint("existing_event_id", existingEventID).
			Str("match_key", matchKey).
			Msg("Found matching proxy log event - merging chat record data")

		// Update the existing event with ALL parsed data from ChatRecord
		// ChatRecord data comes from vendor-specific parsers and is ALWAYS correct
		llmIDUint := uint(record.LLMID)
		updates := map[string]interface{}{
			"llm_id":                    &llmIDUint,                    // From embedded gateway
			"name":                      record.Name,                   // Model name from vendor parser
			"vendor":                    record.Vendor,                 // Vendor from embedded gateway
			"interaction_type":          string(record.InteractionType), // Chat vs proxy
			"prompt_tokens":             record.PromptTokens,           // From vendor parser (works for streaming!)
			"response_tokens":           record.ResponseTokens,         // From vendor parser (works for streaming!)
			"total_tokens":              record.TotalTokens,            // From vendor parser
			"cache_write_prompt_tokens": record.CacheWritePromptTokens, // From vendor parser
			"cache_read_prompt_tokens":  record.CacheReadPromptTokens,  // From vendor parser
			"cost":                      record.Cost,                   // Calculated by embedded gateway
			"currency":                  record.Currency,               // From pricing lookup
			"choices":                   record.Choices,                // From vendor parser
			"tool_calls":                record.ToolCalls,              // From vendor parser
			"chat_id":                   record.ChatID,                 // From session
			"total_time_ms":             record.TotalTimeMS,            // From timing capture
		}

		if err := h.db.Model(&database.AnalyticsEvent{}).Where("id = ?", existingEventID).Updates(updates).Error; err != nil {
			log.Error().Err(err).Uint("event_id", existingEventID).Msg("Failed to merge chat record into existing event")
			return
		}

		log.Debug().
			Uint("event_id", existingEventID).
			Str("model", record.Name).
			Int("prompt_tokens", record.PromptTokens).
			Int("response_tokens", record.ResponseTokens).
			Float64("cost", record.Cost).
			Msg("Successfully merged chat record into proxy log event")

		// Record budget usage if budget service is available and cost > 0
		// This must happen AFTER merge, when we have accurate cost/token data from vendor parser
		if h.budgetService != nil && record.Cost > 0 {
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
					Msg("Failed to record budget usage after merge")
				// Don't fail analytics recording if budget recording fails
			} else {
				log.Debug().
					Uint("app_id", record.AppID).
					Float64("cost", record.Cost).
					Int("total_tokens", record.TotalTokens).
					Msg("Budget usage recorded successfully after merge")
			}
		}

		// NOW execute analytics plugins with the MERGED data for pulse transmission
		if h.pluginManager != nil {
			// Fetch the merged event from database to get request/response bodies
			var mergedEvent database.AnalyticsEvent
			if err := h.db.First(&mergedEvent, existingEventID).Error; err != nil {
				log.Error().Err(err).Msg("Failed to fetch merged event for plugin execution")
			} else {
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
					RequestID:              fmt.Sprintf("proxy_%d_%d", record.AppID, record.TimeStamp.UnixNano()),
					// Include request/response bodies from the merged event
					RequestBody:            mergedEvent.RequestBody,
					ResponseBody:           mergedEvent.ResponseBody,
				}

				// Execute analytics plugins (this buffers data in pulse plugin)
				if err := h.pluginManager.ExecuteDataCollectionPlugins("analytics", analyticsData); err != nil {
					log.Error().Err(err).Msg("Failed to execute analytics plugins after merge")
				} else {
					log.Debug().Msg("Analytics plugins executed with merged data - event buffered for pulse")
				}
			}
		}

		return // Done - event merged, budget recorded, and plugins executed
	}

	// NO MATCH: This is a standalone chat interaction (not via proxy)
	// Create new analytics event from ChatRecord only
	log.Debug().
		Str("match_key", matchKey).
		Msg("No matching proxy log found - creating standalone chat analytics event")

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
			RequestID:              fmt.Sprintf("chat_%d_%d", record.AppID, record.TimeStamp.UnixNano()),
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

	// Write to database
	if err := h.db.Create(event).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create standalone chat analytics event")
	} else {
		log.Debug().
			Uint("event_id", event.ID).
			Str("request_id", event.RequestID).
			Int("total_tokens", event.TotalTokens).
			Float64("cost", event.Cost).
			Msg("Standalone chat analytics event created (no proxy log match)")
	}

	// Record budget usage for standalone events if budget service is available and cost > 0
	if h.budgetService != nil && record.Cost > 0 {
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
				Msg("Failed to record budget usage for standalone event")
			// Don't fail analytics recording if budget recording fails
		} else {
			log.Debug().
				Uint("app_id", record.AppID).
				Float64("cost", record.Cost).
				Int("total_tokens", record.TotalTokens).
				Msg("Budget usage recorded successfully for standalone event")
		}
	}
}

// RecordChatLogEntry implements the midsommar analytics interface
// For detailed logging - we can store this in analytics metadata or ignore for now
func (h *MicrogatewaAnalyticsHandler) RecordChatLogEntry(entry *models.LLMChatLogEntry) {
	log.Debug().
		Str("prompt", entry.Prompt[:min(50, len(entry.Prompt))]).
		Str("vendor", entry.Vendor).
		Msg("Chat log entry (stored in analytics metadata)")
	
	// For now, we'll just log this - could store in analytics event metadata if needed
}

// RecordProxyLog implements the midsommar analytics interface
// Creates analytics events directly from AI Gateway proxy logs
// This is called FIRST for each request, creating a pending event that may be enriched by RecordChatRecord
func (h *MicrogatewaAnalyticsHandler) RecordProxyLog(proxyLog *models.ProxyLog) {
	log.Debug().
		Uint("app_id", proxyLog.AppID).
		Uint("user_id", proxyLog.UserID).
		Str("vendor", proxyLog.Vendor).
		Int("response_code", proxyLog.ResponseCode).
		Time("proxy_timestamp", proxyLog.TimeStamp).
		Int("request_body_size", len(proxyLog.RequestBody)).
		Int("response_body_size", len(proxyLog.ResponseBody)).
		Msg("Processing proxy log - creating pending analytics event")

	// Execute data collection plugins for proxy logs
	if h.pluginManager != nil {
		// Convert to plugin format
		pluginData := &interfaces.ProxyLogData{
			AppID:        proxyLog.AppID,
			UserID:       proxyLog.UserID,
			Vendor:       proxyLog.Vendor,
			RequestBody:  []byte(proxyLog.RequestBody),
			ResponseBody: []byte(proxyLog.ResponseBody),
			ResponseCode: proxyLog.ResponseCode,
			Timestamp:    proxyLog.TimeStamp,
			RequestID:    fmt.Sprintf("proxy_%d_%d", proxyLog.AppID, proxyLog.TimeStamp.UnixNano()),
		}
		
		// Execute proxy log plugins
		if err := h.pluginManager.ExecuteDataCollectionPlugins("proxy_log", pluginData); err != nil {
			log.Error().Err(err).Msg("Failed to execute proxy log data collection plugins")
		}
		
		// Note: proxy log replacement only affects proxy log storage, not analytics processing
		// We continue with analytics processing regardless of proxy log replacement
	}

	// Note: ProxyLog contains RAW request/response bodies but NO parsed token data
	// We create a skeleton event here that will be enriched by RecordChatRecord merge
	// The embedded gateway's vendor-specific parsers extract ALL analytics data

	// Create skeleton analytics event (will be enriched when ChatRecord arrives)
	event := &database.AnalyticsEvent{
		RequestID:    fmt.Sprintf("proxy_%d_%d", proxyLog.AppID, proxyLog.TimeStamp.UnixNano()),
		AppID:        proxyLog.AppID,
		UserID:       proxyLog.UserID,
		Vendor:       proxyLog.Vendor,
		StatusCode:   proxyLog.ResponseCode,
		TimeStamp:    proxyLog.TimeStamp,
		CreatedAt:    proxyLog.TimeStamp,

		// Store request/response bodies immediately (if configured)
		RequestBody:  h.truncateBodyIfConfigured(proxyLog.RequestBody),
		ResponseBody: h.truncateBodyIfConfigured(proxyLog.ResponseBody),

		// NO PARSED DATA - will come from ChatRecord merge:
		// PromptTokens, ResponseTokens, Cost, Model, Choices, ToolCalls, etc.
		// All set to zero/empty until ChatRecord enriches this event
	}

	// Create the analytics event and store for potential merge with ChatRecord
	if err := h.db.Create(event).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create analytics event from proxy log")
		return
	}

	// Store event ID for matching with incoming ChatRecord (uses AppID+Timestamp key)
	h.storeEventForMatching(fmt.Sprintf("pending_%d_%d", proxyLog.AppID, proxyLog.TimeStamp.Unix()), event.ID)

	log.Debug().
		Uint("event_id", event.ID).
		Str("request_id", event.RequestID).
		Msg("Skeleton analytics event created from proxy log (awaiting ChatRecord merge for tokens/cost)")

	// NOTE: Budget recording happens in RecordChatRecord where we have actual cost/token data
	// ProxyLog creates skeleton events; ChatRecord enriches them with parsed vendor data
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

// truncateBodyIfConfigured truncates body if storage is enabled in config
func (h *MicrogatewaAnalyticsHandler) truncateBodyIfConfigured(body string) string {
	if h.config == nil {
		return body // No config, store as-is
	}

	// Check if we should store bodies (both flags should be true by default)
	if h.config.MaxBodySize <= 0 {
		return "" // Disabled
	}

	return h.truncateBody(body, h.config.MaxBodySize)
}

// storeEventForMatching stores an event ID for later matching with chat record
func (h *MicrogatewaAnalyticsHandler) storeEventForMatching(matchKey string, eventID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pendingEvents[matchKey] = eventID

	// Clean up old entries (older than 10 seconds)
	// This prevents memory leaks from unmatched events
	go func() {
		time.Sleep(10 * time.Second)
		h.mu.Lock()
		delete(h.pendingEvents, matchKey)
		h.mu.Unlock()
	}()
}

// findEventForMerge finds a pending event ID for merging chat record data
func (h *MicrogatewaAnalyticsHandler) findEventForMerge(matchKey string) (uint, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	eventID, exists := h.pendingEvents[matchKey]
	if exists {
		// Remove from pending map after finding (one-time merge)
		go func() {
			h.mu.Lock()
			delete(h.pendingEvents, matchKey)
			h.mu.Unlock()
		}()
	}

	return eventID, exists
}

// findEventForMatching finds an event ID by request pattern matching
func (h *MicrogatewaAnalyticsHandler) findEventForMatching(proxyLog *models.ProxyLog) (uint, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	// Try to match by pattern (app ID and timestamp closeness)
	expectedPattern := fmt.Sprintf("req_%d_", proxyLog.AppID)
	
	for requestID, eventID := range h.pendingEvents {
		if len(requestID) >= len(expectedPattern) && requestID[:len(expectedPattern)] == expectedPattern {
			// Check if timestamps are close (within 10 seconds)
			var event database.AnalyticsEvent
			if err := h.db.First(&event, eventID).Error; err == nil {
				timeDiff := proxyLog.TimeStamp.Sub(event.CreatedAt)
				if timeDiff < 0 {
					timeDiff = -timeDiff
				}
				if timeDiff < 10*time.Second {
					return eventID, true
				}
			}
		}
	}
	
	return 0, false
}

// RecordToolCall implements the midsommar analytics interface
// Records tool usage analytics
// Note: Tool calls in microgateway context are typically tracked within LLM responses
// This standalone method is for AI Studio compatibility
func (h *MicrogatewaAnalyticsHandler) RecordToolCall(name string, timestamp time.Time, execTimeMs int, toolID uint) {
	log.Debug().
		Str("tool_name", name).
		Uint("tool_id", toolID).
		Int("exec_time_ms", execTimeMs).
		Msg("Recording standalone tool call analytics (AI Studio compatibility)")

	// Note: In microgateway, tool calls are tracked as part of LLM analytics
	// This method is here for AI Studio interface compatibility
	// Standalone tool call tracking may not be applicable in proxy-only mode
}

// RecordChatRecordsBatch implements batch recording for microgateway analytics
// This method is non-blocking and returns immediately to avoid impacting request latency
func (h *MicrogatewaAnalyticsHandler) RecordChatRecordsBatch(records []*models.LLMChatRecord) {
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
func (h *MicrogatewaAnalyticsHandler) RecordProxyLogsBatch(logs []*models.ProxyLog) {
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