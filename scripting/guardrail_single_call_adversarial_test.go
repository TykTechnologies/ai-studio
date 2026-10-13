package scripting

// Adversarial tests for 4uc.28 (tests agent): the fail mode decides a
// provider outage only for a run of one call no larger than
// budget.single_call_chars (default 32K runes). Padding past that size and
// then inducing a timeout must never let flagged text pass under fail_mode
// open; below it the fail mode still decides, as before.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// sizeFake times out (by sleeping past the context) on any call whose text
// contains a marker, else flags "SECRET", and records each call's size.
type sizeFake struct {
	caps      guardrails.Capabilities
	slowWhen  func(texts []string) bool
	rejectAll string // a call holding this text and another segment is refused

	mu    sync.Mutex
	sizes []int
}

func (f *sizeFake) Capabilities() guardrails.Capabilities { return f.caps }

func (f *sizeFake) Classify(ctx context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	var texts []string
	size := 0
	for _, s := range in.Segments {
		texts = append(texts, s.Text)
		size += utf8.RuneCountInString(s.Text)
	}
	f.mu.Lock()
	f.sizes = append(f.sizes, size)
	f.mu.Unlock()
	if f.rejectAll != "" && len(texts) > 1 {
		for _, t := range texts {
			if strings.Contains(t, f.rejectAll) {
				return guardrails.Verdict{}, &guardrails.StatusError{Status: 400}
			}
		}
	}
	if f.slowWhen != nil && f.slowWhen(texts) {
		<-ctx.Done()
		return guardrails.Verdict{}, ctx.Err()
	}
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "SECRET") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "secrets.test_key", Category: "secrets", Severity: "warning", Score: 1, SegmentIndex: s.Index})
		}
	}
	return v, nil
}

func alwaysSlow([]string) bool { return true }

// The boundary: a timed-out run of exactly single_call_chars passes under
// fail open (the fail mode decides); one rune more and the text is
// unscanned and blocked, whatever the fail mode.
func TestAdversarial_SingleCallBoundary(t *testing.T) {
	const bound = guardrails.DefaultBudgetSingleCallChars
	cases := []struct {
		name      string
		sizes     []int // user message sizes, in runes
		wantBlock bool
	}{
		{"one message at the bound", []int{bound}, false},
		{"one message one over", []int{bound + 1}, true},
		{"two messages at the bound together", []int{bound / 2, bound / 2}, false},
		{"two messages one over together", []int{bound / 2, bound/2 + 1}, true},
		{"many small messages over", []int{4000, 4000, 4000, 4000, 4000, 4000, 4000, 4000, 4000}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			freshVerdictCache(t, 0)
			p := &sizeFake{slowWhen: alwaysSlow}
			f := providerFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "timeout_ms": 20, "scope": guardrails.ScopeAllMessages})
			var msgs []chatMsg
			for i, n := range c.sizes {
				text := strings.Repeat("a", n)
				if i == len(c.sizes)-1 {
					text = "SECRET" + text[len("SECRET"):]
				}
				msgs = append(msgs, chatMsg{Role: "user", Content: text})
			}
			out := runRequest(t, f, chatBody(t, msgs...), app(1))
			if out.Block != c.wantBlock {
				t.Errorf("block = %v, want %v (events %v, reason %q)", out.Block, c.wantBlock, eventTypes(out), unscannedReason(out))
			}
			if c.wantBlock && unscannedReason(out) == "" {
				t.Error("a padded, timed-out run recorded no guardrail.unscanned")
			}
		})
	}
}

// A lower single_call_chars moves the boundary down.
func TestAdversarial_SingleCallCharsConfigured(t *testing.T) {
	freshVerdictCache(t, 0)
	p := &sizeFake{slowWhen: alwaysSlow}
	f := providerFilter(t, p, map[string]any{
		"fail_mode": guardrails.FailOpen, "timeout_ms": 20,
		"budget": map[string]any{"single_call_chars": 1000},
	})
	out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "SECRET " + strings.Repeat("b", 1500)}), app(1))
	if !out.Block {
		t.Errorf("1.5K chars over a 1K single_call_chars passed a timed-out fail-open filter; events %v", eventTypes(out))
	}
}

