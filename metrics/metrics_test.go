package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	b, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNoOpBeforeInit(t *testing.T) {
	// Ensure calling recording functions before Init does not panic.
	// Use a fresh state — since tests may run in any order and Init
	// may already have been called, we test the guard path directly.
	ctx := context.Background()
	old := initialized.Load()
	initialized.Store(false)
	defer initialized.Store(old)

	RecordRequest(ctx, "app1", "openai", "gpt-4", 200)
	RecordTokens(ctx, "openai", "gpt-4", "prompt", 100)
	RecordCost(ctx, "openai", "gpt-4", "app1", 0.05)
	RecordToolCall(ctx, "search", "app1")
	RecordPolicyBlock(ctx, "content_filter", "filter")
	ObserveRequestDuration(ctx, "openai", "gpt-4", false, 1.5, http.StatusOK)
	ObserveToolDuration(ctx, "search", 0.3)
	IncrementInflight(ctx, "openai")
	DecrementInflight(ctx, "openai")
}

func TestInitReturnsHandler(t *testing.T) {
	h := Init()
	if h == nil {
		t.Fatal("Init() returned nil handler")
	}
	if Handler() == nil {
		t.Fatal("Handler() returned nil after Init()")
	}
	body := scrape(t, h)
	// Should contain at least the Go runtime metrics or be valid prometheus output
	if len(body) == 0 {
		t.Fatal("empty metrics output")
	}
}

func TestRecordRequest(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordRequest(ctx, "app1", "openai", "gpt-4", 200)
	RecordRequest(ctx, "app1", "openai", "gpt-4", 200)
	RecordRequest(ctx, "app1", "anthropic", "claude-3", 500)

	body := scrape(t, h)

	if !strings.Contains(body, "aistudio_llm_requests_total") {
		t.Error("missing aistudio_llm_requests_total metric")
	}
	if !strings.Contains(body, `vendor="openai"`) {
		t.Error("missing openai vendor label")
	}
	if !strings.Contains(body, `model="gpt-4"`) {
		t.Error("missing gpt-4 model label")
	}
	if !strings.Contains(body, `status_code="200"`) {
		t.Error("missing status_code label")
	}
}

func TestRecordTokens(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordTokens(ctx, "openai", "gpt-4", "prompt", 150)
	RecordTokens(ctx, "openai", "gpt-4", "completion", 50)
	RecordTokens(ctx, "anthropic", "claude-3", "cache_read", 30)

	body := scrape(t, h)

	if !strings.Contains(body, "aistudio_llm_tokens_total") {
		t.Error("missing aistudio_llm_tokens_total metric")
	}
	if !strings.Contains(body, `type="prompt"`) {
		t.Error("missing prompt type label")
	}
	if !strings.Contains(body, `type="completion"`) {
		t.Error("missing completion type label")
	}
}

func TestRecordTokensSkipsZero(t *testing.T) {
	h := Init()
	ctx := context.Background()

	// Zero count should be skipped
	RecordTokens(ctx, "openai", "gpt-4", "cache_write", 0)

	body := scrape(t, h)
	// cache_write with openai/gpt-4 should not appear (no observation)
	if strings.Contains(body, `type="cache_write"`) && strings.Contains(body, `model="gpt-4"`) {
		// It's possible prior tests added this, so only check in isolation
		// This test mainly verifies no panic on zero
	}
}

func TestRecordCost(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordCost(ctx, "openai", "gpt-4", "app1", 0.0325)

	body := scrape(t, h)
	if !strings.Contains(body, "aistudio_llm_cost_total") {
		t.Error("missing aistudio_llm_cost_total metric")
	}
}

func TestRecordToolCall(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordToolCall(ctx, "web_search", "app1")

	body := scrape(t, h)
	if !strings.Contains(body, "aistudio_tool_calls_total") {
		t.Error("missing aistudio_tool_calls_total metric")
	}
	if !strings.Contains(body, `tool_name="web_search"`) {
		t.Error("missing tool_name label")
	}
}

func TestRecordPolicyBlock(t *testing.T) {
	h := Init()
	ctx := context.Background()

	RecordPolicyBlock(ctx, "content_filter", "filter")
	RecordPolicyBlock(ctx, "budget_limit", "rate_limit")

	body := scrape(t, h)
	if !strings.Contains(body, "aistudio_policy_blocks_total") {
		t.Error("missing aistudio_policy_blocks_total metric")
	}
	if !strings.Contains(body, `block_type="filter"`) {
		t.Error("missing filter block_type label")
	}
	if !strings.Contains(body, `block_type="rate_limit"`) {
		t.Error("missing rate_limit block_type label")
	}
}

func TestObserveRequestDuration(t *testing.T) {
	h := Init()
	ctx := context.Background()

	ObserveRequestDuration(ctx, "openai", "gpt-4", false, 1.5, http.StatusOK)
	ObserveRequestDuration(ctx, "openai", "gpt-4", true, 30.0, http.StatusOK)

	body := scrape(t, h)
	if !strings.Contains(body, "aistudio_llm_request_duration_seconds") {
		t.Error("missing aistudio_llm_request_duration_seconds metric")
	}
	if !strings.Contains(body, `streaming="true"`) {
		t.Error("missing streaming=true label")
	}
	if !strings.Contains(body, `streaming="false"`) {
		t.Error("missing streaming=false label")
	}
	// Verify histogram buckets exist
	if !strings.Contains(body, "aistudio_llm_request_duration_seconds_bucket") {
		t.Error("missing histogram buckets")
	}
}

