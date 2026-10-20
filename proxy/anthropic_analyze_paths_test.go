package proxy

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// chatRecordCounter is an analytics handler that keeps the chat records and
// proxy logs it is handed, synchronously.
type chatRecordCounter struct {
	mu      sync.Mutex
	records []*models.LLMChatRecord
	logs    []*models.ProxyLog
}

func (h *chatRecordCounter) RecordProxyLog(_ context.Context, l *models.ProxyLog) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, l)
}
func (h *chatRecordCounter) RecordChatRecord(_ context.Context, rec *models.LLMChatRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, rec)
}
func (h *chatRecordCounter) RecordChatLogEntry(context.Context, *models.LLMChatLogEntry)       {}
func (h *chatRecordCounter) RecordToolCall(context.Context, string, time.Time, int, uint)      {}
func (h *chatRecordCounter) RecordChatRecordsBatch(context.Context, []*models.LLMChatRecord)   {}
func (h *chatRecordCounter) RecordProxyLogsBatch(context.Context, []*models.ProxyLog)          {}
func (h *chatRecordCounter) RecordComplianceEvents(context.Context, []*models.ComplianceEvent) {}
func (*chatRecordCounter) RecordMCPCalls(context.Context, []models.MCPCallRecord) error { return nil }
func (h *chatRecordCounter) SetAsGlobalHandler()                                               { analytics.SetHandler(h) }
func (h *chatRecordCounter) count() int                                                        { h.mu.Lock(); defer h.mu.Unlock(); return len(h.records) }
func (h *chatRecordCounter) logCount() int                                                     { h.mu.Lock(); defer h.mu.Unlock(); return len(h.logs) }

// Only an Anthropic Messages call carries usage. Batch objects, batch results
// and count_tokens answers used to match the substring /v1/messages and were
// written as zero-cost chat records; they now write none, while /v1/messages
// on every route shape still records one. Every one of them still writes its
// proxy log (the audit trail of the call).
func TestAnalyzeResponse_AnthropicRecordsMessagesOnly(t *testing.T) {
	const model = "claude-luna"
	message := []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"` + model + `","content":[],
		"usage":{"input_tokens":10,"output_tokens":5}}`)
	batch := []byte(`{"id":"msgbatch_1","type":"message_batch","processing_status":"in_progress",
		"request_counts":{"processing":2,"succeeded":0,"errored":0,"canceled":0,"expired":0}}`)
	countTokens := []byte(`{"input_tokens":42}`)

	for _, tc := range []struct {
		path   string
		body   []byte
		record bool
	}{
		{"/llm/rest/claude/v1/messages", message, true},
		{"/llm/call/claude/v1/messages", message, true},
		{"/anthropic/claude/v1/messages", message, true},
		{"/v1/messages/", message, true},
		{"/llm/rest/claude/v1/messages/batches", batch, false},
		{"/llm/rest/claude/v1/messages/batches/msgbatch_1", batch, false},
		{"/llm/rest/claude/v1/messages/batches/msgbatch_1/cancel", batch, false},
		{"/llm/rest/claude/v1/messages/batches/msgbatch_1/results", []byte(`{"custom_id":"a","result":{"type":"succeeded"}}`), false},
		{"/llm/rest/claude/v1/messages/count_tokens", countTokens, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			h := &chatRecordCounter{}
			analytics.SetHandler(h)
			defer analytics.ResetHandler()

			svc := new(MockService)
			svc.On("GetModelPriceByModelNameAndVendor", mock.Anything, string(models.ANTHROPIC)).
				Return(&models.ModelPrice{ModelName: model, Vendor: string(models.ANTHROPIC), CPT: 0.00001, CPIT: 0.00001, Currency: "USD"}, nil)

			req, err := http.NewRequest(http.MethodPost, "http://gw"+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			AnalyzeResponse(svc, &models.LLM{ID: 1, Vendor: models.ANTHROPIC}, &models.App{ID: 2}, http.StatusOK, tc.body, nil, req)

			if assert.Equal(t, 1, h.logCount(), "every call writes one proxy log") {
				assert.Equal(t, uint(2), h.logs[0].AppID)
				assert.Equal(t, uint(1), h.logs[0].LLMID)
				assert.Equal(t, http.StatusOK, h.logs[0].ResponseCode)
				assert.Equal(t, string(tc.body), h.logs[0].ResponseBody)
			}
			if !tc.record {
				assert.Zero(t, h.count(), "no chat record for a non-Messages call")
				return
			}
			if assert.Equal(t, 1, h.count()) {
				assert.Equal(t, model, h.records[0].Name)
				assert.Greater(t, h.records[0].Cost, 0.0)
			}
		})
	}
}
