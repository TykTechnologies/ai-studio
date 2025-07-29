package analytics

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strconv"
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
	recStarted     bool
	recMutex       sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
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
			slog.Warn("ANALYTICS_BUFFER_SIZE must be a string", "error", err)
		} else {
			defaultBufferSize = bfr
		}
	}

	slog.Info("starting database analytics handler", "buffer_size", defaultBufferSize)

	h.chatRecordChan = make(chan *models.LLMChatRecord, defaultBufferSize)
	h.logEntryChan = make(chan *models.LLMChatLogEntry, defaultBufferSize)
	h.toolCallChan = make(chan *models.ToolCallRecord, defaultBufferSize)
	h.proxyLogChan = make(chan *models.ProxyLog, defaultBufferSize)

	// Start background workers
	go h.startWorker()
}

// startWorker runs the main worker loop for handling database writes
func (h *DatabaseHandler) startWorker() {
	for {
		select {
		case record := <-h.chatRecordChan:
			err := h.db.Create(record).Error
			if err != nil {
				slog.Warn("error creating chat record", "error", err, "model", record.Name, "timestamp", record.TimeStamp)
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
			err := h.db.Create(logEntry).Error
			if err != nil {
				slog.Warn("error creating chat log entry", "error", err)
			}
		case toolCall := <-h.toolCallChan:
			err := h.db.Create(toolCall).Error
			if err != nil {
				slog.Warn("error creating tool call record", "error", err)
			}
		case proxyLog := <-h.proxyLogChan:
			err := h.db.Create(proxyLog).Error
			if err != nil {
				slog.Warn("error creating proxy log", "error", err)
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
			return
		}
	}
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
		slog.Warn("error migrating analytics tables", "error", err)
	}
}
