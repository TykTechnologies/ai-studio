package analytics

import (
	"context"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// DatabaseHandler implements AnalyticsHandler using the existing database/channel system
type DatabaseHandler struct {
	db             *gorm.DB
	chatRecordChan chan *models.LLMChatRecord
	logEntryChan   chan *models.LLMChatLogEntry
	toolCallChan   chan *models.ToolCallRecord
	proxyLogChan   chan *models.ProxyLog
	// Batch processing channels for async non-blocking batch operations
	chatRecordBatchChan chan []*models.LLMChatRecord
	proxyLogBatchChan   chan []*models.ProxyLog
	recStarted          bool
	recMutex            sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
}

// Security: Pattern to detect sensitive data in error messages that should be redacted
var sensitiveDataPattern = regexp.MustCompile(`(?i)(token|key|secret|password|credential|authorization|bearer|api_key|auth)\s*[:=]\s*['"]?([^\s'",]+)['"]?`)

// sanitizeError removes potentially sensitive data from error messages for safe logging
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Redact sensitive data patterns
	sanitized := sensitiveDataPattern.ReplaceAllStringFunc(errStr, func(match string) string {
		// Extract the key part (first capture group)
		parts := sensitiveDataPattern.FindStringSubmatch(match)
		if len(parts) >= 2 {
			key := parts[1]
			// Determine the separator used
			if strings.Contains(match, ":") {
				return key + ": ***REDACTED***"
			}
			return key + "=***REDACTED***"
		}
		return "***REDACTED***"
	})

	return sanitized
}