// Padding with many tool results: the calls are split at the default call
// size, the one that times out costs only its own segments (quarantined),
// and the flagged text never reaches upstream under fail open.
// paddedToolTimeout runs 1 user prompt plus 40 tool results of 2.1K runes
// (84K in all) through a quarantine filter under fail open, with the
// provider timing out on the call holding tool result 5.
func paddedToolTimeout(t *testing.T) (*ScriptOutput, *sizeFake) {
	t.Helper()
	freshVerdictCache(t, 0)
	p := &sizeFake{slowWhen: func(texts []string) bool {
		for _, t := range texts {
			if strings.Contains(t, "SLOW") {
				return true
			}
		}
		return false
	}}
	f := providerFilter(t, p, map[string]any{
		"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine,
		"scope": guardrails.ScopeAllMessages, "timeout_ms": 1000, // generous: only the SLOW call may time out, even on a loaded CI runner
	})
	msgs := []chatMsg{{Role: "user", Content: "summarise the crawl"}}
	for i := 0; i < 40; i++ {
		text := strings.Repeat("filler ", 300) // 2100 runes each, 84K in all
		switch i {
		case 5:
			text += "SLOW SECRET"
		case 30:
			text += "plain SECRET"
		}
		msgs = append(msgs, chatMsg{Role: "tool", Content: text, ToolCallID: "c"})
	}
	out := runRequest(t, f, chatBody(t, msgs...), app(1))
	return out, p
}

