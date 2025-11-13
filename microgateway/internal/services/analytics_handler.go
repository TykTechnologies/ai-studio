// internal/services/analytics_handler.go
package services

import (
	"context"
	"encoding/json"
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
	// Batch processing channels for async non-blocking batch operations
	chatRecordBatchChan chan []*models.LLMChatRecord
	proxyLogBatchChan   chan []*models.ProxyLog
	ctx                 context.Context
	cancel              context.CancelFunc
	workerStarted       bool
	workerMutex         sync.Mutex
}

// NewMicrogatewaAnalyticsHandler creates a new analytics handler for the microgateway
func NewMicrogatewaAnalyticsHandler(db *gorm.DB, analyticsConfig *config.AnalyticsConfig, pluginManager *plugins.PluginManager) *MicrogatewaAnalyticsHandler {
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
			log.Info().Msg("Shutting down microgateway analytics batch worker")
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
			Endpoint:               "/v1/chat/completions", // Default endpoint for chat interactions
			Method:                 "POST",
			StatusCode:             200, // Assume success for chat records
			RequestTokens:          record.PromptTokens,
			ResponseTokens:         record.ResponseTokens,
			TotalTokens:            record.TotalTokens,
			CacheWritePromptTokens: record.CacheWritePromptTokens,
			CacheReadPromptTokens:  record.CacheReadPromptTokens,
			Cost:                   record.Cost,
			LatencyMs:              record.TotalTimeMS,
			ErrorMessage:           "",
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
		log.Info().Int("count", len(records)).Int64("processing_time_ms", processingTime.Milliseconds()).
			Float64("records_per_second", float64(len(records))/processingTime.Seconds()).
			Msg("Created chat record batch successfully")
	}
}

