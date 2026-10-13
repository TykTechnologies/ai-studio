package scripting

// Adversarial tests for 4uc.27 and the 4uc.4 budgets (tests agent): the
// re-split after a provider rejection stays inside the piece budget, never
// recurses, never lets an unscanned segment through, and lets the clean
// segments of a rejected call pass; the regression pin that a single slow
// call under fail open still passes holds with tool results in the request.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// splitFake rejects (400) any call that holds a segment with "REJECTME",
// optionally waits per call, and records every call's segment texts.
type splitFake struct {
	caps  guardrails.Capabilities
	delay time.Duration

	mu    sync.Mutex
	calls [][]string
}

func (f *splitFake) Capabilities() guardrails.Capabilities { return f.caps }

func (f *splitFake) Classify(ctx context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	var texts []string
	for _, s := range in.Segments {
		texts = append(texts, s.Text)
	}
	f.mu.Lock()
	f.calls = append(f.calls, texts)
	f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return guardrails.Verdict{}, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "REJECTME") {
			return guardrails.Verdict{}, &guardrails.StatusError{Status: 400}
		}
	}
	return guardrails.Verdict{}, nil
}

func (f *splitFake) piecesSent() (calls, pieces int, perText map[string]int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	perText = map[string]int{}
	for _, c := range f.calls {
		pieces += len(c)
		for _, t := range c {
			perText[t]++
		}
	}
	return len(f.calls), pieces, perText
}

func toolBody(t *testing.T, user string, tools ...string) string {
	t.Helper()
	msgs := []chatMsg{{Role: "user", Content: user}}
	for i, tool := range tools {
		msgs = append(msgs, chatMsg{Role: "tool", Content: tool, ToolCallID: fmt.Sprintf("c%d", i)})
	}
	return chatBody(t, msgs...)
}

// Whatever max_pieces is, the pieces sent over the first pass and the
// re-split never exceed it; every rejected tool result is removed; every
// segment the budget left unscanned is removed too; nothing else is.
func TestAdversarial_ResplitWithinPieceCap(t *testing.T) {
	tools := []string{"t0 ok", "t1 REJECTME", "t2 ok", "t3 REJECTME", "t4 ok", "t5 ok", "t6 REJECTME", "t7 ok"}
	for _, maxPieces := range []int{1, 3, 9, 10, 12, 18, 30} {
		t.Run(fmt.Sprint(maxPieces), func(t *testing.T) {
			freshVerdictCache(t, 0)
			p := &splitFake{}
			f := providerFilter(t, p, map[string]any{
				"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages,
				"fail_mode": guardrails.FailOpen, "budget": map[string]any{"max_pieces": maxPieces},
			})
			out := runRequest(t, f, toolBody(t, "summarise all eight", tools...), app(1))
			_, pieces, perText := p.piecesSent()
			if pieces > maxPieces {
				t.Errorf("%d pieces sent, over max_pieces %d", pieces, maxPieces)
			}
			if out.Block {
				// Only the user prompt left unscanned can block under
				// quarantine; it must then really be unscanned.
				if perText["summarise all eight"] >= 2 {
					t.Errorf("blocked although the user prompt was scanned clean alone: %s", out.Message)
				}
				return
			}
			for _, tool := range tools {
				passed := strings.Contains(out.Payload, `"`+tool+`"`)
				scannedAlone := perText[tool] >= 2 || (perText[tool] == 1 && pieces == 1)
				switch {
				case strings.Contains(tool, "REJECTME") && passed:
					t.Errorf("%q passed although the provider refused it", tool)
				case passed && !scannedAlone && callHadRejection(p, tool):
					t.Errorf("%q passed although its only scan was a refused call", tool)
				}
			}
		})
	}
}

// callHadRejection reports whether every call that carried text was refused.
func callHadRejection(p *splitFake, text string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.calls {
		var has, rejected bool
		for _, t := range c {
			has = has || t == text
			rejected = rejected || strings.Contains(t, "REJECTME")
		}
		if has && !rejected {
			return false
		}
	}
	return true
}

