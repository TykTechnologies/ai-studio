package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// FileAnalyticsHandler implements analytics.AnalyticsHandler using file-based logging
type FileAnalyticsHandler struct {
	logDir        string
	budgetService *FileBudgetService
	mu            sync.Mutex
	chatRecords   []models.LLMChatRecord
	toolCalls     []ToolCallRecord
	proxyLogs     []models.ProxyLog
}

// ToolCallRecord represents a tool call for analytics
type ToolCallRecord struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
	ExecTime  int       `json:"exec_time_ms"`
	ToolID    uint      `json:"tool_id"`
}

// NewFileAnalyticsHandler creates a new file-based analytics handler
func NewFileAnalyticsHandler(logDir string, budgetService *FileBudgetService) (*FileAnalyticsHandler, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	handler := &FileAnalyticsHandler{
		logDir:        logDir,
		budgetService: budgetService,
		chatRecords:   make([]models.LLMChatRecord, 0),
		toolCalls:     make([]ToolCallRecord, 0),
		proxyLogs:     make([]models.ProxyLog, 0),
	}

	// Load existing records if they exist
	handler.loadExistingData()

	return handler, nil
}

// loadExistingData loads existing analytics data from files
func (h *FileAnalyticsHandler) loadExistingData() {
	// Load chat records
	if data, err := os.ReadFile(filepath.Join(h.logDir, "chat_records.json")); err == nil {
		var records []models.LLMChatRecord
		if err := json.Unmarshal(data, &records); err == nil {
			h.chatRecords = records
		}
	}

	// Load tool calls
	if data, err := os.ReadFile(filepath.Join(h.logDir, "tool_calls.json")); err == nil {
		var records []ToolCallRecord
		if err := json.Unmarshal(data, &records); err == nil {
			h.toolCalls = records
		}
	}

	// Load proxy logs
	if data, err := os.ReadFile(filepath.Join(h.logDir, "proxy_logs.json")); err == nil {
		var records []models.ProxyLog
		if err := json.Unmarshal(data, &records); err == nil {
			h.proxyLogs = records
		}
	}
}

// RecordChatRecord records LLM chat/proxy usage
func (h *FileAnalyticsHandler) RecordChatRecord(record *models.LLMChatRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Add to in-memory storage
	h.chatRecords = append(h.chatRecords, *record)

	// Convert cost from cents to dollars for budget tracking
	costInDollars := float64(record.Cost) / 10000.0

	// Log to console for demo purposes
	log.Printf("[ANALYTICS] Chat Record: LLM=%d, Model=%s, Tokens=%d, Cost=$%.4f, App=%d",
		record.LLMID, record.Name, record.TotalTokens, costInDollars, record.AppID)

	// Update budget usage if budget service is available
	if h.budgetService != nil {
		h.budgetService.AddUsage(record.AppID, record.LLMID, costInDollars)
		log.Printf("[BUDGET] Updated usage: App=%d, LLM=%d, Cost=$%.4f",
			record.AppID, record.LLMID, costInDollars)
	}

	// Save to file
	h.saveChatRecords()
}

// RecordProxyLog records proxy request/response logs
func (h *FileAnalyticsHandler) RecordProxyLog(logEntry *models.ProxyLog) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Add to in-memory storage
	h.proxyLogs = append(h.proxyLogs, *logEntry)

	// Log to console for demo purposes
	log.Printf("[ANALYTICS] Proxy Log: Vendor=%s, ResponseCode=%d, App=%d, User=%d",
		logEntry.Vendor, logEntry.ResponseCode, logEntry.AppID, logEntry.UserID)

	// Save to file
	h.saveProxyLogs()
}

// RecordToolCall records tool call execution
func (h *FileAnalyticsHandler) RecordToolCall(name string, timestamp time.Time, execTime int, toolID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	record := ToolCallRecord{
		Name:      name,
		Timestamp: timestamp,
		ExecTime:  execTime,
		ToolID:    toolID,
	}

	// Add to in-memory storage
	h.toolCalls = append(h.toolCalls, record)

	// Log to console for demo purposes
	log.Printf("[ANALYTICS] Tool Call: Name=%s, ExecTime=%dms, ToolID=%d",
		name, execTime, toolID)

	// Save to file
	h.saveToolCalls()
}

