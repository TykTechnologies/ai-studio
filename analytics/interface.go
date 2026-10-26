package analytics

import (
	"context"
	"errors"
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

// MCPCallRecorder is implemented by handlers that record MCP calls a Tyk
// Gateway proxied (ingested from Tyk Pump). It is optional, so a host's own
// AnalyticsHandler keeps compiling; a handler without it does not receive
// MCP calls. Unlike the AnalyticsHandler methods it reports a batch it
// could not take (ErrRecorderBusy), because its caller can have the sender
// retry.
type MCPCallRecorder interface {
	RecordMCPCalls(ctx context.Context, records []models.MCPCallRecord) error
}

// ErrRecorderBusy is returned by RecordMCPCalls when the batch was not
// queued: the buffer is full or the recorder has stopped.
var ErrRecorderBusy = errors.New("analytics recorder busy")

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

// handlerKey is the context key WithHandler stores a handler under.
type handlerKey struct{}

// WithHandler returns a copy of ctx whose records go to h rather than to the
// global handler: the Record functions that take a context (RecordExchange,
// RecordProxyLog, RecordChatRecord, ...) look for it first. Values survive
// context.WithoutCancel, so analysis detached from a request still finds it.
// Tests use it to give each test its own recorder, so tests that record
// analytics can run in parallel; production installs the global handler.
func WithHandler(ctx context.Context, h AnalyticsHandler) context.Context {
	return context.WithValue(ctx, handlerKey{}, h)
}

// BindHandler returns ctx with the handler its records go to now fixed on it:
// the one WithHandler put there, else the current global handler. Records
// made on ctx (or a context derived from it) later go to that handler even if
// the global one has been replaced in between. The proxy binds every request
// it serves, so analysis that finishes after the response never lands in a
// handler installed after the request arrived (a test's, when tests swap the
// global handler). With neither, ctx is returned unchanged and records
// resolve the global handler when they are made, as before.
func BindHandler(ctx context.Context) context.Context {
	if h, ok := ctx.Value(handlerKey{}).(AnalyticsHandler); ok && h != nil {
		return ctx
	}
	if h := GetHandler(); h != nil {
		return WithHandler(ctx, h)
	}
	return ctx
}

// handlerFor returns the handler for ctx's records (WithHandler's, else the
// global one) and the function to call once the record has been handed over:
// the global handler is read under handlerMu, held until then.
func handlerFor(ctx context.Context) (AnalyticsHandler, func()) {
	if ctx != nil {
		if h, ok := ctx.Value(handlerKey{}).(AnalyticsHandler); ok && h != nil {
			return h, func() {}
		}
	}
	handlerMu.RLock()
	return globalHandler, handlerMu.RUnlock
}

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

// ResetHandler resets the global analytics handler (useful for testing).
// When the handler is a stopped database recorder, it waits (briefly) for
// the recorder's worker to finish, so it does not outlive the Studio that
// stopped it: a Studio started next in the process replaces the package
// state (the logger) the worker still uses.
func ResetHandler() {
	handlerMu.Lock()
	h := globalHandler
	globalHandler = nil
	handlerMu.Unlock()

	if w, ok := h.(interface{ waitStopped(time.Duration) }); ok {
		w.waitStopped(5 * time.Second)
	}
}
