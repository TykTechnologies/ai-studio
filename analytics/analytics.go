package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/metrics"
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

func RecordProxyLog(ctx context.Context, log *models.ProxyLog) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordProxyLog(ctx, log)
	}

	metrics.RecordRequest(ctx,
		fmt.Sprintf("%d", log.AppID),
		log.Vendor,
		log.ModelName,
		log.ResponseCode,
	)
}

func RecordToolCall(ctx context.Context, name string, t time.Time, execTime int, toolID uint) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordToolCall(ctx, name, t, execTime, toolID)
	}

	metrics.RecordToolCall(ctx, name, fmt.Sprintf("%d", toolID))
	metrics.ObserveToolDuration(ctx, name, float64(execTime)/1000.0)
}

func RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int, userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
	dontLogBodies bool,
) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
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
	chatLog.Tokens = promptTokens
	chatLog.UserID = userID
	chatLog.ChatID = chatID

	if !dontLogBodies {
		chatLog.Prompt = prompt
		chatLog.Response = strings.Join(responseParts, "\n")
	}

	// No HTTP request context available in chat session path
	RecordChatRecord(context.Background(), rec)
	RecordChatLogEntry(context.Background(), chatLog)
}

func RecordChatRecord(ctx context.Context, record *models.LLMChatRecord) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordChatRecord(ctx, record)
	}

	appID := fmt.Sprintf("%d", record.AppID)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "prompt", record.PromptTokens)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "completion", record.ResponseTokens)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "cache_read", record.CacheReadPromptTokens)
	metrics.RecordTokens(ctx, record.Vendor, record.Name, "cache_write", record.CacheWritePromptTokens)
	metrics.RecordCost(ctx, record.Vendor, record.Name, appID, record.Cost)
}

func RecordChatLogEntry(ctx context.Context, log *models.LLMChatLogEntry) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordChatLogEntry(ctx, log)
	}
}

// RecordComplianceEvents records filter script compliance events asynchronously
func RecordComplianceEvents(ctx context.Context, events []*models.ComplianceEvent) {
	if len(events) == 0 {
		return
	}

	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordComplianceEvents(ctx, events)
	}

	for _, event := range events {
		metrics.RecordComplianceEvent(ctx, event.EventType, event.Severity, event.FilterName)
	}
}

// RecordChatRecordsBatch records multiple chat records in a batch for improved performance
func RecordChatRecordsBatch(ctx context.Context, records []*models.LLMChatRecord) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordChatRecordsBatch(ctx, records)
	}
}

// RecordProxyLogsBatch records multiple proxy logs in a batch for improved performance
func RecordProxyLogsBatch(ctx context.Context, logs []*models.ProxyLog) {
	handlerMu.RLock()
	defer handlerMu.RUnlock()

	if globalHandler != nil {
		globalHandler.RecordProxyLogsBatch(ctx, logs)
	}
}

// InitDefault initializes the default database analytics handler
func InitDefault(ctx context.Context, db *gorm.DB) {
	handlerMu.Lock()
	defer handlerMu.Unlock()

	if globalHandler == nil {
		globalHandler = NewDatabaseHandler(ctx, db)
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