// NewDatabaseHandler creates a new database analytics handler
func NewDatabaseHandler(ctx context.Context, db *gorm.DB) *DatabaseHandler {
	ctx, cancel := context.WithCancel(ctx)

	h := &DatabaseHandler{
		db:     db,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize and start the handler
	h.start()

	return h
}

// start initializes channels and starts background workers
func (h *DatabaseHandler) start() {
	h.recMutex.Lock()
	if h.recStarted {
		h.recMutex.Unlock()
		return
	}
	h.recStarted = true
	h.recMutex.Unlock()

	initDB(h.db)

	defaultBufferSize := 1000
	analyticsBufferSizeStr := os.Getenv("ANALYTICS_BUFFER_SIZE")
	if analyticsBufferSizeStr != "" {
		bfr, err := strconv.Atoi(analyticsBufferSizeStr)
		if err != nil {
			slog.Warn("ANALYTICS_BUFFER_SIZE must be a string", "error", sanitizeError(err))
		} else {
			defaultBufferSize = bfr
		}
	}

	slog.Info("starting database analytics handler", "buffer_size", defaultBufferSize)

	h.chatRecordChan = make(chan *models.LLMChatRecord, defaultBufferSize)
	h.logEntryChan = make(chan *models.LLMChatLogEntry, defaultBufferSize)
	h.toolCallChan = make(chan *models.ToolCallRecord, defaultBufferSize)
	h.proxyLogChan = make(chan *models.ProxyLog, defaultBufferSize)

	// Initialize batch processing channels with smaller buffer for batches
	batchBufferSize := defaultBufferSize / 10 // Batches are larger, so fewer slots needed
	if batchBufferSize < 10 {
		batchBufferSize = 10
	}
	h.chatRecordBatchChan = make(chan []*models.LLMChatRecord, batchBufferSize)
	h.proxyLogBatchChan = make(chan []*models.ProxyLog, batchBufferSize)

	// Start background workers
	go h.startWorker()
}

// startWorker runs the main worker loop for handling database writes
func (h *DatabaseHandler) startWorker() {
	for {
		select {
		case record := <-h.chatRecordChan:
			err := h.createRecordWithRetry(func() error {
				return h.db.Create(record).Error
			})
			if err != nil {
				slog.Warn("error creating chat record", "error", sanitizeError(err), "model", record.Name, "timestamp", record.TimeStamp)
			} else {
				slog.Info("created chat record",
					"model", record.Name,
					"app_id", record.AppID,
					"llm_id", record.LLMID,
					"cost_raw", record.Cost, // Raw value (e.g., 250000.0)
					"cost_adjusted", record.Cost/10000, // Human-readable (e.g., 25.0)
					"timestamp", record.TimeStamp)
			}
		case logEntry := <-h.logEntryChan:
			err := h.createRecordWithRetry(func() error {
				return h.db.Create(logEntry).Error
			})
			if err != nil {
				slog.Warn("error creating chat log entry", "error", sanitizeError(err))
			}
		case toolCall := <-h.toolCallChan:
			err := h.createRecordWithRetry(func() error {
				return h.db.Create(toolCall).Error
			})
			if err != nil {
				slog.Warn("error creating tool call record", "error", sanitizeError(err))
			}
		case proxyLog := <-h.proxyLogChan:
			err := h.createRecordWithRetry(func() error {
				return h.db.Create(proxyLog).Error
			})
			if err != nil {
				slog.Warn("error creating proxy log", "error", sanitizeError(err))
			}
		case records := <-h.chatRecordBatchChan:
			startTime := time.Now()
			err := h.createRecordWithRetry(func() error {
				return h.db.CreateInBatches(records, 100).Error
			})
			processingTime := time.Since(startTime)

			if err != nil {
				slog.Warn("error creating chat record batch",
					"error", sanitizeError(err),
					"count", len(records),
					"processing_time_ms", processingTime.Milliseconds())
			} else {
				slog.Info("created chat record batch",
					"count", len(records),
					"processing_time_ms", processingTime.Milliseconds(),
					"records_per_second", float64(len(records))/processingTime.Seconds(),
					"first_model", records[0].Name,
					"timestamp", records[0].TimeStamp)
			}
		case logs := <-h.proxyLogBatchChan:
			startTime := time.Now()
			err := h.createRecordWithRetry(func() error {
				return h.db.CreateInBatches(logs, 100).Error
			})
			processingTime := time.Since(startTime)

			if err != nil {
				slog.Warn("error creating proxy log batch",
					"error", sanitizeError(err),
					"count", len(logs),
					"processing_time_ms", processingTime.Milliseconds())
			} else {
				slog.Info("created proxy log batch",
					"count", len(logs),
					"processing_time_ms", processingTime.Milliseconds(),
					"records_per_second", float64(len(logs))/processingTime.Seconds(),
					"first_vendor", logs[0].Vendor,
					"timestamp", logs[0].TimeStamp)
			}
		case <-h.ctx.Done():
			slog.Info("shutting down database analytics handler")
			h.recMutex.Lock()
			h.recStarted = false
			h.recMutex.Unlock()
			close(h.chatRecordChan)
			close(h.logEntryChan)
			close(h.toolCallChan)
			close(h.proxyLogChan)
			close(h.chatRecordBatchChan)
			close(h.proxyLogBatchChan)
			return
		}
	}
}

// createRecordWithRetry executes database operations with retry logic for lock errors
func (h *DatabaseHandler) createRecordWithRetry(createFn func() error) error {
	maxRetries := 5
	baseDelay := 50 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := createFn()
		if err == nil {
			return nil
		}

		// Check if this is a database lock error
		if strings.Contains(err.Error(), "database table is locked") ||
			strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "SQLITE_BUSY") {

			if attempt < maxRetries-1 {
				// Exponential backoff with jitter
				delay := baseDelay * time.Duration(1<<attempt)
				// Add some jitter to prevent thundering herd
				jitter := time.Duration(rand.Intn(50)) * time.Millisecond
				totalDelay := delay + jitter

				slog.Debug("database locked, retrying database operation",
					"attempt", attempt+1,
					"max_retries", maxRetries,
					"delay_ms", totalDelay.Milliseconds())

				select {
				case <-time.After(totalDelay):
					continue
				case <-h.ctx.Done():
					return h.ctx.Err()
				}
			}
		}

		// For non-lock errors or final attempt, return the error
		return err
	}

	return nil
}

// Stop gracefully stops the database handler
func (h *DatabaseHandler) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
}

