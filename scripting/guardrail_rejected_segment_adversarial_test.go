package scripting

// Adversarial tests for 4uc.29 (tests agent): a provider that refuses the
// text of one tool result (400/413/422) must cost only that tool result,
// never the user's own prompt or the other tool results, under quarantine;
// and nothing the provider did not scan may pass.

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// rejectingFake refuses any call holding "REJECTME" with ErrInputRejected,
// flags "SECRET", and counts calls.
type rejectingFake struct {
	calls atomic.Int32
}

func (f *rejectingFake) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{}
}

func (f *rejectingFake) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	f.calls.Add(1)
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "REJECTME") {
			return guardrails.Verdict{}, fmt.Errorf("provider said 400: %w", guardrails.ErrInputRejected)
		}
		if strings.Contains(s.Text, "SECRET") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "secrets.test_key", Category: "secrets", Severity: "warning", Score: 1, SegmentIndex: s.Index})
		}
	}
	return v, nil
}

func TestAdversarial_RejectedToolResultCostsOnlyItself(t *testing.T) {
	freshVerdictCache(t, 0)
	cases := []struct {
		name     string
		msgs     []chatMsg
		gone     []string // texts that must be quarantined
		kept     []string // texts that must reach upstream unchanged
		wantPass bool
	}{
		{
			name: "one rejected tool result beside a clean one",
			msgs: []chatMsg{
				{Role: "user", Content: "summarise both pages"},
				{Role: "tool", Content: "page one: REJECTME \xed\xa0\x80", ToolCallID: "c1"},
				{Role: "tool", Content: "page two: all good", ToolCallID: "c2"},
			},
			gone: []string{"page one"}, kept: []string{"summarise both pages", "page two: all good"}, wantPass: true,
		},
		{
			name: "rejected and flagged tool results together",
			msgs: []chatMsg{
				{Role: "user", Content: "go"},
				{Role: "tool", Content: "a REJECTME", ToolCallID: "c1"},
				{Role: "tool", Content: "b SECRET", ToolCallID: "c2"},
				{Role: "tool", Content: "c fine", ToolCallID: "c3"},
			},
			gone: []string{"a REJECTME", "b SECRET"}, kept: []string{`"go"`, "c fine"}, wantPass: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &rejectingFake{}
			f := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
			out := runRequest(t, f, chatBody(t, c.msgs...), app(1))
			if out.Block == c.wantPass {
				t.Fatalf("block = %v (%s), want %v; events %v", out.Block, out.Message, !c.wantPass, eventTypes(out))
			}
			for _, g := range c.gone {
				if strings.Contains(out.Payload, g) {
					t.Errorf("%q reached upstream", g)
				}
			}
			for _, k := range c.kept {
				if !strings.Contains(out.Payload, k) {
					t.Errorf("%q was removed although the provider scanned it clean", k)
				}
			}
		})
	}
}

// Under block, a rejected tool result still blocks (nothing unscanned
// passes), and so does a rejected user message under quarantine.
func TestAdversarial_RejectedTextNeverPasses(t *testing.T) {
	freshVerdictCache(t, 0)
	p := &rejectingFake{}
	toolBlock := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionBlock, "scope": guardrails.ScopeAllMessages, "tool_results": toolResultsBlock})
	rejectedTool := chatBody(t, chatMsg{Role: "user", Content: "go"}, chatMsg{Role: "tool", Content: "REJECTME", ToolCallID: "c1"})
	out := runRequest(t, toolBlock, rejectedTool, app(1))
	if !out.Block {
		t.Error("tool_results.on_detect block let an unscannable tool result through")
	}
	// A block filter with no tool_results key quarantines the tool result
	// (4uc.5): the request continues without it.
	block := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionBlock, "scope": guardrails.ScopeAllMessages})
	out = runRequest(t, block, rejectedTool, app(1))
	if out.Block || strings.Contains(out.Payload, "REJECTME") || !strings.Contains(out.Payload, `"go"`) {
		t.Errorf("default block filter: want the unscannable tool result quarantined and the prompt kept; block=%v payload=%s", out.Block, out.Payload)
	}
	// A rejected user message under that block filter still blocks.
	out = runRequest(t, block, chatBody(t, chatMsg{Role: "user", Content: "REJECTME please"}), app(1))
	if !out.Block {
		t.Error("an unscannable user message passed a block filter")
	}
	q := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
	out = runRequest(t, q, chatBody(t, chatMsg{Role: "user", Content: "REJECTME please"}), app(1))
	if !out.Block && strings.Contains(out.Payload, "REJECTME") {
		t.Error("an unscannable user message passed under quarantine")
	}
}
