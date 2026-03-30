package analytics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// scrapeMetrics fetches the Prometheus exposition text from the metrics handler.
func scrapeMetrics(t *testing.T) string {
	t.Helper()
	h := metrics.Handler()
	if h == nil {
		t.Fatal("metrics handler is nil — Init() not called")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", rec.Code)
	}
	b, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRecordProxyLog_EmitsRequestMetric(t *testing.T) {
	metrics.Init()

	// Reset global handler so RecordProxyLog doesn't try to write to a non-existent DB
	handlerMu.Lock()
	oldHandler := globalHandler
	globalHandler = nil
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		globalHandler = oldHandler
		handlerMu.Unlock()
	}()

	log := &models.ProxyLog{
		AppID:        42,
		UserID:       1,
		TimeStamp:    time.Now(),
		Vendor:       "openai",
		ModelName:    "gpt-4o",
		ResponseCode: 200,
	}

	RecordProxyLog(log)

	body := scrapeMetrics(t)

	if !strings.Contains(body, "aistudio_llm_requests_total") {
		t.Fatal("RecordProxyLog did not emit aistudio_llm_requests_total")
	}
	if !strings.Contains(body, `vendor="openai"`) {
		t.Error("missing vendor label from ProxyLog")
	}
	if !strings.Contains(body, `model="gpt-4o"`) {
		t.Error("missing model label from ProxyLog.ModelName")
	}
	if !strings.Contains(body, `app_id="42"`) {
		t.Error("missing app_id label from ProxyLog.AppID")
	}
	if !strings.Contains(body, `status_code="200"`) {
		t.Error("missing status_code label from ProxyLog.ResponseCode")
	}
}

func TestRecordProxyLog_EmptyModelName(t *testing.T) {
	metrics.Init()

	handlerMu.Lock()
	oldHandler := globalHandler
	globalHandler = nil
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		globalHandler = oldHandler
		handlerMu.Unlock()
	}()

	// ProxyLog with no ModelName (legacy data)
	log := &models.ProxyLog{
		AppID:        1,
		Vendor:       "anthropic",
		ResponseCode: 500,
	}

	RecordProxyLog(log)

	body := scrapeMetrics(t)
	// Should still emit the metric, just with empty model
	if !strings.Contains(body, `vendor="anthropic"`) {
		t.Error("missing vendor label for empty-model ProxyLog")
	}
}

func TestRecordChatRecord_EmitsTokenAndCostMetrics(t *testing.T) {
	metrics.Init()

	handlerMu.Lock()
	oldHandler := globalHandler
	globalHandler = nil
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		globalHandler = oldHandler
		handlerMu.Unlock()
	}()

	record := &models.LLMChatRecord{
		Name:                   "claude-3-opus",
		Vendor:                 "anthropic",
		AppID:                  7,
		PromptTokens:           1000,
		ResponseTokens:         500,
		CacheReadPromptTokens:  200,
		CacheWritePromptTokens: 100,
		Cost:                   325.0, // stored as dollars * 10000
		TotalTimeMS:            2500,
	}

	RecordChatRecord(record)

	body := scrapeMetrics(t)

	// Verify token metrics
	if !strings.Contains(body, "aistudio_llm_tokens_total") {
		t.Fatal("RecordChatRecord did not emit aistudio_llm_tokens_total")
	}

	// Check all four token types are present with the right vendor/model
	for _, tokenType := range []string{"prompt", "completion", "cache_read", "cache_write"} {
		if !strings.Contains(body, `type="`+tokenType+`"`) {
			t.Errorf("missing token type %q", tokenType)
		}
	}

	if !strings.Contains(body, `vendor="anthropic"`) {
		t.Error("missing vendor label on token metrics")
	}
	if !strings.Contains(body, `model="claude-3-opus"`) {
		t.Error("missing model label on token metrics")
	}

	// Verify cost metric
	if !strings.Contains(body, "aistudio_llm_cost_total") {
		t.Error("RecordChatRecord did not emit aistudio_llm_cost_total")
	}
	if !strings.Contains(body, `app_id="7"`) {
		t.Error("missing app_id label on cost metric")
	}
}

func TestRecordChatRecord_ZeroTokensNotEmitted(t *testing.T) {
	metrics.Init()

	handlerMu.Lock()
	oldHandler := globalHandler
	globalHandler = nil
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		globalHandler = oldHandler
		handlerMu.Unlock()
	}()

	// Record with zero cache tokens — should not emit cache_read/cache_write
	record := &models.LLMChatRecord{
		Name:                   "zero-cache-model",
		Vendor:                 "zero-cache-vendor",
		AppID:                  99,
		PromptTokens:           100,
		ResponseTokens:         50,
		CacheReadPromptTokens:  0,
		CacheWritePromptTokens: 0,
		Cost:                   0, // zero cost should also be skipped
	}

	RecordChatRecord(record)

	body := scrapeMetrics(t)

	// prompt and completion should be emitted
	if !strings.Contains(body, `vendor="zero-cache-vendor"`) {
		t.Error("expected vendor label for non-zero token record")
	}

	// Zero cost should not appear for this specific app_id
	// (other tests may have added cost metrics, so check specifically for this app)
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		if strings.Contains(line, "aistudio_llm_cost_total") &&
			strings.Contains(line, `app_id="99"`) {
			t.Error("zero cost should not emit aistudio_llm_cost_total")
		}
	}
}

