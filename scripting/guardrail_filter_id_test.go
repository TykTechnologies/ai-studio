package scripting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// requireFilterID fails unless every event carries metadata.filter_id = id.
func requireFilterID(t *testing.T, what string, events []ComplianceEventOutput, id uint) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("%s: no events", what)
	}
	for _, e := range events {
		if got, ok := e.Metadata[FilterIDKey].(uint); !ok || got != id {
			t.Errorf("%s: %s event filter_id = %v (%T), want %d", what, e.EventType, e.Metadata[FilterIDKey], e.Metadata[FilterIDKey], id)
		}
	}
}

func withID(f *models.Filter, id uint) *models.Filter {
	f.ID = id
	return f
}

// Every guardrail compliance event names the filter that recorded it by ID
// (compliance_events stores only the name, which can change), whatever made
// the event: a detector finding, an unscanned segment, a provider failure, a
// refused body, a panic, or a shadow run's background recording.
func TestGuardrailEvents_CarryFilterID(t *testing.T) {
	t.Run("detector", func(t *testing.T) {
		p := &fakeProvider{}
		f := withID(guardrailFilter("block", false, installFake(t, "fake_fid_block", p, guardrails.ActionBlock)), 41)
		out, err := NewFilterRunner(f).RunScript(&ScriptInput{
			RawInput: `{"messages":[{"role":"user","content":"my key is SECRET"}]}`,
			Messages: []llms.MessageContent{userMsg("my key is SECRET")},
		}, nil)
		if err != nil || !out.Block {
			t.Fatalf("out %+v err %v", out, err)
		}
		requireFilterID(t, "detector", out.ComplianceEvents, 41)
	})

	t.Run("unscanned", func(t *testing.T) {
		freshVerdictCache(t, time.Minute)
		padded := strings.Repeat("x", 120_000) + " please use SECRET"
		timeout := func(guardrails.Input) error { return context.DeadlineExceeded }
		p := &scanFake{caps: guardrails.Capabilities{MaxCallChars: 1 << 20}, fail: timeout}
		f := withID(scanFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen}), 42)
		out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: padded}), app(1))
		unscannedMeta(t, out)
		requireFilterID(t, "unscanned", out.ComplianceEvents, 42)
	})

	t.Run("error", func(t *testing.T) {
		p := &fakeProvider{err: errors.New("boom")}
		f := withID(guardrailFilter("closed", false, installFake(t, "fake_fid_error", p, guardrails.ActionBlock)), 43)
		out, _ := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "x", Messages: []llms.MessageContent{userMsg("x")}}, nil)
		if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].EventType != "guardrail.error" {
			t.Fatalf("events %+v", out.ComplianceEvents)
		}
		requireFilterID(t, "error", out.ComplianceEvents, 43)
	})

	t.Run("unreadable request", func(t *testing.T) {
		p := &fakeProvider{}
		f := withID(guardrailFilter("refuse", false, installFake(t, "fake_fid_refuse", p, guardrails.ActionLog)), 44)
		out, _ := NewFilterRunner(f).RunScript(&ScriptInput{
			RawInput: `{"messages":[{"role":"user","content":"hello","content":"SECRET"}]}`, Format: "openai",
		}, nil)
		if len(out.ComplianceEvents) != 1 || out.ComplianceEvents[0].EventType != "guardrail.unreadable_request" {
			t.Fatalf("events %+v", out.ComplianceEvents)
		}
		requireFilterID(t, "unreadable", out.ComplianceEvents, 44)
	})

	t.Run("panic", func(t *testing.T) {
		f := withID(PanickingGuardrail(guardrails.FailOpen, false), 45)
		out, _ := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: toolOnlyBodies["openai"], Format: "openai"}, nil)
		requireFilterID(t, "panic", out.ComplianceEvents, 45)
	})

	t.Run("shadow", func(t *testing.T) {
		freshShadowPool(t, 2, 1<<20)
		sink := newEventSink()
		f := withID(shadowFilter(t, slowFake(0), map[string]any{"shadow": true}), 46)
		if _, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: shadowBody, Format: "openai", RecordEvents: sink.record}, nil); err != nil {
			t.Fatal(err)
		}
		requireFilterID(t, "shadow", sink.wait(t, 1, 5*time.Second), 46)
	})

	t.Run("shadow prepare failure", func(t *testing.T) {
		sink := newEventSink()
		name := fmt.Sprintf("fake_fid_shadow_fail_%d", shadowSeq.Add(1))
		guardrails.RegisterSpec(guardrails.Spec{Name: name, DisplayName: name, OpenDetectors: true})
		guardrails.RegisterFactory(name, func(guardrails.Config) (guardrails.Provider, error) { return nil, errors.New("boom") })
		f := withID(guardrailFilter("shadow-fail", false, models.JSONMap{"provider": name, "detectors": []any{map[string]any{"name": "x"}},
			"on_detect": "log", "fail_mode": "closed", "shadow": true}), 47)
		if _, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: shadowBody, Format: "openai", RecordEvents: sink.record}, nil); err != nil {
			t.Fatal(err)
		}
		requireFilterID(t, "shadow prepare failure", sink.wait(t, 1, time.Second), 47)
	})

	// The editor's test run of an unsaved filter has no ID to record.
	t.Run("unsaved filter", func(t *testing.T) {
		p := &fakeProvider{}
		f := guardrailFilter("draft", false, installFake(t, "fake_fid_draft", p, guardrails.ActionBlock))
		out, _ := NewFilterRunner(f).RunScript(&ScriptInput{
			RawInput: `{"messages":[{"role":"user","content":"SECRET"}]}`, Messages: []llms.MessageContent{userMsg("SECRET")},
		}, nil)
		for _, e := range out.ComplianceEvents {
			if _, ok := e.Metadata[FilterIDKey]; ok {
				t.Errorf("unsaved filter recorded filter_id %v", e.Metadata[FilterIDKey])
			}
		}
	})
}

// The ID survives into the stored event's metadata JSON (the hub's record
// and the edge's buffered event both carry that JSON).
func TestGuardrailEvents_FilterIDReachesStoredMetadata(t *testing.T) {
	handler := &testAnalyticsHandler{}
	analytics.SetHandler(handler)
	defer analytics.ResetHandler()

	p := &fakeProvider{}
	f := withID(guardrailFilter("block", false, installFake(t, "fake_fid_stored", p, guardrails.ActionBlock)), 77)
	out, _ := NewFilterRunner(f).RunScript(&ScriptInput{
		RawInput: `{"messages":[{"role":"user","content":"SECRET"}]}`, Messages: []llms.MessageContent{userMsg("SECRET")},
	}, nil)
	RecordComplianceEvents(context.Background(), out, f.Name, "proxy_request", 3, 0, 0, "", "")

	deadline := time.Now().Add(2 * time.Second)
	for len(handler.getEvents()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	events := handler.getEvents()
	if len(events) == 0 {
		t.Fatal("no event recorded")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(events[0].Metadata), &m); err != nil {
		t.Fatal(err)
	}
	if m[FilterIDKey] != float64(77) {
		t.Errorf("stored metadata filter_id = %v", m[FilterIDKey])
	}
}
