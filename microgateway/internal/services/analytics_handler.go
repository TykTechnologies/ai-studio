// internal/services/analytics_handler.go
package services

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MicrogatewaAnalyticsHandler implements the midsommar analytics interface
// and converts analytics data to microgateway's analytics_events format
type MicrogatewaAnalyticsHandler struct {
	db *gorm.DB
}

// NewMicrogatewaAnalyticsHandler creates a new analytics handler for the microgateway
func NewMicrogatewaAnalyticsHandler(db *gorm.DB) *MicrogatewaAnalyticsHandler {
	return &MicrogatewaAnalyticsHandler{
		db: db,
	}
}

// RecordChatRecord implements the midsommar analytics interface
// Converts LLMChatRecord to microgateway AnalyticsEvent
func (h *MicrogatewaAnalyticsHandler) RecordChatRecord(record *models.LLMChatRecord) {
	log.Debug().
		Uint("app_id", record.AppID).
		Uint("llm_id", record.LLMID).
		Str("model", record.Name).
		Int("total_tokens", record.TotalTokens).
		Float64("cost", record.Cost).
		Msg("Recording analytics from LLMChatRecord")

	event := &database.AnalyticsEvent{
		RequestID:      fmt.Sprintf("req_%d_%d", record.AppID, time.Now().UnixNano()),
		AppID:          record.AppID,
		LLMID:          &record.LLMID,
		CredentialID:   nil, // We don't use credentials in token-only system
		Endpoint:       fmt.Sprintf("/llm/rest/%s/chat/completions", record.Name),
		Method:         "POST",
		StatusCode:     200, // Assume success if we got analytics
		RequestTokens:  record.PromptTokens,
		ResponseTokens: record.ResponseTokens,
		TotalTokens:    record.TotalTokens,
		Cost:           record.Cost / 10000, // Convert from cents to dollars
		LatencyMs:      record.TotalTimeMS,
		ErrorMessage:   "",
		CreatedAt:      record.TimeStamp,
	}

	if err := h.db.Create(event).Error; err != nil {
		log.Error().Err(err).Msg("Failed to record analytics event")
	} else {
		log.Debug().
			Uint("event_id", event.ID).
			Str("request_id", event.RequestID).
			Msg("Analytics event recorded successfully")
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
// Records proxy-level request/response information
func (h *MicrogatewaAnalyticsHandler) RecordProxyLog(proxyLog *models.ProxyLog) {
	log.Debug().
		Uint("app_id", proxyLog.AppID).
		Uint("user_id", proxyLog.UserID).
		Str("vendor", proxyLog.Vendor).
		Int("response_code", proxyLog.ResponseCode).
		Msg("Recording proxy log")

	// We could create a separate proxy analytics event or enhance the existing analytics event
	// For now, we'll rely on the RecordChatRecord for the main analytics
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

// SetAsGlobalHandler sets this handler as the global midsommar analytics handler
func (h *MicrogatewaAnalyticsHandler) SetAsGlobalHandler() {
	log.Info().Msg("Setting microgateway analytics handler as global handler")
	analytics.SetHandler(h)
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}