package analytics

import (
	"context"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
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
			logger.Warnf("ANALYTICS_BUFFER_SIZE must be a string, error: %s", sanitizeError(err))
		} else {
			defaultBufferSize = bfr
		}
	}

	logger.Infof("Starting database analytics handler with buffer_size: %d", defaultBufferSize)

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
				logger.Log.Warn().
					Str("error", sanitizeError(err)).
					Str("model", record.Name).
					Time("timestamp", record.TimeStamp).
					Msg("Error creating chat record")
			} else {
				logger.Log.Debug().
					Str("model", record.Name).
					Uint("app_id", record.AppID).
					Uint("llm_id", record.LLMID).
					Float64("cost_raw", record.Cost).
					Float64("cost_adjusted", record.Cost/10000).
					Time("timestamp", record.TimeStamp).
					Msg("Created chat record")
			}
		case logEntry := <-h.logEntryChan:
			err := h.createRecordWithRetry(func() error {
				return h.db.Create(logEntry).Error
			})
			if err != nil {
				logger.Warnf("Error creating chat log entry: %s", sanitizeError(err))
			}
		case toolCall := <-h.toolCallChan:
			err := h.createRecordWithRetry(func() error {
				return h.db.Create(toolCall).Error
			})
			if err != nil {
				logger.Warnf("Error creating tool call record: %s", sanitizeError(err))
			}
		case proxyLog := <-h.proxyLogChan:
			err := h.createRecordWithRetry(func() error {
				return h.db.Create(proxyLog).Error
			})
			if err != nil {
				logger.Warnf("Error creating proxy log: %s", sanitizeError(err))
			}
		case records := <-h.chatRecordBatchChan:
			startTime := time.Now()
			err := h.createRecordWithRetry(func() error {
				return h.db.CreateInBatches(records, 100).Error
			})
			processingTime := time.Since(startTime)

			if err != nil {
				logger.Log.Warn().
					Str("error", sanitizeError(err)).
					Int("count", len(records)).
					Int64("processing_time_ms", processingTime.Milliseconds()).
					Msg("Error creating chat record batch")
			} else {
				logger.Log.Debug().
					Int("count", len(records)).
					Int64("processing_time_ms", processingTime.Milliseconds()).
					Float64("records_per_second", float64(len(records))/processingTime.Seconds()).
					Str("first_model", records[0].Name).
					Time("timestamp", records[0].TimeStamp).
					Msg("Created chat record batch")
			}
		case logs := <-h.proxyLogBatchChan:
			startTime := time.Now()
			err := h.createRecordWithRetry(func() error {
				return h.db.CreateInBatches(logs, 100).Error
			})
			processingTime := time.Since(startTime)

			if err != nil {
				logger.Log.Warn().
					Str("error", sanitizeError(err)).
					Int("count", len(logs)).
					Int64("processing_time_ms", processingTime.Milliseconds()).
					Msg("Error creating proxy log batch")
			} else {
				logger.Log.Debug().
					Int("count", len(logs)).
					Int64("processing_time_ms", processingTime.Milliseconds()).
					Float64("records_per_second", float64(len(logs))/processingTime.Seconds()).
					Str("first_vendor", logs[0].Vendor).
					Time("timestamp", logs[0].TimeStamp).
					Msg("Created proxy log batch")
			}
		case <-h.ctx.Done():
			logger.Info("shutting down database analytics handler")
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

				logger.Debugf("Database locked, retrying database operation (attempt %d/%d, delay %dms)",
					attempt+1, maxRetries, totalDelay.Milliseconds())

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
		logger.Warnf("Analytics recording not started, dropping chat record: model=%s, app_id=%d, llm_id=%d, cost=%.2f", record.Name, record.AppID, record.LLMID, record.Cost)
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	select {
	case <-h.ctx.Done():
		logger.Warnf("Dropping chat record due to cancellation: %s", h.ctx.Err())
		return
	case h.chatRecordChan <- record:
		logger.Debugf("Sent chat record to channel: model=%s, app_id=%d, llm_id=%d, cost=%.2f", record.Name, record.AppID, record.LLMID, record.Cost)
	default:
		logger.Warnf("Chat record buffer full, dropping record: model=%s, app_id=%d, llm_id=%d", record.Name, record.AppID, record.LLMID)
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
	case <-h.ctx.Done():
		logger.Warnf("dropping proxy log record due to cancellation: %s", h.ctx.Err())
		return
	case h.proxyLogChan <- log:
	default:
		logger.Warn("proxy log buffer full, dropping log")
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
	case <-h.ctx.Done():
		logger.Warnf("dropping tool call record due to cancellation: %s", h.ctx.Err())
		return
	case h.toolCallChan <- tcEntry:
	default:
		logger.Warn("tool call buffer full, dropping tool call")
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
	case <-h.ctx.Done():
		logger.Warnf("dropping chat log record due to cancellation: %s", h.ctx.Err())
		return
	case h.logEntryChan <- logEntry:
	default:
		logger.Warn("chat log buffer full, dropping log")
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
		logger.Warnf("Analytics recording not started, dropping batch of chat records, count: %d", len(records))
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	// Send batch to async worker - non-blocking to avoid request latency
	select {
	case <-h.ctx.Done():
		logger.Warnf("Dropping chat record batch due to cancellation: %s", h.ctx.Err())
		return
	case h.chatRecordBatchChan <- records:
		logger.Debugf("Sent chat record batch to async worker, count: %d", len(records))
	default:
		logger.Warnf("Chat record batch buffer full, dropping batch, count: %d", len(records))
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
		logger.Warnf("Analytics recording not started, dropping batch of proxy logs, count: %d", len(logs))
		h.recMutex.RUnlock()
		return
	}
	h.recMutex.RUnlock()

	// Send batch to async worker - non-blocking to avoid request latency
	select {
	case <-h.ctx.Done():
		logger.Warnf("Dropping proxy log batch due to cancellation: %s", h.ctx.Err())
		return
	case h.proxyLogBatchChan <- logs:
		logger.Debugf("Sent proxy log batch to async worker, count: %d", len(logs))
	default:
		logger.Warnf("Proxy log batch buffer full, dropping batch, count: %d", len(logs))
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
		logger.Warnf("Error migrating analytics tables: %s", sanitizeError(err))
	}
}