// Implement AnalyticsHandler interface methods
func (h *DatabaseHandler) RecordChatRecord(record *models.LLMChatRecord) {
	h.recMutex.RLock()
	if !h.recStarted {
		log.Printf("Analytics recording not started, dropping chat record: model=%s, app_id=%d, llm_id=%d, cost=%.2f", record.Name, record.AppID, record.LLMID, record.Cost)
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	select {
	case h.chatRecordChan <- record:
		log.Printf("Sent chat record to channel: model=%s, app_id=%d, llm_id=%d, cost=%.2f", record.Name, record.AppID, record.LLMID, record.Cost)
	default:
		log.Printf("Chat record buffer full, dropping record: model=%s, app_id=%d, llm_id=%d", record.Name, record.AppID, record.LLMID)
	}
}

func (h *DatabaseHandler) RecordProxyLog(log *models.ProxyLog) {
	h.recMutex.RLock()
	if !h.recStarted {
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	select {
	case h.proxyLogChan <- log:
	default:
		slog.Warn("proxy log buffer full, dropping log")
	}
}

func (h *DatabaseHandler) RecordToolCall(name string, timestamp time.Time, execTime int, toolID uint) {
	h.recMutex.RLock()
	if !h.recStarted {
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	tcEntry := &models.ToolCallRecord{
		ToolID:    toolID,
		Name:      name,
		ExecTime:  execTime,
		TimeStamp: timestamp,
	}

	select {
	case h.toolCallChan <- tcEntry:
	default:
		slog.Warn("tool call buffer full, dropping tool call")
	}
}

// RecordChatLogEntry implements analytics.AnalyticsHandler
func (h *DatabaseHandler) RecordChatLogEntry(logEntry *models.LLMChatLogEntry) {
	h.recMutex.RLock()
	if !h.recStarted {
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	select {
	case h.logEntryChan <- logEntry:
	default:
		slog.Warn("chat log buffer full, dropping log")
	}
}

// RecordChatRecordsBatch records multiple chat records asynchronously using background worker
// This method is non-blocking and returns immediately to avoid impacting request latency
func (h *DatabaseHandler) RecordChatRecordsBatch(records []*models.LLMChatRecord) {
	if len(records) == 0 {
		return
	}

	h.recMutex.RLock()
	if !h.recStarted {
		slog.Warn("Analytics recording not started, dropping batch of chat records", "count", len(records))
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	// Send batch to async worker - non-blocking to avoid request latency
	select {
	case h.chatRecordBatchChan <- records:
		slog.Debug("sent chat record batch to async worker", "count", len(records))
	default:
		slog.Warn("chat record batch buffer full, dropping batch", "count", len(records))
	}
}

// RecordProxyLogsBatch records multiple proxy logs asynchronously using background worker
// This method is non-blocking and returns immediately to avoid impacting request latency
func (h *DatabaseHandler) RecordProxyLogsBatch(logs []*models.ProxyLog) {
	if len(logs) == 0 {
		return
	}

	h.recMutex.RLock()
	if !h.recStarted {
		slog.Warn("Analytics recording not started, dropping batch of proxy logs", "count", len(logs))
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	// Send batch to async worker - non-blocking to avoid request latency
	select {
	case h.proxyLogBatchChan <- logs:
		slog.Debug("sent proxy log batch to async worker", "count", len(logs))
	default:
		slog.Warn("proxy log batch buffer full, dropping batch", "count", len(logs))
	}
}

// SetAsGlobalHandler sets this handler as the global analytics handler
func (h *DatabaseHandler) SetAsGlobalHandler() {
	SetHandler(h)
}

// initDB handles database migration - moved from analytics.go
func initDB(db *gorm.DB) {
	err := db.AutoMigrate(
		&models.LLMChatRecord{},
		&models.LLMChatLogEntry{},
		&models.ToolCallRecord{},
		&models.ProxyLog{},
	)

	if err != nil {
		slog.Warn("error migrating analytics tables", "error", sanitizeError(err))
	}
}
