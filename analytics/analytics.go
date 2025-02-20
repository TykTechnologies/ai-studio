package analytics

import (
	"context"
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

var (
	chatRecordChan chan *models.LLMChatRecord
	logEntryChan   chan *models.LLMChatLogEntry
	toolCallChan   chan *models.ToolCallRecord
	proxyLogChan   chan *models.ProxyLog
	recStarted     bool
	recMutex       sync.RWMutex
)

func RecordProxyLog(log *models.ProxyLog) {
	recMutex.RLock()
	if !recStarted {
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	proxyLogChan <- log
}

func RecordToolCall(name string, t time.Time, execTime int, toolID uint) {
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

// Will create a Analytics ChatLog and ChatRecord entry for a given ContentMessage
func RecordContentMessage(
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

	rec.Choices = len(cr.Choices)
	rec.Name = price.ModelName
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
	rec.Cost = cpt*float64(responseTokens) + cpit*float64(promptTokens)
	rec.Currency = price.Currency
	rec.InteractionType = models.ChatInteraction
	rec.LLMID = llmID

	// LLM Response
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

// records a tool call
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

func RecordChatRecord(record *models.LLMChatRecord) {
	recordChatRecord(record)
}

// Records a chat record
func recordChatRecord(record *models.LLMChatRecord) {
	recMutex.RLock()
	if !recStarted {
		recMutex.RUnlock()
		return
	}
	recMutex.RUnlock()

	select {
	case chatRecordChan <- record:
	default:
		slog.Warn("chat record buffer full, dropping record")
	}
}

// Records a chat log entry
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

func StartRecording(ctx context.Context, db *gorm.DB) {
	recMutex.Lock()
	if recStarted {
		recMutex.Unlock()
		return
	}
	recStarted = true
	recMutex.Unlock()

	initDB(db)

	defaultBufferSize := 1000
	analyticsBufferSizeStr := os.Getenv("ANALYTICS_BUFFER_ZIZE")
	if analyticsBufferSizeStr != "" {
		bfr, err := strconv.Atoi(analyticsBufferSizeStr)
		if err != nil {
			slog.Warn("ANALYTICS_BUFFER_SIZE must be a string", "error", err)
		} else {
			defaultBufferSize = bfr
		}
	}

	chatRecordChan = make(chan *models.LLMChatRecord, defaultBufferSize)
	logEntryChan = make(chan *models.LLMChatLogEntry, defaultBufferSize)
	toolCallChan = make(chan *models.ToolCallRecord, defaultBufferSize)
	proxyLogChan = make(chan *models.ProxyLog, defaultBufferSize)

	go func() {
		for {
			select {
			case record := <-chatRecordChan:
				err := db.Create(record).Error
				if err != nil {
					slog.Warn("error creating chat record", "error", err)
				}
			case log := <-logEntryChan:
				err := db.Create(log).Error
				if err != nil {
					slog.Warn("error creating chat log entry", "error", err)
				}
			case toolCall := <-toolCallChan:
				err := db.Create(toolCall).Error
				if err != nil {
					slog.Warn("error creating tool call record", "error", err)
				}
			case proxyLog := <-proxyLogChan:
				err := db.Create(proxyLog).Error
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
