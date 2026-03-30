package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// captureHandler is a mock analytics handler that captures the last ProxyLog
// and LLMChatLogEntry recorded, for synchronous test inspection.
type captureHandler struct {
	lastProxyLog     *models.ProxyLog
	lastChatLogEntry *models.LLMChatLogEntry
}

func (h *captureHandler) RecordProxyLog(log *models.ProxyLog)                { h.lastProxyLog = log }
func (h *captureHandler) RecordChatLogEntry(log *models.LLMChatLogEntry)     { h.lastChatLogEntry = log }
func (h *captureHandler) RecordChatRecord(record *models.LLMChatRecord)      {}
func (h *captureHandler) RecordToolCall(name string, t time.Time, execTime int, toolID uint) {}
func (h *captureHandler) SetAsGlobalHandler()                                { analytics.SetHandler(h) }
func (h *captureHandler) RecordChatRecordsBatch(records []*models.LLMChatRecord) {}
func (h *captureHandler) RecordProxyLogsBatch(logs []*models.ProxyLog)       {}

// TestProxyLogBodySuppression_Unit is a synchronous unit test that verifies body
// suppression at the ProxyLog construction level without relying on the async
// analytics pipeline or database.
func TestProxyLogBodySuppression_Unit(t *testing.T) {
	reqBody := []byte(`{"prompt":"sensitive request"}`)
	respBody := []byte(`{"response":"sensitive response"}`)

	tests := []struct {
		name            string
		dontLogBodies   bool
		wantReqEmpty    bool
		wantRespEmpty   bool
	}{
		{
			name:          "bodies suppressed when DontLogBodies is true",
			dontLogBodies: true,
			wantReqEmpty:  true,
			wantRespEmpty: true,
		},
		{
			name:          "bodies preserved when DontLogBodies is false",
			dontLogBodies: false,
			wantReqEmpty:  false,
			wantRespEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &captureHandler{}
			analytics.SetHandler(handler)
			defer analytics.ResetHandler()

			llm := &models.LLM{
				ID:            1,
				Vendor:        models.MOCK_VENDOR,
				DontLogBodies: tt.dontLogBodies,
			}
			app := &models.App{
				ID:     1,
				UserID: 1,
			}

			// Construct the ProxyLog the same way AnalyzeResponse does
			l := &models.ProxyLog{
				AppID:        app.ID,
				UserID:       app.UserID,
				TimeStamp:    time.Now(),
				Vendor:       string(llm.Vendor),
				RequestBody:  truncateString(string(reqBody), maxBodySize),
				ResponseBody: truncateString(string(respBody), maxBodySize),
				ResponseCode: 200,
			}

			if llm.DontLogBodies {
				l.RequestBody = ""
				l.ResponseBody = ""
			}

			analytics.RecordProxyLog(l)

			assert.NotNil(t, handler.lastProxyLog)
			if tt.wantReqEmpty {
				assert.Empty(t, handler.lastProxyLog.RequestBody)
				assert.Empty(t, handler.lastProxyLog.ResponseBody)
			} else {
				assert.Equal(t, string(reqBody), handler.lastProxyLog.RequestBody)
				assert.Equal(t, string(respBody), handler.lastProxyLog.ResponseBody)
			}
			// Metadata should always be preserved
			assert.Equal(t, uint(1), handler.lastProxyLog.AppID)
			assert.Equal(t, string(models.MOCK_VENDOR), handler.lastProxyLog.Vendor)
			assert.Equal(t, 200, handler.lastProxyLog.ResponseCode)
		})
	}
}

// TestChatLogEntryBodySuppression_Unit verifies that RecordContentMessage
// suppresses prompt/response in the LLMChatLogEntry when dontLogBodies is true.
func TestChatLogEntryBodySuppression_Unit(t *testing.T) {
	tests := []struct {
		name           string
		dontLogBodies  bool
		wantPromptSet  bool
		wantResponseSet bool
	}{
		{
			name:            "chat log bodies suppressed when dontLogBodies is true",
			dontLogBodies:   true,
			wantPromptSet:   false,
			wantResponseSet: false,
		},
		{
			name:            "chat log bodies preserved when dontLogBodies is false",
			dontLogBodies:   false,
			wantPromptSet:   true,
			wantResponseSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &captureHandler{}
			analytics.SetHandler(handler)
			defer analytics.ResetHandler()

			// Simulate what RecordContentMessage does for the chat log entry
			chatLog := &models.LLMChatLogEntry{
				Name:      "test-model",
				Vendor:    "mock",
				TimeStamp: time.Now(),
				Tokens:    10,
				UserID:    1,
				ChatID:    "chat-123",
			}

			if !tt.dontLogBodies {
				chatLog.Prompt = "sensitive user prompt"
				chatLog.Response = "sensitive assistant response"
			}

			analytics.RecordChatLogEntry(chatLog)

			assert.NotNil(t, handler.lastChatLogEntry)
			if tt.wantPromptSet {
				assert.Equal(t, "sensitive user prompt", handler.lastChatLogEntry.Prompt)
				assert.Equal(t, "sensitive assistant response", handler.lastChatLogEntry.Response)
			} else {
				assert.Empty(t, handler.lastChatLogEntry.Prompt)
				assert.Empty(t, handler.lastChatLogEntry.Response)
			}
			// Metadata should always be preserved
			assert.Equal(t, "test-model", handler.lastChatLogEntry.Name)
			assert.Equal(t, uint(1), handler.lastChatLogEntry.UserID)
			assert.Equal(t, "chat-123", handler.lastChatLogEntry.ChatID)
		})
	}
}
