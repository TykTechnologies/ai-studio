package analytics

import (
	"context"
	"log/slog"
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
	// Legacy variables kept for backwards compatibility
	recStarted bool
	recMutex   sync.RWMutex
)

func RecordProxyLog(log *models.ProxyLog) {
	if globalHandler != nil {
		globalHandler.RecordProxyLog(log)
	}
}

func RecordToolCall(name string, t time.Time, execTime int, toolID uint) {
	if globalHandler != nil {
		globalHandler.RecordToolCall(name, t, execTime, toolID)
	}
}

func RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int, userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
) {
	// Check if analytics handler is available
	if globalHandler == nil {
		return
	}

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

	RecordChatRecord(rec)
	RecordChatLogEntry(chatLog)
}

func RecordChatRecord(record *models.LLMChatRecord) {
	if globalHandler != nil {
		globalHandler.RecordChatRecord(record)
	}
}

func RecordChatLogEntry(log *models.LLMChatLogEntry) {
	if globalHandler != nil {
		globalHandler.RecordChatLogEntry(log)
	}
}

// InitDefault initializes the default database analytics handler
func InitDefault(ctx context.Context, db *gorm.DB) {
	if globalHandler == nil {
		handler := NewDatabaseHandler(ctx, db)
		SetHandler(handler)
	}
}

// Init initializes analytics with default database handler (for backward compatibility)
func Init(ctx context.Context, db *gorm.DB) {
	InitDefault(ctx, db)
}

// StartRecording is deprecated, use InitDefault instead
func StartRecording(ctx context.Context, db *gorm.DB) {
	// Just delegate to the new interface-based system
	InitDefault(ctx, db)

	// Set the legacy flag for any code that checks it
	recMutex.Lock()
	recStarted = true
	recMutex.Unlock()

	// Wait for context cancellation to reset the flag
	go func() {
		<-ctx.Done()
		recMutex.Lock()
		recStarted = false
		recMutex.Unlock()
	}()
}
