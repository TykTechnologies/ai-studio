package analytics

import (
	"context"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// AnalyticsHandler defines the interface for analytics implementations
type AnalyticsHandler interface {
	// RecordChatRecord records LLM chat/proxy usage
	RecordChatRecord(ctx context.Context, record *models.LLMChatRecord)

	// RecordChatLogEntry records detailed chat log entries
	RecordChatLogEntry(ctx context.Context, log *models.LLMChatLogEntry)

	// RecordProxyLog records proxy request/response logs
	RecordProxyLog(ctx context.Context, log *models.ProxyLog)

	// RecordToolCall records tool call execution
	RecordToolCall(ctx context.Context, name string, timestamp time.Time, execTime int, toolID uint)

	// SetAsGlobalHandler sets this handler as the global analytics handler
	SetAsGlobalHandler()

	// Batch processing methods for improved performance
	RecordChatRecordsBatch(ctx context.Context, records []*models.LLMChatRecord)
	RecordProxyLogsBatch(ctx context.Context, logs []*models.ProxyLog)

	// RecordComplianceEvents records filter script compliance events
	RecordComplianceEvents(ctx context.Context, events []*models.ComplianceEvent)
}

// ExchangeRecorder is implemented by handlers that record a proxied request's
// proxy log and its chat record together. The gateway produces both for one
// request; a handler that stores them as one row needs them in one call
// rather than having to pair two separate calls up again.
type ExchangeRecorder interface {
	// RecordExchange records one proxied request. rec is nil when the
	// response yielded no usage (an error, or nothing to parse).
	RecordExchange(ctx context.Context, log *models.ProxyLog, rec *models.LLMChatRecord)
}

var (
	globalHandler AnalyticsHandler
	// Synchronizes access to the globalHandler variable.
	// This prevents a data race that occurs when tests call ResetHandler
	// while other goroutines are reading the variable.
	handlerMu sync.RWMutex
)

// SetHandler sets the global analytics handler implementation
func SetHandler(handler AnalyticsHandler) {
	handlerMu.Lock()
	defer handlerMu.Unlock()

	globalHandler = handler
}

// GetHandler returns the current analytics handler (useful for testing)
func GetHandler() AnalyticsHandler {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	return globalHandler
}

// ResetHandler resets the global analytics handler (useful for testing)
func ResetHandler() {
	handlerMu.Lock()
	defer handlerMu.Unlock()

	globalHandler = nil
}
