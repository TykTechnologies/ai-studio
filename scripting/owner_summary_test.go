package scripting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// metadataJSON round-trips metadata as the hub stores and reads it.
func metadataJSON(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOwnerSummary(t *testing.T) {
	action, code, text := OwnerSummary("guardrail.unscanned", metadataJSON(t, map[string]any{
		"reason": "size", "action": "block", "max_chars": 321097, "limit": 262144,
		"segments": []map[string]any{{"kind": "tool_result", "role": "tool", "tool": "fetch_results", "chars": 321097, "sha256": "ab"}},
	}))
	if action != "block" || code != ReasonTooLargeToScan ||
		text != "a tool result from fetch_results of 321,097 characters is longer than the guardrail scans (262,144 characters), so it was not checked" {
		t.Fatalf("unscanned: %q %q %q", action, code, text)
	}

	action, code, text = OwnerSummary("guardrail.tool_definition.block", metadataJSON(t, map[string]any{
		"action": "remove", "category": "injection", "max_score": 0.99, "detector": "tool_definition.block",
		"segments": []map[string]any{{"kind": "tool_definition", "role": "system", "tool": "refine-core_run_next", "excerpt": "SECRET TEXT"}},
	}))
	if action != "remove" || code != "" || text != "the guardrail found a likely prompt injection in the definition of the tool refine-core_run_next" {
		t.Fatalf("detection: %q %q %q", action, code, text)
	}
	for _, leak := range []string{"SECRET", "0.99", "tool_definition.block"} {
		if strings.Contains(text, leak) {
			t.Fatalf("summary %q shows %q", text, leak)
		}
	}

	_, code, _ = OwnerSummary("guardrail.error", metadataJSON(t, map[string]any{"blocked": true}))
	if code != ReasonUnavailable {
		t.Fatalf("error code = %q", code)
	}
	if _, _, text := OwnerSummary("guardrail.x", map[string]any{}); text == "" {
		t.Fatal("empty metadata gave no summary")
	}
}

// Only a filter that shares with App owners marks its events, and only the
// ones where it acted on the request.
func TestMarkOwnerVisible(t *testing.T) {
	body := chatBody(t,
		chatMsg{Role: "user", Content: "summarise"},
		chatMsg{Role: "tool", ToolCallID: "c1", Content: "data SECRET more"})

	shared := scanFilter(t, &scanFake{caps: guardrails.Capabilities{ToolSegments: true}},
		map[string]any{"tool_results": map[string]any{"on_detect": "block"}, "evidence": map[string]any{"share_with_app_owners": true}})
	out := runRequest(t, shared, body, app(5))
	if len(out.ComplianceEvents) == 0 {
		t.Fatal("no events")
	}
	for _, e := range out.ComplianceEvents {
		if e.Metadata[OwnerVisibleKey] != true {
			t.Fatalf("%s not marked: %v", e.EventType, e.Metadata)
		}
	}

	logOnly := scanFilter(t, &scanFake{caps: guardrails.Capabilities{ToolSegments: true}},
		map[string]any{"on_detect": "log", "tool_results": map[string]any{"on_detect": "log"}, "evidence": map[string]any{"share_with_app_owners": true}})
	for _, e := range runRequest(t, logOnly, body, app(5)).ComplianceEvents {
		if _, marked := e.Metadata[OwnerVisibleKey]; marked {
			t.Fatalf("a log-only event was shared: %v", e.Metadata)
		}
	}

	notShared := scanFilter(t, &scanFake{caps: guardrails.Capabilities{ToolSegments: true}},
		map[string]any{"tool_results": map[string]any{"on_detect": "block"}})
	for _, e := range runRequest(t, notShared, body, app(5)).ComplianceEvents {
		if _, marked := e.Metadata[OwnerVisibleKey]; marked {
			t.Fatalf("an unshared filter's event was marked: %v", e.Metadata)
		}
	}
}