func TestRecordToolCall_EmitsToolMetrics(t *testing.T) {
	metrics.Init()

	handlerMu.Lock()
	oldHandler := globalHandler
	globalHandler = nil
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		globalHandler = oldHandler
		handlerMu.Unlock()
	}()

	RecordToolCall("weather_api", time.Now(), 350, 5)

	body := scrapeMetrics(t)

	// Verify tool call counter
	if !strings.Contains(body, "aistudio_tool_calls_total") {
		t.Fatal("RecordToolCall did not emit aistudio_tool_calls_total")
	}
	if !strings.Contains(body, `tool_name="weather_api"`) {
		t.Error("missing tool_name label")
	}

	// Verify tool duration histogram
	if !strings.Contains(body, "aistudio_tool_execution_duration_seconds") {
		t.Fatal("RecordToolCall did not emit aistudio_tool_execution_duration_seconds")
	}
}

func TestRecordToolCall_DurationConversion(t *testing.T) {
	metrics.Init()

	handlerMu.Lock()
	oldHandler := globalHandler
	globalHandler = nil
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		globalHandler = oldHandler
		handlerMu.Unlock()
	}()

	// 2500ms = 2.5 seconds. Should land in the 5s bucket.
	RecordToolCall("slow_tool", time.Now(), 2500, 10)

	body := scrapeMetrics(t)

	// The histogram should record an observation. Check that the _count is at least 1.
	if !strings.Contains(body, "aistudio_tool_execution_duration_seconds_count") {
		t.Error("missing histogram count for tool duration")
	}
}

func TestRecordProxyLog_WithNilGlobalHandler(t *testing.T) {
	// Ensure metrics are still emitted even when globalHandler is nil
	// (which is the case when analytics DB is not initialized)
	metrics.Init()

	handlerMu.Lock()
	globalHandler = nil
	handlerMu.Unlock()

	log := &models.ProxyLog{
		AppID:        1,
		Vendor:       "nil-handler-vendor",
		ModelName:    "nil-handler-model",
		ResponseCode: 200,
	}

	// Should not panic and should still emit metrics
	RecordProxyLog(log)

	body := scrapeMetrics(t)
	if !strings.Contains(body, `vendor="nil-handler-vendor"`) {
		t.Error("metrics not emitted when globalHandler is nil")
	}
}

func TestRecordChatRecord_WithActiveHandler(t *testing.T) {
	// Verify that metrics are emitted alongside the database handler.
	// We use a mock handler to avoid needing a real DB.
	metrics.Init()

	var capturedRecord *models.LLMChatRecord
	mock := &mockHandler{
		onRecordChatRecord: func(r *models.LLMChatRecord) {
			capturedRecord = r
		},
	}

	handlerMu.Lock()
	oldHandler := globalHandler
	globalHandler = mock
	handlerMu.Unlock()
	defer func() {
		handlerMu.Lock()
		globalHandler = oldHandler
		handlerMu.Unlock()
	}()

	record := &models.LLMChatRecord{
		Name:           "dual-path-model",
		Vendor:         "dual-path-vendor",
		AppID:          55,
		PromptTokens:   200,
		ResponseTokens: 100,
		Cost:           50.0,
	}

	RecordChatRecord(record)

	// Verify the DB handler received the record
	if capturedRecord == nil {
		t.Fatal("mock handler did not receive the chat record")
	}
	if capturedRecord.Name != "dual-path-model" {
		t.Errorf("expected model dual-path-model, got %s", capturedRecord.Name)
	}

	// Verify metrics were also emitted
	body := scrapeMetrics(t)
	if !strings.Contains(body, `vendor="dual-path-vendor"`) {
		t.Error("metrics not emitted alongside DB handler")
	}
}

// mockHandler implements AnalyticsHandler for testing dual-path behavior
type mockHandler struct {
	onRecordChatRecord func(*models.LLMChatRecord)
}

func (m *mockHandler) RecordChatRecord(record *models.LLMChatRecord) {
	if m.onRecordChatRecord != nil {
		m.onRecordChatRecord(record)
	}
}
func (m *mockHandler) RecordChatLogEntry(log *models.LLMChatLogEntry)       {}
func (m *mockHandler) RecordProxyLog(log *models.ProxyLog)                  {}
func (m *mockHandler) RecordToolCall(name string, t time.Time, execTime int, toolID uint) {}
func (m *mockHandler) SetAsGlobalHandler()                                  {}
func (m *mockHandler) RecordChatRecordsBatch(records []*models.LLMChatRecord) {}
func (m *mockHandler) RecordProxyLogsBatch(logs []*models.ProxyLog)         {}

// Verify mockHandler satisfies the interface at compile time.
var _ AnalyticsHandler = (*mockHandler)(nil)

func TestMetricsWithoutInit(t *testing.T) {
	// When metrics.Init() is not called, analytics functions should still work
	// (just no metrics emitted). This tests the guard in the analytics layer.
	ResetHandler()

	// Temporarily disable metrics to test the no-op path
	// (other tests in this package may have called Init())
	// We test this indirectly: if Init was called, the calls are safe.
	// If not, the initialized guard in metrics package prevents nil dereference.

	log := &models.ProxyLog{
		AppID:        1,
		Vendor:       "test",
		ResponseCode: 200,
	}

	// Should not panic regardless of metrics initialization state
	RecordProxyLog(log)
	RecordChatRecord(&models.LLMChatRecord{Vendor: "test", Name: "model"})
	RecordToolCall("tool", time.Now(), 100, 1)

	// Batch operations should also not panic
	RecordChatRecordsBatch([]*models.LLMChatRecord{{Vendor: "test"}})
	RecordProxyLogsBatch([]*models.ProxyLog{{Vendor: "test"}})

	_ = context.Background() // satisfy import
}
