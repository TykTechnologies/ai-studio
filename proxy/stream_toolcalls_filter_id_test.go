package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// Streamed tool calls too large to scan are recorded as guardrail.unscanned
// under each scanning filter, with that filter's ID like every guardrail event.
func TestUnscannedHeldCalls_EventCarriesFilterID(t *testing.T) {
	guardrails.RegisterSpec(guardrails.Spec{Name: "held_calls_fid", DisplayName: "held calls",
		Detectors: []guardrails.DetectorSpec{{Name: "injection", Category: "injection"}}})
	old := analytics.GetHandler()
	sink := &eventRecorder{}
	analytics.SetHandler(sink)
	t.Cleanup(func() { analytics.SetHandler(old) })

	f := &models.Filter{Name: "tool-calls", Kind: models.FilterKindGuardrail, ResponseFilter: true, Config: models.JSONMap{
		"provider": "held_calls_fid", "detectors": []any{map[string]any{"name": "injection"}},
		"on_detect": "block", "tool_calls": map[string]any{"scan": true},
	}}
	f.ID = 58
	llm := &models.LLM{Filters: []*models.Filter{f}}
	llm.ID = 3

	blocked, _, err := unscannedHeldCalls(llm, []*heldCall{{id: "c1", name: "run"}}, "size", httptest.NewRequest("POST", "/", nil))
	if err != nil || !blocked {
		t.Fatalf("blocked=%v err=%v", blocked, err)
	}
	var events []*models.ComplianceEvent
	for deadline := time.Now().Add(2 * time.Second); len(events) == 0 && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		sink.mu.Lock()
		events = append(events, sink.events...)
		sink.mu.Unlock()
	}
	if len(events) != 1 || events[0].EventType != "guardrail.unscanned" {
		t.Fatalf("events %+v", events)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(events[0].Metadata), &m); err != nil {
		t.Fatal(err)
	}
	if m[scripting.FilterIDKey] != float64(58) {
		t.Errorf("filter_id = %v", m[scripting.FilterIDKey])
	}
}
