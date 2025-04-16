package analytics

import (
	"context"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"
)

// Recorder defines the interface for recording analytics data
type Recorder interface {
	RecordProxyLog(log *models.ProxyLog)
	RecordChatRecord(record *models.LLMChatRecord)
	RecordToolCall(name string, t time.Time, execTime int, toolID uint)
	RecordContentMessage(mc *llms.MessageContent, cr *llms.ContentResponse, vendor models.Vendor,
		name, chatID string, timeMs int, userID, appID, llmID uint, t time.Time,
		svc services.ServiceInterface)
}

var (
	// defaultRecorder is the default implementation that writes to the database
	defaultRecorder Recorder

	// currentRecorder is the currently active recorder
	currentRecorder Recorder

	// recorderMutex protects access to the recorder
	recorderMutex sync.RWMutex

	// Internal channels used by the DatabaseRecorder
	chatRecordChan chan *models.LLMChatRecord
	logEntryChan   chan *models.LLMChatLogEntry
	toolCallChan   chan *models.ToolCallRecord
	proxyLogChan   chan *models.ProxyLog
	recStarted     bool
	recMutex       sync.RWMutex
)

// DatabaseRecorder implements Recorder by writing to a database
type DatabaseRecorder struct {
	db *gorm.DB
}

// RecordProxyLog implements Recorder.RecordProxyLog for DatabaseRecorder
func (r *DatabaseRecorder) RecordProxyLog(log *models.ProxyLog) {
	recMutex.RLock()
	if !recStarted {
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	proxyLogChan <- log
}

// RecordProxyLog delegates to the current recorder
func RecordProxyLog(log *models.ProxyLog) {
	recorderMutex.RLock()
	recorder := currentRecorder
	recorderMutex.RUnlock()

	if recorder != nil {
		recorder.RecordProxyLog(log)
	}
}

// RecordToolCall implements Recorder.RecordToolCall for DatabaseRecorder
func (r *DatabaseRecorder) RecordToolCall(name string, t time.Time, execTime int, toolID uint) {
	recMutex.RLock()
	if !recStarted {
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	tcEntry := &models.ToolCallRecord{}
	tcEntry.TimeStamp = t
	tcEntry.ExecTime = execTime
	tcEntry.Name = name
	tcEntry.ToolID = toolID

	recordToolCall(tcEntry)
}

// RecordToolCall delegates to the current recorder
func RecordToolCall(name string, t time.Time, execTime int, toolID uint) {
	recorderMutex.RLock()
	recorder := currentRecorder
	recorderMutex.RUnlock()

	if recorder != nil {
		recorder.RecordToolCall(name, t, execTime, toolID)
	}
}

// RecordContentMessage implements Recorder.RecordContentMessage for DatabaseRecorder
func (r *DatabaseRecorder) RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int, userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
) {
	recMutex.RLock()
	if !recStarted {
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	rec := &models.LLMChatRecord{}

	totalTokens := 0
	promptTokens := 0
	responseTokens := 0
	toolCalls := 0

	responseParts := []string{}
	for _, c := range cr.Choices {
		toolCalls += len(c.ToolCalls)
		tt, pt, rt := switches.GetTokenCounts(c, vendor)

		totalTokens += tt
		promptTokens += pt
		responseTokens += rt

		if c.Content != "" {
			responseParts = append(responseParts, c.Content)
		}
	}

	promptParts := []string{}
	for _, p := range mc.Parts {
		tp, ok := p.(llms.TextContent)
		if !ok {
			continue
		}

		if tp.Text != "" {
			promptParts = append(promptParts, tp.Text)
		}
	}

	prompt := strings.Join(promptParts, "\n")

	price, err := svc.GetModelPriceByModelNameAndVendor(name, string(vendor))
	cpt := 0.0
	cpit := 0.0
	if err == nil {
		cpt = price.CPT
		cpit = price.CPIT
	}

	if price == nil {
		price = &models.ModelPrice{
			CPT:       0.0,
			CPIT:      0.0,
			ModelName: name,
			Vendor:    string(vendor),
			Currency:  "USD",
		}
	}

	// Get cache token information from the response if available
	if len(cr.Choices) > 0 && cr.Choices[0].GenerationInfo != nil {
		// Log the keys in GenerationInfo for debugging
		slog.Info("GenerationInfo keys", "keys", cr.Choices[0].GenerationInfo)

		// Try int first, then float64
		if cacheWrite, ok := cr.Choices[0].GenerationInfo["CacheCreationInputTokens"].(int); ok {
			rec.CacheWritePromptTokens = cacheWrite
			slog.Info("Cache write tokens (int)", "value", rec.CacheWritePromptTokens)
		} else if cacheWrite, ok := cr.Choices[0].GenerationInfo["CacheCreationInputTokens"].(float64); ok {
			rec.CacheWritePromptTokens = int(cacheWrite)
			slog.Info("Cache write tokens (float64)", "value", rec.CacheWritePromptTokens)
		}

		if cacheRead, ok := cr.Choices[0].GenerationInfo["CacheReadInputTokens"].(int); ok {
			rec.CacheReadPromptTokens = cacheRead
			slog.Info("Cache read tokens (int)", "value", rec.CacheReadPromptTokens)
		} else if cacheRead, ok := cr.Choices[0].GenerationInfo["CacheReadInputTokens"].(float64); ok {
			rec.CacheReadPromptTokens = int(cacheRead)
			slog.Info("Cache read tokens (float64)", "value", rec.CacheReadPromptTokens)
		}
	}

	rec.Choices = len(cr.Choices)
	// Use provided model name if available, fallback to price model name
	if name != "" {
		rec.Name = name
	} else {
		rec.Name = price.ModelName
	}
	rec.Vendor = string(vendor)
	rec.TotalTimeMS = timeMs
	rec.PromptTokens = promptTokens
	rec.ResponseTokens = responseTokens
	rec.TotalTokens = promptTokens + responseTokens
	rec.TimeStamp = t
	rec.UserID = userID
	rec.ToolCalls = toolCalls
	rec.ChatID = chatID
	rec.AppID = appID
	// Calculate cost including cache tokens (store as cents with 4 decimal places)
	cost := cpt*float64(responseTokens) +
		cpit*float64(promptTokens) +
		price.CacheWritePT*float64(rec.CacheWritePromptTokens) +
		price.CacheReadPT*float64(rec.CacheReadPromptTokens)

	slog.Debug("Calculated cost before scaling",
		"cost", cost,
		"responseTokens", responseTokens,
		"promptTokens", promptTokens,
		"cacheWriteTokens", rec.CacheWritePromptTokens,
		"cacheReadTokens", rec.CacheReadPromptTokens)

	rec.Cost = cost * 10000
	rec.Currency = price.Currency
	rec.InteractionType = models.ChatInteraction
	rec.LLMID = llmID

	chatLog := &models.LLMChatLogEntry{}
	chatLog.Name = name
	chatLog.Vendor = string(vendor)
	chatLog.TimeStamp = t
	chatLog.Prompt = prompt
	chatLog.Response = strings.Join(responseParts, "\n")
	chatLog.Tokens = promptTokens
	chatLog.UserID = userID
	chatLog.ChatID = chatID

	recordChatRecord(rec)
	recordChatLogEntry(chatLog)
}

// RecordContentMessage delegates to the current recorder
func RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int, userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
) {
	recorderMutex.RLock()
	recorder := currentRecorder
	recorderMutex.RUnlock()

	if recorder != nil {
		recorder.RecordContentMessage(mc, cr, vendor, name, chatID, timeMs, userID, appID, llmID, t, svc)
	}
}

func recordToolCall(tc *models.ToolCallRecord) {
	recMutex.RLock()
	if !recStarted {
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	select {
	case toolCallChan <- tc:
	default:
		slog.Warn("tool call buffer full, dropping tool call")
	}
}

// RecordChatRecord implements Recorder.RecordChatRecord for DatabaseRecorder
func (r *DatabaseRecorder) RecordChatRecord(record *models.LLMChatRecord) {
	recordChatRecord(record)
}

// RecordChatRecord delegates to the current recorder
func RecordChatRecord(record *models.LLMChatRecord) {
	recorderMutex.RLock()
	recorder := currentRecorder
	recorderMutex.RUnlock()

	if recorder != nil {
		recorder.RecordChatRecord(record)
	}
}

func recordChatRecord(record *models.LLMChatRecord) {
	recMutex.RLock()
	if !recStarted {
		log.Printf("Analytics recording not started, dropping chat record: model=%s, app_id=%d, llm_id=%d, cost=%.2f", record.Name, record.AppID, record.LLMID, record.Cost)
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	select {
	case chatRecordChan <- record:
		log.Printf("Sent chat record to channel: model=%s, app_id=%d, llm_id=%d, cost=%.2f", record.Name, record.AppID, record.LLMID, record.Cost)
	default:
		log.Printf("Chat record buffer full, dropping record: model=%s, app_id=%d, llm_id=%d", record.Name, record.AppID, record.LLMID)
	}
}

func recordChatLogEntry(log *models.LLMChatLogEntry) {
	recMutex.RLock()
	if !recStarted {
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	select {
	case logEntryChan <- log:
	default:
		slog.Warn("chat log buffer full, dropping log")
	}
}

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

func (r *DatabaseRecorder) start(ctx context.Context) {
	recMutex.Lock()
	if recStarted {
		recMutex.Unlock()
		return
	}
	recStarted = true
	recMutex.Unlock()

	initDB(r.db)

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

	slog.Info("starting analytics recording", "buffer_size", defaultBufferSize)

	chatRecordChan = make(chan *models.LLMChatRecord, defaultBufferSize)
	logEntryChan = make(chan *models.LLMChatLogEntry, defaultBufferSize)
	toolCallChan = make(chan *models.ToolCallRecord, defaultBufferSize)
	proxyLogChan = make(chan *models.ProxyLog, defaultBufferSize)

	go func() {
		for {
			select {
			case record := <-chatRecordChan:
				err := r.db.Create(record).Error
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
			case log := <-logEntryChan:
				err := r.db.Create(log).Error
				if err != nil {
					slog.Warn("error creating chat log entry", "error", err)
				}
			case toolCall := <-toolCallChan:
				err := r.db.Create(toolCall).Error
				if err != nil {
					slog.Warn("error creating tool call record", "error", err)
				}
			case proxyLog := <-proxyLogChan:
				err := r.db.Create(proxyLog).Error
				if err != nil {
					slog.Warn("error creating proxy log", "error", err)
				}
			case <-ctx.Done():
				slog.Info("shutting down analytics recording")
				recMutex.Lock()
				recStarted = false
				recMutex.Unlock()
				close(chatRecordChan)
				close(logEntryChan)
				close(toolCallChan)
				close(proxyLogChan)
				return
			}
		}
	}()
}

// NoOpRecorder implements Recorder with no-op methods
type NoOpRecorder struct{}

func (r *NoOpRecorder) RecordProxyLog(log *models.ProxyLog)                                {}
func (r *NoOpRecorder) RecordChatRecord(record *models.LLMChatRecord)                      {}
func (r *NoOpRecorder) RecordToolCall(name string, t time.Time, execTime int, toolID uint) {}
func (r *NoOpRecorder) RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int, userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
) {
}

// RESTAPIRecorder is an example implementation that sends analytics to a REST API
type RESTAPIRecorder struct {
	endpoint string
	apiKey   string
	// Other fields like HTTP client, etc.
}

func (r *RESTAPIRecorder) RecordProxyLog(log *models.ProxyLog) {
	// Example implementation - in a real scenario, you would:
	// 1. Convert the log to an appropriate format for your API
	// 2. Send an async HTTP request to your endpoint
	// 3. Handle errors, retries, etc.
	slog.Info("RESTAPIRecorder: would send proxy log to API endpoint",
		"endpoint", r.endpoint,
		"log_id", log.ID)
}

func (r *RESTAPIRecorder) RecordChatRecord(record *models.LLMChatRecord) {
	// Similar implementation to RecordProxyLog for chat records
	slog.Info("RESTAPIRecorder: would send chat record to API endpoint",
		"endpoint", r.endpoint,
		"model", record.Name,
		"vendor", record.Vendor,
		"cost", record.Cost/10000)
}

func (r *RESTAPIRecorder) RecordToolCall(name string, t time.Time, execTime int, toolID uint) {
	// Implementation for tool call recording
	slog.Info("RESTAPIRecorder: would send tool call to API endpoint",
		"endpoint", r.endpoint,
		"name", name,
		"execTime", execTime)
}

func (r *RESTAPIRecorder) RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int, userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
) {
	// Implementation for content message recording
	slog.Info("RESTAPIRecorder: would send content message to API endpoint",
		"endpoint", r.endpoint,
		"model", name,
		"vendor", vendor)
}

// SetRecorder sets a custom recorder implementation
func SetRecorder(recorder Recorder) {
	recorderMutex.Lock()
	defer recorderMutex.Unlock()
	currentRecorder = recorder
}

// ResetToDefaultRecorder resets to the default database recorder
func ResetToDefaultRecorder() {
	recorderMutex.Lock()
	defer recorderMutex.Unlock()
	currentRecorder = defaultRecorder
}

// DisableRecording sets a no-op recorder
func DisableRecording() {
	SetRecorder(&NoOpRecorder{})
}

// StartRecording initializes the default recorder and starts recording analytics
func StartRecording(ctx context.Context, db *gorm.DB) {
	recorderMutex.Lock()

	// Initialize the default recorder
	defaultRecorder = &DatabaseRecorder{
		db: db,
	}

	// Set the current recorder to the default if not already set
	if currentRecorder == nil {
		currentRecorder = defaultRecorder
	}

	recorderMutex.Unlock()

	// Start the database recorder (if it's being used)
	if dbRecorder, ok := defaultRecorder.(*DatabaseRecorder); ok {
		dbRecorder.start(ctx)
	}
}