// RecordChatLogEntry records chat log entries (required for analytics.AnalyticsHandler interface)
func (h *FileAnalyticsHandler) RecordChatLogEntry(logEntry *models.LLMChatLogEntry) {
	// For this demo, we'll just log to console as we don't store chat log entries in files
	log.Printf("[ANALYTICS] Chat Log Entry: Model=%s, Vendor=%s, User=%d, Chat=%s",
		logEntry.Name, logEntry.Vendor, logEntry.UserID, logEntry.ChatID)
}

// SetAsGlobalHandler sets this handler as the global analytics handler
func (h *FileAnalyticsHandler) SetAsGlobalHandler() {
	analytics.SetHandler(h)
}

// saveChatRecords saves chat records to file
func (h *FileAnalyticsHandler) saveChatRecords() {
	data, err := json.MarshalIndent(h.chatRecords, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal chat records: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(h.logDir, "chat_records.json"), data, 0644); err != nil {
		log.Printf("Failed to save chat records: %v", err)
	}
}

// saveToolCalls saves tool calls to file
func (h *FileAnalyticsHandler) saveToolCalls() {
	data, err := json.MarshalIndent(h.toolCalls, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal tool calls: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(h.logDir, "tool_calls.json"), data, 0644); err != nil {
		log.Printf("Failed to save tool calls: %v", err)
	}
}

// saveProxyLogs saves proxy logs to file
func (h *FileAnalyticsHandler) saveProxyLogs() {
	data, err := json.MarshalIndent(h.proxyLogs, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal proxy logs: %v", err)
		return
	}

	if err := os.WriteFile(filepath.Join(h.logDir, "proxy_logs.json"), data, 0644); err != nil {
		log.Printf("Failed to save proxy logs: %v", err)
	}
}

// GetChatRecords returns all chat records for analysis
func (h *FileAnalyticsHandler) GetChatRecords() []models.LLMChatRecord {
	h.mu.Lock()
	defer h.mu.Unlock()

	records := make([]models.LLMChatRecord, len(h.chatRecords))
	copy(records, h.chatRecords)
	return records
}

// GetToolCalls returns all tool call records for analysis
func (h *FileAnalyticsHandler) GetToolCalls() []ToolCallRecord {
	h.mu.Lock()
	defer h.mu.Unlock()

	records := make([]ToolCallRecord, len(h.toolCalls))
	copy(records, h.toolCalls)
	return records
}

// GetProxyLogs returns all proxy log records for analysis
func (h *FileAnalyticsHandler) GetProxyLogs() []models.ProxyLog {
	h.mu.Lock()
	defer h.mu.Unlock()

	records := make([]models.ProxyLog, len(h.proxyLogs))
	copy(records, h.proxyLogs)
	return records
}

// PrintSummary prints a summary of analytics data
func (h *FileAnalyticsHandler) PrintSummary() {
	h.mu.Lock()
	defer h.mu.Unlock()

	fmt.Printf("\n=== Analytics Summary ===\n")
	fmt.Printf("Total Chat Records: %d\n", len(h.chatRecords))
	fmt.Printf("Total Tool Calls: %d\n", len(h.toolCalls))
	fmt.Printf("Total Proxy Logs: %d\n", len(h.proxyLogs))

	if len(h.chatRecords) > 0 {
		var totalTokens int
		var totalCost float64
		for _, record := range h.chatRecords {
			totalTokens += record.TotalTokens
			totalCost += record.Cost
		}
		fmt.Printf("Total Tokens Used: %d\n", totalTokens)
		fmt.Printf("Total Cost: $%.4f\n", totalCost/10000.0)
	}

	fmt.Printf("=== End Summary ===\n\n")
}

// ClearData clears all analytics data (useful for testing)
func (h *FileAnalyticsHandler) ClearData() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.chatRecords = make([]models.LLMChatRecord, 0)
	h.toolCalls = make([]ToolCallRecord, 0)
	h.proxyLogs = make([]models.ProxyLog, 0)

	// Remove files
	os.Remove(filepath.Join(h.logDir, "chat_records.json"))
	os.Remove(filepath.Join(h.logDir, "tool_calls.json"))
	os.Remove(filepath.Join(h.logDir, "proxy_logs.json"))
}

// Ensure FileAnalyticsHandler implements the interface
var _ analytics.AnalyticsHandler = (*FileAnalyticsHandler)(nil)
