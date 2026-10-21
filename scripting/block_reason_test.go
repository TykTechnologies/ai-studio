package scripting

import (
	"errors"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// A block names its cause, where it was and the setting that applies, and
// never the matched text, the score or the detector.
func TestBlockReason_DetectionNamesCategoryAndSourceOnly(t *testing.T) {
	p := &scanFake{caps: guardrails.Capabilities{ToolSegments: true}}
	f := scanFilter(t, p, map[string]any{"tool_results": map[string]any{"on_detect": "block"}})
	body := chatBody(t,
		chatMsg{Role: "user", Content: "summarise"},
		chatMsg{Role: "tool", ToolCallID: "c1", Content: "data SECRET-sk-live-123 more"})
	out := runRequest(t, f, body, app(7))
	if !out.Block {
		t.Fatalf("not blocked: %v", eventTypes(out))
	}
	if out.ReasonCode != ReasonDetected {
		t.Fatalf("code = %q", out.ReasonCode)
	}
	if !strings.Contains(out.Reason, "credentials") || !strings.Contains(out.Reason, "a tool result") {
		t.Fatalf("reason = %q", out.Reason)
	}
	for _, leak := range []string{"SECRET", "sk-live", "secrets.test_key", "1", "score"} {
		if strings.Contains(out.Reason, leak) {
			t.Fatalf("reason %q shows %q", out.Reason, leak)
		}
	}
}

func TestBlockReason_TooLargeNamesSizeLimitAndSetting(t *testing.T) {
	p := &scanFake{caps: guardrails.Capabilities{ToolSegments: true}}
	f := scanFilter(t, p, map[string]any{
		"tool_results": map[string]any{"on_detect": "block"},
		"budget":       map[string]any{"max_scan_chars": 1000},
	})
	body := chatBody(t,
		chatMsg{Role: "user", Content: "judge these"},
		chatMsg{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("row ", 3000)})
	out := runRequest(t, f, body, app(342))
	if !out.Block || out.ReasonCode != ReasonTooLargeToScan {
		t.Fatalf("block=%v code=%q events=%v", out.Block, out.ReasonCode, eventTypes(out))
	}
	for _, want := range []string{"12,000 characters", "1,000 characters", "budget.max_scan_chars", "a tool result"} {
		if !strings.Contains(out.Reason, want) {
			t.Fatalf("reason %q lacks %q", out.Reason, want)
		}
	}

	// The compliance event says which tool result, how big, and the setting.
	var ev *ComplianceEventOutput
	for i := range out.ComplianceEvents {
		if out.ComplianceEvents[i].EventType == "guardrail.unscanned" {
			ev = &out.ComplianceEvents[i]
		}
	}
	if ev == nil {
		t.Fatalf("no unscanned event: %v", eventTypes(out))
	}
	if ev.Metadata["setting"] != "budget.max_scan_chars" || ev.Metadata["reason_code"] != ReasonTooLargeToScan {
		t.Fatalf("metadata = %v", ev.Metadata)
	}
	segs, _ := ev.Metadata["segments"].([]map[string]any)
	if len(segs) != 1 || segs[0]["chars"] != 12000 || segs[0]["kind"] != guardrails.KindToolResult || segs[0]["sha256"] == "" {
		t.Fatalf("segments = %v", segs)
	}
	if _, has := segs[0]["excerpt"]; has {
		t.Fatal("an unscanned event carries text")
	}
}

func TestBlockReason_FailClosedProviderIsUnavailable(t *testing.T) {
	p := &scanFake{fail: func(guardrails.Input) error { return errProviderDown }}
	f := scanFilter(t, p, map[string]any{"fail_mode": "closed", "scope": guardrails.ScopeAllMessages})
	out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "hello"}), app(1))
	if !out.Block || out.ReasonCode != ReasonUnavailable || !strings.Contains(out.Reason, "fail_mode closed") {
		t.Fatalf("block=%v code=%q reason=%q", out.Block, out.ReasonCode, out.Reason)
	}
}

var errProviderDown = errors.New("provider unreachable")

func TestExplain(t *testing.T) {
	out := &ScriptOutput{Block: true, ReasonCode: ReasonTooLargeToScan, Reason: "a tool result of 9 characters is too long"}
	if got := out.Explain("Blocked"); got != "Blocked. Reason: a tool result of 9 characters is too long (guardrail_too_large_to_scan)." {
		t.Fatalf("Explain = %q", got)
	}
	script := &ScriptOutput{Block: true, Message: "PII"}
	if got := script.Explain("PII"); got != "PII" {
		t.Fatalf("script filter message changed: %q", got)
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 262144: "262,144", 2000000: "2,000,000"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}
