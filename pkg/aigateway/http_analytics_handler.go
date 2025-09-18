package aigateway

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// HTTPAnalyticsHandler sends analytics to an HTTP endpoint
type HTTPAnalyticsHandler struct {
	endpoint string
	client   *http.Client
}

// NewHTTPAnalyticsHandler creates a new HTTP analytics handler
func NewHTTPAnalyticsHandler(endpoint string) *HTTPAnalyticsHandler {
	return &HTTPAnalyticsHandler{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// RecordChatRecord implements analytics.AnalyticsHandler
func (h *HTTPAnalyticsHandler) RecordChatRecord(record *models.LLMChatRecord) {
	data := map[string]interface{}{
		"type":                      "llm_usage",
		"llm_id":                    record.LLMID,
		"model":                     record.Name,
		"vendor":                    record.Vendor,
		"prompt_tokens":             record.PromptTokens,
		"response_tokens":           record.ResponseTokens,
		"cache_write_prompt_tokens": record.CacheWritePromptTokens,
		"cache_read_prompt_tokens":  record.CacheReadPromptTokens,
		"total_tokens":              record.TotalTokens,
		"cost":                      record.Cost,
		"currency":                  record.Currency,
		"timestamp":                 record.TimeStamp,
		"app_id":                    record.AppID,
		"user_id":                   record.UserID,
		"interaction_type":          record.InteractionType,
		"choices":                   record.Choices,
		"tool_calls":                record.ToolCalls,
	}
	h.postJSON("/analytics", data)
}

// RecordChatLogEntry implements analytics.AnalyticsHandler
func (h *HTTPAnalyticsHandler) RecordChatLogEntry(log *models.LLMChatLogEntry) {
	data := map[string]interface{}{
		"type":      "chat_log_entry",
		"name":      log.Name,
		"vendor":    log.Vendor,
		"prompt":    log.Prompt,
		"response":  log.Response,
		"tokens":    log.Tokens,
		"timestamp": log.TimeStamp,
		"user_id":   log.UserID,
		"chat_id":   log.ChatID,
	}
	h.postJSON("/analytics", data)
}

// RecordProxyLog implements analytics.AnalyticsHandler
func (h *HTTPAnalyticsHandler) RecordProxyLog(log *models.ProxyLog) {
	data := map[string]interface{}{
		"type":          "proxy_log",
		"app_id":        log.AppID,
		"user_id":       log.UserID,
		"vendor":        log.Vendor,
		"status_code":   log.ResponseCode,
		"timestamp":     log.TimeStamp,
		"request_body":  log.RequestBody,  // Already truncated by proxy
		"response_body": log.ResponseBody, // Already truncated by proxy
	}
	h.postJSON("/analytics", data)
}

// RecordToolCall implements analytics.AnalyticsHandler
func (h *HTTPAnalyticsHandler) RecordToolCall(name string, timestamp time.Time, execTime int, toolID uint) {
	data := map[string]interface{}{
		"type":      "tool_call",
		"tool_id":   toolID,
		"name":      name,
		"exec_time": execTime,
		"timestamp": timestamp,
	}
	h.postJSON("/analytics", data)
}

// postJSON sends a JSON POST request to the specified path
func (h *HTTPAnalyticsHandler) postJSON(path string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		slog.Warn("failed to marshal analytics data", "error", err)
		return
	}

	resp, err := h.client.Post(h.endpoint+path, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Warn("failed to send analytics data", "error", err, "endpoint", h.endpoint+path)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("analytics endpoint returned error", "status", resp.StatusCode, "endpoint", h.endpoint+path)
	}
}

// RecordChatRecordsBatch implements batch recording for HTTP analytics
func (h *HTTPAnalyticsHandler) RecordChatRecordsBatch(records []*models.LLMChatRecord) {
	// For HTTP handler, we can send individual records or batch them in a single request
	// For simplicity, we'll batch them into a single HTTP request
	if len(records) == 0 {
		return
	}

	batch := make([]map[string]interface{}, len(records))
	for i, record := range records {
		batch[i] = map[string]interface{}{
			"type":                      "llm_usage",
			"llm_id":                    record.LLMID,
			"model":                     record.Name,
			"vendor":                    record.Vendor,
			"prompt_tokens":             record.PromptTokens,
			"response_tokens":           record.ResponseTokens,
			"cache_write_prompt_tokens": record.CacheWritePromptTokens,
			"cache_read_prompt_tokens":  record.CacheReadPromptTokens,
			"total_tokens":              record.TotalTokens,
			"cost":                      record.Cost,
			"currency":                  record.Currency,
			"timestamp":                 record.TimeStamp,
			"app_id":                    record.AppID,
			"user_id":                   record.UserID,
			"interaction_type":          record.InteractionType,
			"choices":                   record.Choices,
			"tool_calls":                record.ToolCalls,
		}
	}

	batchData := map[string]interface{}{
		"type": "llm_usage_batch",
		"records": batch,
	}
	h.postJSON("/analytics/batch", batchData)
}

// RecordProxyLogsBatch implements batch recording for HTTP analytics
func (h *HTTPAnalyticsHandler) RecordProxyLogsBatch(logs []*models.ProxyLog) {
	// For HTTP handler, we can send individual records or batch them in a single request
	if len(logs) == 0 {
		return
	}

	batch := make([]map[string]interface{}, len(logs))
	for i, log := range logs {
		batch[i] = map[string]interface{}{
			"type":          "proxy_log",
			"app_id":        log.AppID,
			"user_id":       log.UserID,
			"vendor":        log.Vendor,
			"status_code":   log.ResponseCode,
			"timestamp":     log.TimeStamp,
			"request_body":  log.RequestBody,  // Already truncated by proxy
			"response_body": log.ResponseBody, // Already truncated by proxy
		}
	}

	batchData := map[string]interface{}{
		"type": "proxy_log_batch",
		"records": batch,
	}
	h.postJSON("/analytics/batch", batchData)
}

// SetAsGlobalHandler sets this handler as the global analytics handler
func (h *HTTPAnalyticsHandler) SetAsGlobalHandler() {
	analytics.SetHandler(h)
}