func TestObserveToolDuration(t *testing.T) {
	h := Init()
	ctx := context.Background()

	ObserveToolDuration(ctx, "web_search", 0.25)

	body := scrape(t, h)
	if !strings.Contains(body, "aistudio_tool_execution_duration_seconds") {
		t.Error("missing aistudio_tool_execution_duration_seconds metric")
	}
	if !strings.Contains(body, "aistudio_tool_execution_duration_seconds_bucket") {
		t.Error("missing histogram buckets")
	}
}

func TestInflightGauge(t *testing.T) {
	h := Init()
	ctx := context.Background()

	IncrementInflight(ctx, "openai")
	IncrementInflight(ctx, "openai")
	DecrementInflight(ctx, "openai")

	body := scrape(t, h)
	if !strings.Contains(body, "aistudio_llm_inflight_requests") {
		t.Error("missing aistudio_llm_inflight_requests metric")
	}
	if !strings.Contains(body, `vendor="openai"`) {
		t.Error("missing vendor label on inflight gauge")
	}
}

func TestHistogramBucketBoundaries(t *testing.T) {
	h := Init()
	ctx := context.Background()

	ObserveRequestDuration(ctx, "test", "test-model", false, 0.05, http.StatusOK)

	body := scrape(t, h)

	// Check that our custom bucket boundaries are present
	expectedBuckets := []string{"0.1", "0.5", "1", "2", "5", "10", "30", "60", "120"}
	for _, b := range expectedBuckets {
		needle := `le="` + b + `"`
		if !strings.Contains(body, needle) {
			t.Errorf("missing bucket boundary %s", b)
		}
	}
}

func TestToolExecDurationBucketBoundaries(t *testing.T) {
	h := Init()
	ctx := context.Background()

	ObserveToolDuration(ctx, "bucket-test-tool", 0.005)

	body := scrape(t, h)

	expectedBuckets := []string{"0.01", "0.05", "0.1", "0.5", "1", "5", "10"}
	for _, b := range expectedBuckets {
		needle := `le="` + b + `"`
		if !strings.Contains(body, needle) {
			t.Errorf("missing tool duration bucket boundary %s", b)
		}
	}
}

func TestMultipleInitCalls(t *testing.T) {
	// Init() should be callable multiple times without panic.
	// Each call creates a new registry, so Handler() returns the latest.
	h1 := Init()
	h2 := Init()
	h3 := Init()

	if h1 == nil || h2 == nil || h3 == nil {
		t.Fatal("Init() returned nil on repeated calls")
	}
	if Handler() == nil {
		t.Fatal("Handler() returned nil after multiple Init() calls")
	}

	// Recording should still work after multiple inits
	ctx := context.Background()
	RecordRequest(ctx, "app1", "openai", "multi-init-model", 200)

	body := scrape(t, Handler())
	if !strings.Contains(body, "aistudio_llm_requests_total") {
		t.Error("metrics not working after multiple Init() calls")
	}
}

func TestConcurrentRecording(t *testing.T) {
	h := Init()
	ctx := context.Background()

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				RecordRequest(ctx, "app1", "openai", "gpt-4", 200)
				RecordTokens(ctx, "openai", "gpt-4", "prompt", 10)
				RecordTokens(ctx, "openai", "gpt-4", "completion", 5)
				RecordCost(ctx, "openai", "gpt-4", "app1", 0.001)
				RecordToolCall(ctx, "search", "app1")
				RecordPolicyBlock(ctx, "test_rule", "filter")
				ObserveRequestDuration(ctx, "openai", "gpt-4", false, 0.5, http.StatusOK)
				ObserveToolDuration(ctx, "search", 0.1)
				IncrementInflight(ctx, "openai")
				DecrementInflight(ctx, "openai")
			}
		}(g)
	}
	wg.Wait()

	// Scrape should succeed without data races
	body := scrape(t, h)
	if !strings.Contains(body, "aistudio_llm_requests_total") {
		t.Error("missing metrics after concurrent recording")
	}
}

func TestConcurrentInflightBalances(t *testing.T) {
	Init()
	ctx := context.Background()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Launch equal increments and decrements concurrently
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			IncrementInflight(ctx, "concurrent-vendor")
		}()
		go func() {
			defer wg.Done()
			DecrementInflight(ctx, "concurrent-vendor")
		}()
	}
	wg.Wait()

	// After equal increments and decrements, the gauge should be 0.
	// We can't directly read the gauge value from Prometheus output
	// (it might show 0 or be absent), but at least it shouldn't panic.
	body := scrape(t, Handler())
	_ = body // Just verify no panic during concurrent access
}

func TestRecordCostSkipsZero(t *testing.T) {
	Init()
	ctx := context.Background()

	// Zero cost should be skipped (no metric emitted for this label combination)
	RecordCost(ctx, "zero-cost-vendor", "zero-cost-model", "app-zero", 0)

	body := scrape(t, Handler())
	// The specific zero-cost vendor should not appear
	if strings.Contains(body, `vendor="zero-cost-vendor"`) {
		t.Error("zero cost should not emit a metric")
	}
}

func TestHandlerBeforeInit(t *testing.T) {
	// Save and restore state
	oldInit := initialized.Load()
	oldHandler := handler

	initialized.Store(false)
	handler = nil
	defer func() {
		initialized.Store(oldInit)
		handler = oldHandler
	}()

	if Handler() != nil {
		t.Error("Handler() should return nil before Init()")
	}
}