func TestAdversarial_PaddedToolResultsTimeoutCostsOnlyThatCall(t *testing.T) {
	out, p := paddedToolTimeout(t)
	if out.Block {
		t.Fatalf("blocked; want the affected tool results quarantined and the prompt through: %s", out.Message)
	}
	if strings.Contains(out.Payload, "SECRET") {
		t.Error("flagged text reached upstream after a timeout under fail open")
	}
	if kept := strings.Count(out.Payload, strings.Repeat("filler ", 300)); kept == 0 {
		t.Error("every tool result was removed; a timeout must cost only its own call")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, n := range p.sizes {
		if n > 32<<10 {
			t.Errorf("a call of %d runes exceeds the default remote call size", n)
		}
	}
}

// The user's prompt must not share the fate of a tool result's failed call.
func TestAdversarial_ToolTimeoutKeepsUserPrompt(t *testing.T) {
	out, _ := paddedToolTimeout(t)
	if out.Block || !strings.Contains(out.Payload, "summarise the crawl") {
		t.Errorf("a timed-out call of tool results removed the user prompt (block=%v)", out.Block)
	}
}

// Azure-shaped caps (MaxChars = MaxCallChars = 10000): no call exceeds the
// limit, a phrase across a 10K cut is still found, and a timeout on one
// piece of a large segment does not pass it.
func TestAdversarial_AzureShapedCallSplitting(t *testing.T) {
	freshVerdictCache(t, 0)
	p := &sizeFake{caps: guardrails.Capabilities{MaxChars: 10000, MaxCallChars: 10000}}
	f := providerFilter(t, p, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages})
	text := strings.Repeat("x", 9997) + "SECRET" + strings.Repeat("y", 15000) // straddles the first cut
	out := runRequest(t, f, chatBody(t, chatMsg{Role: "user", Content: "go"}, chatMsg{Role: "tool", Content: text, ToolCallID: "c"}), app(1))
	if strings.Contains(out.Payload, "SECRET") {
		t.Error("a flag straddling a 10K cut was missed")
	}
	p.mu.Lock()
	for _, n := range p.sizes {
		if n > 10000 {
			t.Errorf("a call of %d runes exceeds MaxCallChars 10000", n)
		}
	}
	p.mu.Unlock()

	freshVerdictCache(t, 0)
	q := &sizeFake{caps: guardrails.Capabilities{MaxChars: 10000, MaxCallChars: 10000}, slowWhen: func(texts []string) bool {
		for _, t := range texts {
			if strings.Contains(t, "SECRET") {
				return true
			}
		}
		return false
	}}
	g := providerFilter(t, q, map[string]any{"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages, "timeout_ms": 30})
	padded := strings.Repeat("z", 20000) + " SECRET " + strings.Repeat("z", 5000)
	out = runRequest(t, g, chatBody(t, chatMsg{Role: "user", Content: "go"}, chatMsg{Role: "tool", Content: padded, ToolCallID: "c"}), app(1))
	if !out.Block && strings.Contains(out.Payload, "SECRET") {
		t.Errorf("a timed-out piece of a padded tool result passed under fail open; events %v", eventTypes(out))
	}
}

// A small run whose call is refused and re-sent is no longer single: a
// timeout in a re-sent call leaves that segment unscanned, not passed.
func TestAdversarial_ResplitNeverReclassifiesAsSingle(t *testing.T) {
	freshVerdictCache(t, 0)
	p := &sizeFake{rejectAll: "REJECTME", slowWhen: func(texts []string) bool {
		return len(texts) == 1 && strings.Contains(texts[0], "slow SECRET")
	}}
	f := providerFilter(t, p, map[string]any{
		"fail_mode": guardrails.FailOpen, "on_detect": guardrails.ActionQuarantine,
		"scope": guardrails.ScopeAllMessages, "timeout_ms": 30,
	})
	body := chatBody(t,
		chatMsg{Role: "user", Content: "go"},
		chatMsg{Role: "tool", Content: "REJECTME", ToolCallID: "c1"},
		chatMsg{Role: "tool", Content: "slow SECRET", ToolCallID: "c2"},
	)
	out := runRequest(t, f, body, app(1))
	if !out.Block && strings.Contains(out.Payload, "slow SECRET") {
		t.Errorf("a timeout in a re-sent call fell to fail_mode open; events %v", eventTypes(out))
	}
}

// 4uc.32: in a small run under fail_mode closed, a failure of a call that
// holds only tool results quarantines those results; it does not block the
// request. A failure of the call holding the user's own text still blocks.
func TestAdversarial_SmallMixedRunFailClosed(t *testing.T) {
	slowOn := func(marker string) func([]string) bool {
		return func(texts []string) bool {
			for _, t := range texts {
				if strings.Contains(t, marker) {
					return true
				}
			}
			return false
		}
	}
	body := func(t *testing.T) string {
		return chatBody(t,
			chatMsg{Role: "user", Content: "please summarise USERTEXT"},
			chatMsg{Role: "tool", Content: "fetched page TOOLTEXT", ToolCallID: "c1"},
			chatMsg{Role: "tool", Content: "second page TOOLTEXT", ToolCallID: "c2"},
		)
	}
	for _, action := range []string{guardrails.ActionBlock, guardrails.ActionQuarantine, guardrails.ActionLog} {
		t.Run("tool call fails/"+action, func(t *testing.T) {
			freshVerdictCache(t, 0)
			p := &sizeFake{slowWhen: slowOn("TOOLTEXT")}
			f := providerFilter(t, p, map[string]any{"fail_mode": guardrails.FailClosed, "on_detect": action, "timeout_ms": 500})
			out := runRequest(t, f, body(t), app(1))
			if out.Block {
				t.Fatalf("a failed tool-only call blocked the request; events %v", eventTypes(out))
			}
			if strings.Contains(out.Payload, "TOOLTEXT") {
				t.Error("unscanned tool text reached upstream under fail_mode closed")
			}
			if !strings.Contains(out.Payload, "USERTEXT") {
				t.Error("the user's scanned prompt was removed")
			}
		})
	}
	t.Run("user call fails", func(t *testing.T) {
		freshVerdictCache(t, 0)
		p := &sizeFake{slowWhen: slowOn("USERTEXT")}
		f := providerFilter(t, p, map[string]any{"fail_mode": guardrails.FailClosed, "on_detect": guardrails.ActionQuarantine, "timeout_ms": 500})
		if out := runRequest(t, f, body(t), app(1)); !out.Block {
			t.Errorf("an unscanned user prompt passed fail_mode closed; payload %s", out.Payload)
		}
	})
}
