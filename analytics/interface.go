package analytics

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// AnalyticsHandler defines the interface for analytics implementations
type AnalyticsHandler interface {
	// RecordChatRecord records LLM chat/proxy usage
	RecordChatRecord(record *models.LLMChatRecord)

	// RecordChatLogEntry records detailed chat log entries
	RecordChatLogEntry(log *models.LLMChatLogEntry)

	// RecordProxyLog records proxy request/response logs
	RecordProxyLog(log *models.ProxyLog)

	// RecordToolCall records tool call execution
	RecordToolCall(name string, timestamp time.Time, execTime int, toolID uint)
}

var globalHandler AnalyticsHandler

// SetHandler sets the global analytics handler implementation
func SetHandler(handler AnalyticsHandler) {
	globalHandler = handler
}

// GetHandler returns the current analytics handler (useful for testing)
func GetHandler() AnalyticsHandler {
	return globalHandler
}

// ResetHandler resets the global analytics handler (useful for testing)
func ResetHandler() {
	globalHandler = nil
}
