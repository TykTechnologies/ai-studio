package analytics

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"
)

// Records requests
type LLMChatRecord struct {
	gorm.Model
	ID             uint `gorm:"primaryKey"`
	Name           string
	Vendor         string
	TotalTimeMS    int
	PromptTokens   int
	ResponseTokens int
	TotalTokens    int
	TimeStamp      time.Time
	UserID         uint
	Choices        int
	ToolCalls      int
	ChatID         string
	AppID          uint
	Cost           float64
}

// logs content
type LLMChatLogEntry struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	Name      string
	Vendor    string
	TimeStamp time.Time
	Prompt    string
	Response  string
	Tokens    int
	UserID    uint
}

// records Tool Calls
type ToolCallRecord struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	ToolID    uint
	Name      string
	ExecTime  int
	TimeStamp time.Time
}

var (
	chatRecordChan chan *LLMChatRecord
	logEntryChan   chan *LLMChatLogEntry
	toolCallChan   chan *ToolCallRecord
)

func RecordToolCall(name string, t time.Time, execTime int, toolID uint) {
	if !recStarted {
		return
	}

	tcEntry := &ToolCallRecord{}
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
	timeMs int, userID, appID uint,
	t time.Time,
	svc *services.Service,
) {

	if !recStarted {
		return
	}

	rec := &LLMChatRecord{}

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
	if err == nil {
		cpt = price.CPT
	}

	rec.Choices = len(cr.Choices)
	rec.Name = name
	rec.Vendor = string(vendor)
	rec.TotalTimeMS = timeMs
	rec.PromptTokens = promptTokens
	rec.ResponseTokens = responseTokens
	rec.TotalTokens = totalTokens
	rec.TimeStamp = t
	rec.UserID = userID
	rec.ToolCalls = toolCalls
	rec.ChatID = chatID
	rec.AppID = appID
	rec.Cost = cpt * float64(totalTokens)

	// LLM Response
	chatLog := &LLMChatLogEntry{}
	chatLog.Name = name
	chatLog.Vendor = string(vendor)
	chatLog.TimeStamp = t
	chatLog.Prompt = prompt
	chatLog.Response = strings.Join(responseParts, "\n")
	chatLog.Tokens = promptTokens
	chatLog.UserID = userID

	recordChatRecord(rec)
	recordChatLogEntry(chatLog)
}

// records a tool call
func recordToolCall(tc *ToolCallRecord) {
	select {
	case toolCallChan <- tc:
	default:
		slog.Warn("tool call buffer full, dropping tool call")
	}
}

func RecordChatRecord(record *LLMChatRecord) {
	recordChatRecord(record)
}

// Records a chat record
func recordChatRecord(record *LLMChatRecord) {
	select {
	case chatRecordChan <- record:
	default:
		slog.Warn("chat record buffer full, dropping record")
	}
}

// Records a chat log entry
func recordChatLogEntry(log *LLMChatLogEntry) {
	select {
	case logEntryChan <- log:
	default:
		slog.Warn("chat log buffer full, dropping log")
	}
}

func initDB(db *gorm.DB) {
	err := db.AutoMigrate(
		&LLMChatRecord{},
		&LLMChatLogEntry{},
		&ToolCallRecord{})

	if err != nil {
		slog.Warn("error migrating analytics tables", "error", err)
	}
}

var recStarted bool

func StartRecording(ctx context.Context, db *gorm.DB) {
	if recStarted {
		return
	}
	recStarted = true

	initDB(db)

	defaultBufferSize := 100
	analyticsBufferSizeStr := os.Getenv("ANALYTICS_BUFFER_ZIZE")
	if analyticsBufferSizeStr != "" {
		bfr, err := strconv.Atoi(analyticsBufferSizeStr)
		if err != nil {
			slog.Warn("ANALYTICS_BUFFER_SIZE must be a string", "error", err)
		} else {
			defaultBufferSize = bfr
		}
	}

	chatRecordChan = make(chan *LLMChatRecord, defaultBufferSize)
	logEntryChan = make(chan *LLMChatLogEntry, defaultBufferSize)
	toolCallChan = make(chan *ToolCallRecord, defaultBufferSize)

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
			case <-ctx.Done():
				slog.Info("shutting down analytics recording")
				close(chatRecordChan)
				close(logEntryChan)
				close(toolCallChan)
				recStarted = false
				return
			}
		}
	}()
}
