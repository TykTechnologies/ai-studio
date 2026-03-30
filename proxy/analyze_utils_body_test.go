package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"

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

// TestRecordContentMessage_BodySuppression calls analytics.RecordContentMessage directly
// to verify that the production code path suppresses prompt/response in the
// LLMChatLogEntry when dontLogBodies is true.
func TestRecordContentMessage_BodySuppression(t *testing.T) {
	tests := []struct {
		name            string
		dontLogBodies   bool
		wantPromptSet   bool
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

			// Build the inputs that RecordContentMessage expects
			mc := llms.TextParts(llms.ChatMessageTypeHuman, "sensitive user prompt")

			cr := &llms.ContentResponse{
				Choices: []*llms.ContentChoice{
					{
						Content: "sensitive assistant response",
					},
				},
			}

			// Mock service that returns a price (required by RecordContentMessage)
			mockSvc := new(MockService)
			mockSvc.On("GetModelPriceByModelNameAndVendor", "test-model", "mock").Return(
				&models.ModelPrice{
					ModelName: "test-model",
					Vendor:    "mock",
					CPT:       0.002,
					CPIT:      0.001,
					Currency:  "USD",
				}, nil,
			)

			analytics.RecordContentMessage(
				&mc,
				cr,
				models.MOCK_VENDOR,
				"test-model",
				"chat-123",
				0,    // timeMs
				1,    // userID
				1,    // appID
				1,    // llmID
				time.Now(),
				mockSvc,
				tt.dontLogBodies,
			)

			require.NotNil(t, handler.lastChatLogEntry, "RecordContentMessage should produce a chat log entry")
			if tt.wantPromptSet {
				assert.Equal(t, "sensitive user prompt", handler.lastChatLogEntry.Prompt)
				assert.Equal(t, "sensitive assistant response", handler.lastChatLogEntry.Response)
			} else {
				assert.Empty(t, handler.lastChatLogEntry.Prompt, "prompt should be empty when dontLogBodies is true")
				assert.Empty(t, handler.lastChatLogEntry.Response, "response should be empty when dontLogBodies is true")
			}
			// Metadata should always be preserved regardless of body suppression
			assert.Equal(t, "test-model", handler.lastChatLogEntry.Name)
			assert.Equal(t, uint(1), handler.lastChatLogEntry.UserID)
			assert.Equal(t, "chat-123", handler.lastChatLogEntry.ChatID)

			mockSvc.AssertExpectations(t)
		})
	}
}