// processProxyLogsBatchSync processes proxy logs batch synchronously in worker
func (h *MicrogatewaAnalyticsHandler) processProxyLogsBatchSync(logs []*models.ProxyLog) {
	startTime := time.Now()

	events := make([]*database.AnalyticsEvent, len(logs))
	for i, proxyLog := range logs {
		// Parse token usage and cost from response body if available
		tokens := h.parseTokensFromResponse(proxyLog.ResponseBody)
		model := h.extractModelFromRequest(proxyLog.RequestBody)
		cost := h.parseCostFromResponse(proxyLog.ResponseBody, proxyLog.Vendor, model)
		llmID := h.findLLMIDByVendorAndModel(proxyLog.Vendor, model)

		var llmIDPtr *uint
		if llmID > 0 {
			llmIDPtr = &llmID
		}

		event := &database.AnalyticsEvent{
			RequestID:              fmt.Sprintf("proxy_%d_%d", proxyLog.AppID, proxyLog.TimeStamp.UnixNano()),
			AppID:                  proxyLog.AppID,
			LLMID:                  llmIDPtr,
			Endpoint:               h.extractEndpointFromVendor(proxyLog.Vendor, proxyLog.AppID),
			Method:                 "POST",
			StatusCode:             proxyLog.ResponseCode,
			RequestTokens:          tokens.PromptTokens,
			ResponseTokens:         tokens.ResponseTokens,
			TotalTokens:            tokens.TotalTokens,
			CacheWritePromptTokens: tokens.CacheWriteTokens,
			CacheReadPromptTokens:  tokens.CacheReadTokens,
			Cost:                   cost,
			LatencyMs:              0, // Not available in proxy log
			ErrorMessage:           "",
			CreatedAt:              proxyLog.TimeStamp,
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
		log.Info().Int("count", len(logs)).Int64("processing_time_ms", processingTime.Milliseconds()).
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
// This is for AI Studio chat features - for microgateway, we use RecordProxyLog exclusively
func (h *MicrogatewaAnalyticsHandler) RecordChatRecord(record *models.LLMChatRecord) {
	log.Debug().
		Uint("app_id", record.AppID).
		Uint("llm_id", record.LLMID).
		Str("model", record.Name).
		Msg("RecordChatRecord called - this is for AI Studio chat, not microgateway proxy")
	
	// For microgateway, we handle all analytics via RecordProxyLog
	// This method is here for interface compatibility but not used
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
func (h *MicrogatewaAnalyticsHandler) RecordProxyLog(proxyLog *models.ProxyLog) {
	log.Info().
		Uint("app_id", proxyLog.AppID).
		Uint("user_id", proxyLog.UserID).
		Str("vendor", proxyLog.Vendor).
		Int("response_code", proxyLog.ResponseCode).
		Time("proxy_timestamp", proxyLog.TimeStamp).
		Int("request_body_size", len(proxyLog.RequestBody)).
		Int("response_body_size", len(proxyLog.ResponseBody)).
		Msg("Processing proxy log - executing data collection plugins")

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

	// Parse token usage and cost from response body if available
	tokens := h.parseTokensFromResponse(proxyLog.ResponseBody)
	
	// Extract model name from request body for accurate pricing
	model := h.extractModelFromRequest(proxyLog.RequestBody)
	cost := h.parseCostFromResponse(proxyLog.ResponseBody, proxyLog.Vendor, model)
	
	// Execute analytics data collection plugins
	if h.pluginManager != nil {
		// Extract LLM ID from vendor and model information
		llmID := h.findLLMIDByVendorAndModel(proxyLog.Vendor, model)

		// Convert to analytics plugin format
		analyticsData := &interfaces.AnalyticsData{
			LLMID:          llmID,
			ModelName:      model,
			Vendor:         proxyLog.Vendor,
			PromptTokens:   tokens.PromptTokens,
			ResponseTokens: tokens.ResponseTokens,
			TotalTokens:    tokens.TotalTokens,
			Cost:           cost,
			Currency:       "USD",
			AppID:          proxyLog.AppID,
			UserID:         proxyLog.UserID,
			Timestamp:      proxyLog.TimeStamp,
			ToolCalls:      0, // Could be parsed from request if needed
			Choices:        1, // Default to 1 choice
			RequestID:      fmt.Sprintf("proxy_%d_%d", proxyLog.AppID, proxyLog.TimeStamp.UnixNano()),
			// Include request/response data for pulse plugins
			RequestBody:    proxyLog.RequestBody,
			ResponseBody:   proxyLog.ResponseBody,
		}
		
		// Execute analytics plugins
		if err := h.pluginManager.ExecuteDataCollectionPlugins("analytics", analyticsData); err != nil {
			log.Error().Err(err).Msg("Failed to execute analytics data collection plugins")
		}
		
		// Check if any plugins are configured to replace database storage for analytics
		if h.pluginManager.ShouldReplaceDatabaseStorage("analytics") {
			log.Debug().Msg("Analytics database storage replaced by plugin - skipping database write")
			return
		}
	}
	
	// Extract LLM ID for analytics event
	llmID := h.findLLMIDByVendorAndModel(proxyLog.Vendor, model)
	var llmIDPtr *uint
	if llmID > 0 {
		llmIDPtr = &llmID
	}

	// Create analytics event directly from proxy log
	event := &database.AnalyticsEvent{
		RequestID:              fmt.Sprintf("proxy_%d_%d", proxyLog.AppID, proxyLog.TimeStamp.UnixNano()),
		AppID:                  proxyLog.AppID,
		LLMID:                  llmIDPtr,
		CredentialID:           nil, // Not used in token-only system
		Endpoint:               h.extractEndpointFromVendor(proxyLog.Vendor, proxyLog.AppID),
		Method:                 "POST",
		StatusCode:             proxyLog.ResponseCode,
		RequestTokens:          tokens.PromptTokens,
		ResponseTokens:         tokens.ResponseTokens,
		TotalTokens:            tokens.TotalTokens,
		CacheWritePromptTokens: tokens.CacheWriteTokens,
		CacheReadPromptTokens:  tokens.CacheReadTokens,
		Cost:                   cost,
		LatencyMs:              0, // Not available in proxy log
		ErrorMessage:           "",
		CreatedAt:              proxyLog.TimeStamp,
	}

	// Add request/response bodies if configured
	if h.config.StoreRequestBodies {
		event.RequestBody = h.truncateBody(proxyLog.RequestBody, h.config.MaxBodySize)
		log.Debug().Int("request_size", len(event.RequestBody)).Msg("Storing request body")
	}
	
	if h.config.StoreResponseBodies {
		event.ResponseBody = h.truncateBody(proxyLog.ResponseBody, h.config.MaxBodySize)
		log.Debug().Int("response_size", len(event.ResponseBody)).Msg("Storing response body")
	}

	// Create the analytics event
	if err := h.db.Create(event).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create analytics event from proxy log")
	} else {
		log.Info().
			Uint("event_id", event.ID).
			Str("request_id", event.RequestID).
			Int("total_tokens", event.TotalTokens).
			Float64("cost", event.Cost).
			Msg("Analytics event created successfully from proxy log")
	}
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

// storeEventForMatching stores an event ID for later matching with proxy log
func (h *MicrogatewaAnalyticsHandler) storeEventForMatching(requestID string, eventID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pendingEvents[requestID] = eventID
	
	// Clean up old entries (older than 1 minute)
	// This prevents memory leaks from unmatched events
	go func() {
		time.Sleep(60 * time.Second)
		h.mu.Lock()
		delete(h.pendingEvents, requestID)
		h.mu.Unlock()
	}()
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
func (h *MicrogatewaAnalyticsHandler) RecordToolCall(name string, timestamp time.Time, execTimeMs int, toolID uint) {
	log.Debug().
		Str("tool_name", name).
		Uint("tool_id", toolID).
		Int("exec_time_ms", execTimeMs).
		Msg("Recording tool call analytics")

	// Create analytics event for tool call
	event := &database.AnalyticsEvent{
		RequestID:     fmt.Sprintf("tool_%d_%d", toolID, timestamp.UnixNano()),
		AppID:         1, // Default to admin app for tool calls
		LLMID:         nil,
		CredentialID:  nil,
		Endpoint:      fmt.Sprintf("/tools/%s", name),
		Method:        "POST",
		StatusCode:    200,
		RequestTokens: 0,
		ResponseTokens: 0,
		TotalTokens:   0,
		Cost:          0,
		LatencyMs:     execTimeMs,
		ErrorMessage:  "",
		CreatedAt:     timestamp,
	}

	if err := h.db.Create(event).Error; err != nil {
		log.Error().Err(err).Msg("Failed to record tool analytics event")
	}
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
	log.Info().Msg("Setting microgateway analytics handler as global handler")
	analytics.SetHandler(h)
}

// TokenUsage represents parsed token usage from response (includes cache tokens)
type TokenUsage struct {
	PromptTokens      int
	ResponseTokens    int
	TotalTokens       int
	CacheWriteTokens  int // For prompt caching features
	CacheReadTokens   int // For prompt caching features
}

// parseTokensFromResponse extracts token usage from response body JSON
func (h *MicrogatewaAnalyticsHandler) parseTokensFromResponse(responseBody string) TokenUsage {
	if responseBody == "" {
		return TokenUsage{}
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		log.Debug().Err(err).Msg("Failed to parse response body for token extraction")
		return TokenUsage{}
	}

	// Try to extract usage information (OpenAI/Anthropic format)
	if usage, ok := response["usage"].(map[string]interface{}); ok {
		tokens := TokenUsage{}
		
		if pt, ok := usage["input_tokens"].(float64); ok {
			tokens.PromptTokens = int(pt)
		} else if pt, ok := usage["prompt_tokens"].(float64); ok {
			tokens.PromptTokens = int(pt)
		}
		
		if rt, ok := usage["output_tokens"].(float64); ok {
			tokens.ResponseTokens = int(rt)
		} else if rt, ok := usage["completion_tokens"].(float64); ok {
			tokens.ResponseTokens = int(rt)
		}
		
		if tt, ok := usage["total_tokens"].(float64); ok {
			tokens.TotalTokens = int(tt)
		} else {
			tokens.TotalTokens = tokens.PromptTokens + tokens.ResponseTokens
		}

		// Parse cache token usage (for prompt caching)
		if cwt, ok := usage["cache_creation_input_tokens"].(float64); ok {
			tokens.CacheWriteTokens = int(cwt)
		}
		if crt, ok := usage["cache_read_input_tokens"].(float64); ok {
			tokens.CacheReadTokens = int(crt)
		}
		
		return tokens
	}

	return TokenUsage{}
}

// parseCostFromResponse calculates cost using actual database pricing or defaults
func (h *MicrogatewaAnalyticsHandler) parseCostFromResponse(responseBody, vendor, model string) float64 {
	tokens := h.parseTokensFromResponse(responseBody)
	
	// Try to get actual pricing from database
	var price database.ModelPrice
	err := h.db.Where("model_name = ? AND vendor = ?", model, vendor).
		Order("created_at DESC").
		First(&price).Error
	
	if err != nil {
		// Use default pricing if not found (per-token rates)
		const defaultCPIT = 3.0 / 1000000   // $3.00 per million input tokens
		const defaultCPT = 15.0 / 1000000   // $15.00 per million output tokens
		
		promptCost := float64(tokens.PromptTokens) * defaultCPIT
		responseCost := float64(tokens.ResponseTokens) * defaultCPT
		
		return promptCost + responseCost
	}
	
	// Use actual database pricing (stored as per-token rates)
	promptCost := float64(tokens.PromptTokens) * price.CPIT
	responseCost := float64(tokens.ResponseTokens) * price.CPT
	cacheWriteCost := float64(tokens.CacheWriteTokens) * price.CacheWritePT
	cacheReadCost := float64(tokens.CacheReadTokens) * price.CacheReadPT
	
	return promptCost + responseCost + cacheWriteCost + cacheReadCost
}

// extractModelFromRequest parses the model name from request JSON
func (h *MicrogatewaAnalyticsHandler) extractModelFromRequest(requestBody string) string {
	if requestBody == "" {
		return "unknown"
	}

	var request map[string]interface{}
	if err := json.Unmarshal([]byte(requestBody), &request); err != nil {
		log.Debug().Err(err).Msg("Failed to parse request body for model extraction")
		return "unknown"
	}

	if model, ok := request["model"].(string); ok {
		return model
	}

	return "unknown"
}

// extractEndpointFromVendor creates the actual endpoint path by looking up LLM configuration
func (h *MicrogatewaAnalyticsHandler) extractEndpointFromVendor(vendor string, appID uint) string {
	// Try to find the LLM being used by looking up active LLMs for this vendor
	var llm database.LLM
	err := h.db.Where("vendor = ? AND is_active = ?", vendor, true).
		First(&llm).Error
	
	if err != nil {
		log.Debug().Err(err).Str("vendor", vendor).Msg("Could not find LLM for vendor, using generic endpoint")
		return fmt.Sprintf("/llm/rest/%s-model/v1/messages", vendor)
	}
	
	// Use the actual LLM slug for the endpoint
	// Different vendors have different API paths
	switch vendor {
	case "anthropic":
		return fmt.Sprintf("/llm/rest/%s/v1/messages", llm.Slug)
	case "openai":
		return fmt.Sprintf("/llm/rest/%s/v1/chat/completions", llm.Slug)
	case "google", "vertex":
		return fmt.Sprintf("/llm/rest/%s/v1/chat/completions", llm.Slug)
	default:
		return fmt.Sprintf("/llm/rest/%s/chat/completions", llm.Slug)
	}
}

// findLLMIDByVendorAndModel finds the LLM ID based on vendor and model name
func (h *MicrogatewaAnalyticsHandler) findLLMIDByVendorAndModel(vendor, modelName string) uint {
	var llm database.LLM

	// Try to find LLM by vendor and default model
	err := h.db.Where("vendor = ? AND default_model = ?", vendor, modelName).
		First(&llm).Error

	if err == nil {
		return llm.ID
	}

	// Fallback: find LLM by vendor only (first match)
	err = h.db.Where("vendor = ? AND is_active = ?", vendor, true).
		First(&llm).Error

	if err == nil {
		log.Debug().
			Str("vendor", vendor).
			Str("model", modelName).
			Uint("llm_id", llm.ID).
			Str("llm_name", llm.Name).
			Msg("Found LLM by vendor (model not matched)")
		return llm.ID
	}

	log.Debug().
		Str("vendor", vendor).
		Str("model", modelName).
		Msg("Could not find LLM for vendor and model")
	return 0
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}