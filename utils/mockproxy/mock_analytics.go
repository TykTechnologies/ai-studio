// Package main provides a mock implementation of the analytics.Recorder
// interface for the mockproxy utility without requiring a database connection.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/llms"
)

// MockRecorder implements analytics.Recorder interface for demo purposes
type MockRecorder struct {
	logFile    *os.File
	logMutex   sync.Mutex
	outputMode string // "console", "file", or "both"
}

// NewMockRecorder creates a new MockRecorder
func NewMockRecorder(outputMode string, logFilePath string) (*MockRecorder, error) {
	recorder := &MockRecorder{
		outputMode: outputMode,
	}

	// If we're logging to a file, open it
	if outputMode == "file" || outputMode == "both" {
		file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %v", err)
		}
		recorder.logFile = file
	}

	return recorder, nil
}

// Close closes the log file if it's open
func (r *MockRecorder) Close() {
	if r.logFile != nil {
		r.logFile.Close()
	}
}

// logData logs data to console, file, or both based on configuration
func (r *MockRecorder) logData(recordType string, data interface{}) {
	r.logMutex.Lock()
	defer r.logMutex.Unlock()

	// Marshal the data to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("Error marshaling %s data: %v", recordType, err)
		return
	}

	// Format the log entry
	timestamp := time.Now().Format(time.RFC3339)
	logEntry := fmt.Sprintf("[%s] %s Record:\n%s\n\n", timestamp, recordType, jsonData)

	// Log to console if configured
	if r.outputMode == "console" || r.outputMode == "both" {
		fmt.Print(logEntry)
	}

	// Log to file if configured
	if (r.outputMode == "file" || r.outputMode == "both") && r.logFile != nil {
		if _, err := r.logFile.WriteString(logEntry); err != nil {
			log.Printf("Error writing to log file: %v", err)
		}
	}
}

// RecordProxyLog implements Recorder.RecordProxyLog
func (r *MockRecorder) RecordProxyLog(log *models.ProxyLog) {
	r.logData("ProxyLog", log)
}

// RecordChatRecord implements Recorder.RecordChatRecord
func (r *MockRecorder) RecordChatRecord(record *models.LLMChatRecord) {
	r.logData("ChatRecord", record)
}

// RecordToolCall implements Recorder.RecordToolCall
func (r *MockRecorder) RecordToolCall(name string, t time.Time, execTime int, toolID uint) {
	toolCall := struct {
		Name     string    `json:"name"`
		Time     time.Time `json:"time"`
		ExecTime int       `json:"exec_time_ms"`
		ToolID   uint      `json:"tool_id"`
	}{
		Name:     name,
		Time:     t,
		ExecTime: execTime,
		ToolID:   toolID,
	}
	r.logData("ToolCall", toolCall)
}

// RecordContentMessage implements Recorder.RecordContentMessage
func (r *MockRecorder) RecordContentMessage(
	mc *llms.MessageContent,
	cr *llms.ContentResponse,
	vendor models.Vendor,
	name, chatID string,
	timeMs int,
	userID, appID, llmID uint,
	t time.Time,
	svc services.ServiceInterface,
) {
	// Create a simplified structure for logging
	contentMsg := struct {
		Model            string        `json:"model"`
		Vendor           models.Vendor `json:"vendor"`
		ChatID           string        `json:"chat_id"`
		ProcessingTimeMs int           `json:"processing_time_ms"`
		UserID           uint          `json:"user_id"`
		AppID            uint          `json:"app_id"`
		LLMID            uint          `json:"llm_id"`
		Timestamp        time.Time     `json:"timestamp"`
		PromptTokens     int           `json:"prompt_tokens,omitempty"`
		ResponseTokens   int           `json:"response_tokens,omitempty"`
	}{
		Model:            name,
		Vendor:           vendor,
		ChatID:           chatID,
		ProcessingTimeMs: timeMs,
		UserID:           userID,
		AppID:            appID,
		LLMID:            llmID,
		Timestamp:        t,
	}

	// Extract token counts if available
	if cr != nil && len(cr.Choices) > 0 {
		// For simplicity in this mock implementation, we'll just use the first choice
		// In a real implementation, you would sum tokens across all choices
		choice := cr.Choices[0]
		_, promptTokens, responseTokens := switches.GetTokenCounts(choice, vendor)
		contentMsg.PromptTokens = promptTokens
		contentMsg.ResponseTokens = responseTokens
	}

	r.logData("ContentMessage", contentMsg)
}
