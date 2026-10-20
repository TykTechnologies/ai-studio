package scripting

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// testAnalyticsHandler captures compliance events for test assertions
type testAnalyticsHandler struct {
	mu     sync.Mutex
	events []*models.ComplianceEvent
}

func (h *testAnalyticsHandler) RecordComplianceEvents(_ context.Context, events []*models.ComplianceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, events...)
}
func (h *testAnalyticsHandler) RecordChatRecord(_ context.Context, record *models.LLMChatRecord)  {}
func (h *testAnalyticsHandler) RecordChatLogEntry(_ context.Context, log *models.LLMChatLogEntry)  {}
func (h *testAnalyticsHandler) RecordProxyLog(_ context.Context, log *models.ProxyLog)             {}
func (h *testAnalyticsHandler) RecordToolCall(_ context.Context, name string, t time.Time, execTime int, toolID uint) {
}
func (h *testAnalyticsHandler) SetAsGlobalHandler()                                                  {}
func (h *testAnalyticsHandler) RecordChatRecordsBatch(_ context.Context, records []*models.LLMChatRecord) {}
func (h *testAnalyticsHandler) RecordProxyLogsBatch(_ context.Context, logs []*models.ProxyLog)    {}

func (h *testAnalyticsHandler) getEvents() []*models.ComplianceEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]*models.ComplianceEvent, len(h.events))
	copy(result, h.events)
	return result
}

func TestRecordComplianceEvents_EnrichesContext(t *testing.T) {
	handler := &testAnalyticsHandler{}
	analytics.SetHandler(handler)
	defer analytics.ResetHandler()

	output := &ScriptOutput{
		Block:   false,
		Payload: "[REDACTED]",
		ComplianceEvents: []ComplianceEventOutput{
			{
				EventType:   "pii_redacted",
				Severity:    "warning",
				Description: "SSN pattern found",
				Metadata:    map[string]interface{}{"pattern": "ssn", "count": int64(3)},
			},
			{
				EventType:   "content_logged",
				Severity:    "info",
				Description: "Input logged for audit",
			},
		},
	}

	RecordComplianceEvents(
		context.Background(),
		output,
		"my-pii-filter",   // filterName
		"proxy_request",    // filterScope
		42,                 // appID
		7,                  // userID
		15,                 // llmID
		"openai",           // vendor
		"gpt-4",            // modelName
	)

	events := handler.getEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify first event is enriched with context
	e := events[0]
	if e.AppID != 42 {
		t.Errorf("AppID = %d, want 42", e.AppID)
	}
	if e.UserID != 7 {
		t.Errorf("UserID = %d, want 7", e.UserID)
	}
	if e.LLMID != 15 {
		t.Errorf("LLMID = %d, want 15", e.LLMID)
	}
	if e.FilterName != "my-pii-filter" {
		t.Errorf("FilterName = %q, want %q", e.FilterName, "my-pii-filter")
	}
	if e.FilterScope != "proxy_request" {
		t.Errorf("FilterScope = %q, want %q", e.FilterScope, "proxy_request")
	}
	if e.EventType != "pii_redacted" {
		t.Errorf("EventType = %q, want %q", e.EventType, "pii_redacted")
	}
	if e.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", e.Severity, "warning")
	}
	if e.Description != "SSN pattern found" {
		t.Errorf("Description = %q, want %q", e.Description, "SSN pattern found")
	}
	if e.Vendor != "openai" {
		t.Errorf("Vendor = %q, want %q", e.Vendor, "openai")
	}
	if e.ModelName != "gpt-4" {
		t.Errorf("ModelName = %q, want %q", e.ModelName, "gpt-4")
	}
	if e.TimeStamp.IsZero() {
		t.Error("TimeStamp should not be zero")
	}

	// Verify metadata was serialized to JSON
	if e.Metadata == "" {
		t.Error("Metadata should not be empty")
	}
	if e.Metadata != `{"count":3,"pattern":"ssn"}` {
		t.Errorf("Metadata = %q, want JSON with pattern and count", e.Metadata)
	}

	// Verify second event
	e2 := events[1]
	if e2.EventType != "content_logged" {
		t.Errorf("second event EventType = %q, want %q", e2.EventType, "content_logged")
	}
	if e2.FilterName != "my-pii-filter" {
		t.Errorf("second event FilterName = %q, want %q", e2.FilterName, "my-pii-filter")
	}
}

func TestRecordComplianceEvents_NilOutput(t *testing.T) {
	handler := &testAnalyticsHandler{}
	analytics.SetHandler(handler)
	defer analytics.ResetHandler()

	// Should not panic
	RecordComplianceEvents(context.Background(), nil, "filter", "scope", 1, 1, 1, "v", "m")

	events := handler.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events for nil output, got %d", len(events))
	}
}

func TestRecordComplianceEvents_EmptyEvents(t *testing.T) {
	handler := &testAnalyticsHandler{}
	analytics.SetHandler(handler)
	defer analytics.ResetHandler()

	output := &ScriptOutput{
		Block:            false,
		ComplianceEvents: []ComplianceEventOutput{},
	}

	RecordComplianceEvents(context.Background(), output, "filter", "scope", 1, 1, 1, "v", "m")

	events := handler.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events for empty slice, got %d", len(events))
	}
}

func TestRecordComplianceEvents_NoComplianceEvents(t *testing.T) {
	handler := &testAnalyticsHandler{}
	analytics.SetHandler(handler)
	defer analytics.ResetHandler()

	output := &ScriptOutput{
		Block:   false,
		Payload: "clean",
	}

	RecordComplianceEvents(context.Background(), output, "filter", "scope", 1, 1, 1, "v", "m")

	events := handler.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events when no compliance events set, got %d", len(events))
	}
}