// A segment refused on its own is not split or re-sent again: one level of
// re-split, then it is unscanned.
func TestAdversarial_ResplitDoesNotRecurse(t *testing.T) {
	freshVerdictCache(t, 0)
	// MaxChars small enough that the rejected tool result is three pieces.
	p := &splitFake{caps: guardrails.Capabilities{MaxChars: 40}}
	f := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages, "fail_mode": guardrails.FailOpen})
	long := strings.Repeat("x", 50) + " REJECTME " + strings.Repeat("y", 30)
	out := runRequest(t, f, toolBody(t, "go", long), app(1))
	calls, _, perText := p.piecesSent()
	if calls > 3 {
		t.Errorf("%d calls; want at most the first pass plus one re-send per segment", calls)
	}
	for text, n := range perText {
		if n > 2 {
			t.Errorf("piece %q sent %d times", text, n)
		}
	}
	if out.Block || strings.Contains(out.Payload, "REJECTME") || !strings.Contains(out.Payload, `"go"`) {
		t.Errorf("want the tool result quarantined and the prompt kept; block=%v payload=%s", out.Block, out.Payload)
	}

	// A call that held only the refused segment is not re-sent at all.
	freshVerdictCache(t, 0)
	q := &splitFake{}
	g := providerFilter(t, q, map[string]any{"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeLastUser, "fail_mode": guardrails.FailOpen})
	runRequest(t, g, chatBody(t, chatMsg{Role: "user", Content: "REJECTME alone"}), app(1))
	if n, _, _ := q.piecesSent(); n != 1 {
		t.Errorf("a single refused segment was sent %d times, want once", n)
	}
}

// When the time budget runs out during the re-split, the segments it did
// not reach are unscanned and take the action; none passes as clean.
func TestAdversarial_TimeBudgetExhaustedMidResplit(t *testing.T) {
	freshVerdictCache(t, 0)
	p := &splitFake{delay: 40 * time.Millisecond}
	f := providerFilter(t, p, map[string]any{
		"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages, "fail_mode": guardrails.FailOpen,
		"timeout_ms": 50, "budget": map[string]any{"time_ms": 100},
	})
	tools := []string{"a ok", "b REJECTME", "c ok", "d ok", "e ok", "f ok", "g ok", "h ok", "i ok", "j ok"}
	out := runRequest(t, f, toolBody(t, "summarise", tools...), app(1))
	_, _, perText := p.piecesSent()
	for _, tool := range tools {
		if strings.Contains(out.Payload, `"`+tool+`"`) && perText[tool] < 2 {
			t.Errorf("%q passed although the time budget ended before it was scanned alone", tool)
		}
	}
	if !out.Block && strings.Contains(out.Payload, "REJECTME") {
		t.Error("the refused tool result passed")
	}
	if unscannedReason(out) == "" {
		t.Errorf("no guardrail.unscanned event; events %v", eventTypes(out))
	}
}

// Regression pin: one slow call under fail open still lets the request
// through unchanged, tool results included; under fail closed it blocks.
func TestAdversarial_SingleSlowCallKeepsFailMode(t *testing.T) {
	for mode, wantBlock := range map[string]bool{guardrails.FailOpen: false, guardrails.FailClosed: true} {
		t.Run(mode, func(t *testing.T) {
			freshVerdictCache(t, 0)
			p := &splitFake{delay: time.Second}
			f := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionQuarantine, "scope": guardrails.ScopeAllMessages, "fail_mode": mode, "timeout_ms": 30})
			body := toolBody(t, "go", "tool output one", "tool output two")
			out := runRequest(t, f, body, app(1))
			if out.Block != wantBlock {
				t.Fatalf("block = %v, want %v; events %v", out.Block, wantBlock, eventTypes(out))
			}
			if !wantBlock && out.Payload != body {
				t.Errorf("fail open changed the body: %s", out.Payload)
			}
			if r := unscannedReason(out); r != "" {
				t.Errorf("a single-call timeout was treated as unscanned (%s); fail mode must decide", r)
			}
		})
	}
}

// An edit that changes only the action (log -> block) takes effect at
// once on text whose verdict is cached: the hub stamps every edit with a new
// UpdatedAt, which the edge keeps.
func TestAdversarial_ActionEditActsOnCachedText(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &roleFake{flagRoles: map[string]bool{"user": true}}
	f := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionLog})
	body := chatBody(t, chatMsg{Role: "user", Content: "hello SECRET"})
	if out := runRequest(t, f, body, app(1)); out.Block {
		t.Fatal("a log filter blocked")
	}
	edited := *f
	edited.UpdatedAt = f.UpdatedAt.Add(time.Nanosecond)
	edited.Config = map[string]any{}
	for k, v := range f.Config {
		edited.Config[k] = v
	}
	edited.Config["on_detect"] = guardrails.ActionBlock
	if out := runRequest(t, &edited, body, app(1)); !out.Block {
		t.Error("after the edit to block, text seen under the old version still passes")
	}
}

// An edited config takes effect even when the filter's version stamp did
// not move (the hub-side half of 01t: the prepared-filter cache keys on
// ID@UpdatedAt for preparedTTL).
func TestAdversarial_NoStaleVerdictAfterConfigEditSameVersion(t *testing.T) {
	freshVerdictCache(t, time.Minute)
	p := &roleFake{flagRoles: map[string]bool{"user": true}}
	f := providerFilter(t, p, map[string]any{"on_detect": guardrails.ActionLog})
	body := chatBody(t, chatMsg{Role: "user", Content: "hello SECRET"})
	runRequest(t, f, body, app(1))
	edited := *f
	edited.Config = map[string]any{}
	for k, v := range f.Config {
		edited.Config[k] = v
	}
	edited.Config["on_detect"] = guardrails.ActionBlock
	if out := runRequest(t, &edited, body, app(1)); !out.Block {
		t.Error("an edited filter (log -> block) with the same ID@UpdatedAt reused the old config")
	}
}
